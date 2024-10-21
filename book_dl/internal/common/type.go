package common

import (
	"bufio"
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SirZenith/bilinovel/base"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
)

type Options struct {
	TargetURL    string // TOC URL for novel
	OutputDir    string // output directory for downloaded HTML page
	ImgOutputDir string // output directory for downloaded images

	HeaderFile         string // header file path
	ChapterNameMapFile string // chapter name mapping JSON file path

	RequestDelay int64 // delay for each download request
	Timeout      int64 // download timeout
}

// ----------------------------------------------------------------------------

type NameMapEntry struct {
	URL   string `json:"url"`   // URL of the first page of chapter
	Title string `json:"title"` // chapter title on TOC web page
	File  string `json:"file"`  // final title title used in file name for saving downloaded content
}

type GardedNameMap struct {
	lock    sync.Mutex
	NameMap map[string]NameMapEntry
}

// Reads name map from JSON.
func (m *GardedNameMap) ReadNameMap(path string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else {
			return err
		}
	}

	list := []NameMapEntry{}
	err = json.Unmarshal(data, &list)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %s", path, err)
	}

	for _, entry := range list {
		m.NameMap[entry.URL] = entry
	}

	return nil
}

// Save current name map to file.
func (m *GardedNameMap) SaveNameMap(path string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	list := []NameMapEntry{}
	for _, entry := range m.NameMap {
		list = append(list, entry)
	}

	data, err := json.MarshalIndent(list, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to convert data to JSON: %s", err)
	}

	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return fmt.Errorf("faield to write name map %s: %s", path, err)
	}

	return nil
}

// Get file name of given chapter key, when title name can not be found in
// current name map, empty string will be returned.
func (m *GardedNameMap) GetMapTo(url string) string {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.NameMap[url].File
}

// Sets file name used by a chapter key.
func (m *GardedNameMap) SetMapTo(entry *NameMapEntry) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.NameMap[entry.URL] = *entry
}

// ----------------------------------------------------------------------------

type VolumeInfo struct {
	VolIndex        int
	Title           string
	TotalChapterCnt int

	OutputDir    string
	ImgOutputDir string
}

type ChapterInfo struct {
	VolumeInfo
	ChapIndex int    // chapter index of this chapter
	Title     string // chapter title

	URL string // absolute URL of the first page of chapter
}

type PageContent struct {
	PageNumber     int    // page number of this content in this chapter
	Title          string // display title of the chapter will be update to this value if it's not empty
	Content        string // page content
	IsFinished     bool   // this should be true if current content is the last page of this chapter
	NextChapterURL string // when non-empty, it's value will be used to initialize downloading of next chapter
}

type ChapterDownloadState struct {
	Info          ChapterInfo
	RootNameExt   string
	RootNameStem  string
	ResultChan    chan PageContent
	CurPageNumber int
}

// Composes outputpath of chapter content with chapter info.
func (c *ChapterInfo) GetChapterOutputPath(title string) string {
	outputTitle := base.InvalidPathCharReplace(title)
	if outputTitle == "" {
		outputTitle = fmt.Sprintf("Chap.%04d.html", c.ChapIndex+1)
	} else {
		outputTitle = fmt.Sprintf("%04d - %s.html", c.ChapIndex+1, outputTitle)
	}

	return filepath.Join(c.OutputDir, outputTitle)
}

// Composes chapter key used by name map look up.
func (c *ChapterInfo) GetNameMapKey(title string) string {
	return fmt.Sprintf("%03d-%04d-%s", c.VolIndex+1, c.ChapIndex+1, title)
}

func (c *ChapterInfo) GetLogName(title string) string {
	return fmt.Sprintf("Vol.%03d - Chap.%04d - %s", c.VolIndex+1, c.ChapIndex+1, title)
}

// ----------------------------------------------------------------------------

// Spawns new colly job for downloading chapter pages.
// One can get a `chapterDownloadState` pointer from request contenxt with key
// `downloadState`.
func CollectChapterPages(r *colly.Request, timeout int64, info ChapterInfo) {
	ctx := r.Ctx
	options := ctx.GetAny("options").(*Options)
	collector := ctx.GetAny("collector").(*colly.Collector)
	nameMap := ctx.GetAny("nameMap").(*GardedNameMap)

	if strings.HasPrefix(info.URL, "javascript:") {
		log.Warnf("not supported chapter URL %s: %s", info.GetLogName(info.Title), info.URL)
		return
	}

	if visited, _ := collector.HasVisited(info.URL); visited {
		return
	}

	// check skip
	existingTitle := checkShouldSkipChapter(nameMap, &info)
	if existingTitle != "" {
		updateChapterNameMap(nameMap, options.ChapterNameMapFile, &info, existingTitle)
		log.Infof("skip chapter: %s", info.GetLogName(existingTitle))
		return
	}

	// downloading
	resultChan := make(chan PageContent, 5)
	dlCtx := makeChapterPageContext(info, resultChan)

	collector.Request("GET", info.URL, nil, dlCtx, r.Headers.Clone())

	waitResult := waitPages(info.Title, resultChan, timeout)
	if waitResult.Err != nil {
		onWaitPagesError(&info, waitResult.Err)
		return
	}

	pageCnt := waitResult.PageList.Len()
	waitResult.PageList.PushFront(PageContent{
		Content: "<h1 class=\"chapter-title\">" + waitResult.Title + "</h1>\n",
	})

	// save content to file
	outputName := info.GetChapterOutputPath(waitResult.Title)
	if err := saveChapterContent(waitResult.PageList, outputName); err == nil {
		updateChapterNameMap(nameMap, options.ChapterNameMapFile, &info, waitResult.Title)
		log.Infof("save chapter (%dp): %s", pageCnt, info.GetLogName(waitResult.Title))
	} else {
		log.Warnf("error occured during saving %s: %s", outputName, err)
		return
	}

	// try to go to next chapter
	if info.ChapIndex < info.TotalChapterCnt-1 && waitResult.NextChapterURL != "" {
		nextURL := r.AbsoluteURL(waitResult.NextChapterURL)

		if visited, err := collector.HasVisited(nextURL); err == nil && !visited {
			CollectChapterPages(r, timeout, ChapterInfo{
				VolumeInfo: info.VolumeInfo,
				ChapIndex:  info.ChapIndex + 1,
				URL:        nextURL,
			})
		}
	}
}

// Checks if downloading of a chapter can be skipped. If yes, then title name
// used by downloaded file will be return, else empty string will be returned.
func checkShouldSkipChapter(nameMap *GardedNameMap, info *ChapterInfo) string {
	title := nameMap.GetMapTo(info.URL)
	if title == "" {
		return ""
	}

	outputName := info.GetChapterOutputPath(title)
	_, err := os.Stat(outputName)
	if err != nil {
		return ""
	}

	return title
}

// Makes a context variable with necessary download state infomation in it.
func makeChapterPageContext(info ChapterInfo, resultChan chan PageContent) *colly.Context {
	rootBaseName := path.Base(info.URL)
	rootExt := path.Ext(rootBaseName)
	rootBaseStem := rootBaseName[:len(rootBaseName)-len(rootExt)]
	state := ChapterDownloadState{
		Info:          info,
		RootNameExt:   rootExt,
		RootNameStem:  rootBaseStem,
		ResultChan:    resultChan,
		CurPageNumber: 1,
	}

	var onError colly.ErrorCallback = func(_ *colly.Response, err error) {
		log.Warnf("failed to download %s: %s", state.Info.Title, err)
		close(resultChan)
	}

	ctx := colly.NewContext()
	ctx.Put("downloadState", &state)
	ctx.Put("onError", onError)

	return ctx
}

type WaitPagesResult struct {
	PageList       *list.List
	Title          string
	NextChapterURL string
	Err            error
}

// Collects all pages sent from colly jobs with timeout.
func waitPages(title string, resultChan chan PageContent, timeout int64) WaitPagesResult {
	pageList := list.New()
	pageList.Init()

	waitResult := WaitPagesResult{
		PageList: pageList,
		Title:    title,
	}

	timeoutDuration := time.Duration(timeout) * time.Millisecond

loop:
	for {
		select {
		case data, ok := <-resultChan:
			if !ok {
				waitResult.Err = fmt.Errorf("request failed")
				break loop
			}

			insertChapterPage(pageList, data)

			if data.Title != "" {
				waitResult.Title = data.Title
			}

			if data.NextChapterURL != "" {
				waitResult.NextChapterURL = data.NextChapterURL
			}

			if data.IsFinished {
				break loop
			}
		case <-time.After(timeoutDuration):
			waitResult.Err = fmt.Errorf("download timeout")
			break loop
		}
	}

	return waitResult
}

// Handling error happended during download chapter pages, write a marker file
// as a record of error.
func onWaitPagesError(info *ChapterInfo, err error) {
	outputName := info.GetChapterOutputPath(info.Title)
	outputDir := filepath.Dir(outputName)
	outputBase := filepath.Base(outputName)

	failedName := filepath.Join(outputDir, "failed - "+outputBase+".mark")

	failedContent := info.URL + "\n" + err.Error()

	os.WriteFile(failedName, []byte(failedContent), 0o644)

	log.Warnf("failed to download %s: %s", info.Title, err)
}

// Inserts newly fetched page content into page list according its page number.
func insertChapterPage(list *list.List, data PageContent) {
	target := list.Front()
	if target == nil {
		list.PushFront(data)
		return
	}

	for target.Value.(PageContent).PageNumber < data.PageNumber {
		next := target.Next()
		if next == nil {
			break
		}

		target = next
	}

	list.InsertAfter(data, target)
}

// Writes content of chapter pages to file.
func saveChapterContent(list *list.List, outputName string) error {
	file, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to open output file %s: %s", outputName, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for element := list.Front(); element != nil; element = element.Next() {
		writer.WriteString(element.Value.(PageContent).Content)
	}

	if err = writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush chapter file %s: %s", outputName, err)
	}

	return nil
}

// Saves name map to file.
func updateChapterNameMap(nameMap *GardedNameMap, saveTo string, info *ChapterInfo, fileTitle string) {
	title := info.Title
	if title == "" {
		title = fileTitle
	}

	entry := &NameMapEntry{
		URL:   info.URL,
		Title: info.GetNameMapKey(title),
		File:  fileTitle,
	}

	nameMap.SetMapTo(entry)

	nameMap.SaveNameMap(saveTo)
}
