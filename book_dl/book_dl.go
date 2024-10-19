package book_dl

import (
	"bufio"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SirZenith/bilinovel/base"
	"github.com/gocolly/colly/v2"
	"github.com/urfave/cli/v3"
)

const defaultHtmlOutput = "./text"
const defaultImgOutput = "./image"

const defaultNameMapPath = "./name_map.json"

type options struct {
	targetURL    string // TOC URL for novel
	outputDir    string // output directory for downloaded HTML page
	imgOutputDir string // output directory for downloaded images

	headerFile         string // header file path
	chapterNameMapFile string // chapter name mapping JSON file path

	requestDelay time.Duration // delay for each download request
	timeout      time.Duration // download timeout
}

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:    "download",
		Aliases: []string{"dl"},
		Usage:   "download book from www.bilinovel.com or www.linovelib.com with book's TOC page URL",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "url",
				Value: "",
				Usage: "url of book's table of contents page",
			},
			&cli.StringFlag{
				Name:  "output",
				Value: "",
				Usage: fmt.Sprintf("output directory for downloaded HTML (default: %s)", defaultHtmlOutput),
			},
			&cli.StringFlag{
				Name:  "img-output",
				Value: "",
				Usage: fmt.Sprintf("output directory for downloaded images (default: %s)", defaultImgOutput),
			},
			&cli.DurationFlag{
				Name:  "delay",
				Value: 1500 * time.Millisecond,
				Usage: "page request delay in milisecond",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Value: 8 * time.Second,
				Usage: "request timeout for content page",
			},
			&cli.StringFlag{
				Name:  "header-file",
				Value: "",
				Usage: "a JSON file containing header info, headers is given in form of Array<{ name: string, value: string }>",
			},
			&cli.StringFlag{
				Name:  "name-map",
				Value: "",
				Usage: "a JSON file containing name mapping between chapter title and actual output file, in form of Array<{ title: string, file: string }>",
			},
			&cli.StringFlag{
				Name:  "info-file",
				Value: "",
				Usage: "path of book info JSON, if given command will try to download with option written in info file",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			if options.targetURL == "" {
				return fmt.Errorf("no TOC URL is given, please use --url flag to specify one or use --info-file flag to give a book info JSON")
			}

			return cmdMain(options)
		},
	}

	return cmd
}

func getOptionsFromCmd(cmd *cli.Command) (options, error) {
	options := options{
		targetURL:    cmd.String("url"),
		outputDir:    cmd.String("output"),
		imgOutputDir: cmd.String("img-output"),

		headerFile:         cmd.String("header-file"),
		chapterNameMapFile: cmd.String("name-map"),

		requestDelay: cmd.Duration("delay"),
		timeout:      cmd.Duration("timeout"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := base.ReadBookInfo(infoFile)
		if err != nil {
			return options, err
		}

		options.targetURL = base.GetStrOr(options.targetURL, bookInfo.TocURL)
		options.outputDir = base.GetStrOr(options.outputDir, bookInfo.RawHTMLOutput)
		options.imgOutputDir = base.GetStrOr(options.imgOutputDir, bookInfo.ImgOutput)

		options.headerFile = base.GetStrOr(options.headerFile, bookInfo.HeaderFile)
		options.chapterNameMapFile = base.GetStrOr(options.chapterNameMapFile, bookInfo.NameMapFile)
	}

	options.outputDir = base.GetStrOr(options.outputDir, defaultHtmlOutput)
	options.imgOutputDir = base.GetStrOr(options.imgOutputDir, defaultImgOutput)

	options.chapterNameMapFile = base.GetStrOr(options.chapterNameMapFile, defaultNameMapPath)

	return options, nil
}

func cmdMain(options options) error {
	writer := log.Writer()
	fmt.Fprintln(writer, "download    :", options.targetURL)
	fmt.Fprintln(writer, "text  output:", options.outputDir)
	fmt.Fprintln(writer, "image output:", options.imgOutputDir)

	c, err := makeCollector(options)
	if err != nil {
		return err
	}

	c.Visit(options.targetURL)
	c.Wait()

	return nil
}

// Returns collector used for novel downloading.
func makeCollector(options options) (*colly.Collector, error) {
	// ensure output directory
	if stat, err := os.Stat(options.outputDir); errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(options.outputDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %s", err)
		}
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("An file with name %s already exists", options.outputDir)
	}

	// load headers
	headers := map[string]string{}
	if options.headerFile != "" {
		err := readHeaderFile(options.headerFile, headers)
		if err != nil {
			return nil, err
		}
	}

	// load name map
	nameMap := &gardedNameMap{nameMap: make(map[string]string)}
	if options.chapterNameMapFile != "" {
		err := nameMap.readNameMap(options.chapterNameMapFile)
		if err != nil {
			return nil, err
		}
	}

	c := colly.NewCollector(
		colly.Headers(headers),
		colly.Async(true),
	)
	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("options", &options)
		r.Ctx.Put("collector", c)
		r.Ctx.Put("nameMap", nameMap)
	})
	c.OnResponse(func(r *colly.Response) {
		if data, err := decompressResponseBody(r); err == nil {
			r.Body = data
		} else {
			log.Println(err)
		}

		if dlName := r.Ctx.Get("dlFileTo"); dlName != "" {
			downloadFile(r, dlName)
		}
	})
	c.OnError(func(r *colly.Response, err error) {
		if onError, ok := r.Ctx.GetAny("onError").(colly.ErrorCallback); ok {
			onError(r, err)
		} else {
			log.Printf("error requesting %s: %s", r.Request.URL, err)
		}
	})

	if url, err := url.Parse(options.targetURL); err == nil {
		hostname := url.Hostname()

		if strings.HasSuffix(hostname, "bilinovel.com") {
			mobileSetupCollector(c, options)
		} else if strings.HasSuffix(hostname, "linovelib.com") {
			desktopSetupCollector(c, options)
		}
	}

	return c, nil
}

type headerValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Reads header value from file and stores then into the map passed as argument.
// Header file should a JSON containing array of header objects. Each header
// objects should be object with tow string field `name` and `value`.
func readHeaderFile(path string, result map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %s", path, err)
	}

	list := []headerValue{}
	err = json.Unmarshal(data, &list)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %s", path, err)
	}

	for _, entry := range list {
		result[entry.Name] = entry.Value
	}

	return nil
}

type nameMapEntry struct {
	Title string `json:"title"` // chapter title on TOC web page
	File  string `json:"file"`  // final title title used in file name for saving downloaded content
}

type gardedNameMap struct {
	lock    sync.Mutex
	nameMap map[string]string
}

// Reads name map from JSON.
func (m *gardedNameMap) readNameMap(path string) error {
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

	list := []nameMapEntry{}
	err = json.Unmarshal(data, &list)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %s", path, err)
	}

	for _, entry := range list {
		m.nameMap[entry.Title] = entry.File
	}

	return nil
}

// Save current name map to file.
func (m *gardedNameMap) saveNameMap(path string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	list := []nameMapEntry{}
	for title, file := range m.nameMap {
		list = append(list, nameMapEntry{Title: title, File: file})
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
func (m *gardedNameMap) getFileTitle(key string) string {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.nameMap[key]
}

// Sets file name used by a chapter key.
func (m *gardedNameMap) setFileTitle(key string, filename string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.nameMap[key] = filename
}

// ----------------------------------------------------------------------------

// Save response body to file.
func downloadFile(r *colly.Response, outputName string) {
	if err := os.WriteFile(outputName, r.Body, 0o644); err == nil {
		log.Println("file downloaded:", outputName)
	} else {
		log.Printf("failed to save file %s: %s\n", outputName, err)
	}
}

const nextPageTextTC = "下一頁"
const nextPageTextSC = "下一页"

type volumeInfo struct {
	volIndex int
	title    string

	outputDir    string
	imgOutputDir string
}

type chapterInfo struct {
	volumeInfo
	chapIndex int    // chapter index of this chapter
	title     string // chapter title

	url string
}

type pageContent struct {
	pageNumber int    // page number of this content in this chapter
	title      string // display title of the chapter will be update to this value if it's not empty
	content    string // page content
	isFinished bool   // this should be true if current content is the last page of this chapter
}

type chapterDownloadState struct {
	info          chapterInfo
	rootNameExt   string
	rootNameStem  string
	resultChan    chan pageContent
	curPageNumber int
}

// Composes outputpath of chapter content with chapter info.
func (c *chapterInfo) getChapterOutputPath(title string) string {
	var outputTitle string
	if title == "" {
		outputTitle = fmt.Sprintf("Chap.%04d.html", c.chapIndex+1)
	} else {
		outputTitle = fmt.Sprintf("%04d - %s.html", c.chapIndex+1, title)
	}

	return filepath.Join(c.outputDir, outputTitle)
}

// Composes chapter key used by name map look up.
func (c *chapterInfo) getNameMapKey() string {
	return fmt.Sprintf("%03d-%04d-%s", c.volIndex+1, c.chapIndex+1, c.title)
}

func (c *chapterInfo) getLogName(title string) string {
	return fmt.Sprintf("Vol.%03d - Chap.%04d - %s", c.volIndex+1, c.chapIndex+1, title)
}

// ----------------------------------------------------------------------------

// Spawns new colly job for downloading chapter pages.
// One can get a `chapterDownloadState` pointer from request contenxt with key
// `downloadState`.
func collectChapterPages(e *colly.HTMLElement, info chapterInfo) {
	ctx := e.Request.Ctx
	options := ctx.GetAny("options").(*options)
	collector := ctx.GetAny("collector").(*colly.Collector)
	nameMap := ctx.GetAny("nameMap").(*gardedNameMap)

	// check skip
	existingTitle := checkShouldSkipChapter(nameMap, &info)
	if existingTitle != "" {
		log.Printf("skip chapter: %s\n", info.getLogName(existingTitle))
		return
	}

	// downloading
	resultChan := make(chan pageContent, 5)
	dlCtx := makeChapterPageContext(info, resultChan)

	url := e.Request.AbsoluteURL(info.url)
	collector.Request("GET", url, nil, dlCtx, e.Request.Headers.Clone())

	pageList, title, err := waitPages(info.title, resultChan, options.timeout)
	if err != nil {
		onWaitPagesError(&info, err)
		return
	}

	pageCnt := pageList.Len()
	pageList.PushFront(pageContent{
		content: "<h1 class=\"chapter-title\">" + title + "</h1>\n",
	})

	// save content to file
	outputName := info.getChapterOutputPath(title)
	err = saveChapterContent(pageList, outputName)
	if err != nil {
		log.Printf("error occured during saving %s: %s", outputName, err)
		return
	}

	key := info.getNameMapKey()
	nameMap.setFileTitle(key, title)
	nameMap.saveNameMap(options.chapterNameMapFile)

	log.Printf("save chapter (%dp): %s\n", pageCnt, info.getLogName(title))
}

// Checks if downloading of a chapter can be skipped. If yes, then title name
// used by downloaded file will be return, else empty string will be returned.
func checkShouldSkipChapter(nameMap *gardedNameMap, info *chapterInfo) string {
	key := info.getNameMapKey()

	title := nameMap.getFileTitle(key)
	if title == "" {
		return ""
	}

	outputName := info.getChapterOutputPath(title)
	_, err := os.Stat(outputName)
	if err != nil {
		return ""
	}

	return title
}

// Makes a context variable with necessary download state infomation in it.
func makeChapterPageContext(info chapterInfo, resultChan chan pageContent) *colly.Context {
	rootBaseName := path.Base(info.url)
	rootExt := path.Ext(rootBaseName)
	rootBaseStem := rootBaseName[:len(rootBaseName)-len(rootExt)]
	state := chapterDownloadState{
		info:          info,
		rootNameExt:   rootExt,
		rootNameStem:  rootBaseStem,
		resultChan:    resultChan,
		curPageNumber: 1,
	}

	var onError colly.ErrorCallback = func(_ *colly.Response, err error) {
		log.Printf("failed to download %s: %s", state.info.title, err)
		close(resultChan)
	}

	ctx := colly.NewContext()
	ctx.Put("downloadState", &state)
	ctx.Put("onError", onError)

	return ctx
}

// Collects all pages sent from colly jobs with timeout.
func waitPages(title string, resultChan chan pageContent, timeout time.Duration) (*list.List, string, error) {
	pageList := list.New()
	pageList.Init()

	outputTitle := title

	var err error
loop:
	for {
		select {
		case data, ok := <-resultChan:
			if !ok {
				err = fmt.Errorf("request failed")
				break loop
			}

			insertChapterPage(pageList, data)

			if data.title != "" {
				outputTitle = data.title
			}

			if data.isFinished {
				break loop
			}
		case <-time.After(timeout):
			err = fmt.Errorf("download timeout")
			break loop
		}
	}

	return pageList, outputTitle, err
}

// Handling error happended during download chapter pages, write a marker file
// as a record of error.
func onWaitPagesError(info *chapterInfo, err error) {
	outputName := info.getChapterOutputPath(info.title)
	outputDir := filepath.Dir(outputName)
	outputBase := filepath.Base(outputName)

	failedName := filepath.Join(outputDir, "failed - "+outputBase+".mark")

	failedContent := info.url + "\n" + err.Error()

	os.WriteFile(failedName, []byte(failedContent), 0o644)

	log.Printf("failed to download %s: %s\n", info.title, err)
}

// Inserts newly fetched page content into page list according its page number.
func insertChapterPage(list *list.List, data pageContent) {
	target := list.Front()
	if target == nil {
		list.PushFront(data)
		return
	}

	for target.Value.(pageContent).pageNumber < data.pageNumber {
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
		writer.WriteString(element.Value.(pageContent).content)
	}

	if err = writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush chapter file %s: %s", outputName, err)
	}

	return nil
}
