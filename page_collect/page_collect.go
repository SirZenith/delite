package page_collect

import (
	"bufio"
	"container/list"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SirZenith/delite/database/data_model"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Spawns new colly job for downloading chapter pages.
// One can get a `chapterDownloadState` pointer from request contenxt with key
// `downloadState`.
func CollectChapterPages(r *colly.Request, timeout time.Duration, info ChapterInfo) {
	global := r.Ctx.GetAny("global").(*CtxGlobal)
	collector := global.Collector
	db := global.Db

	if global.Link.CheckVisited(info.VolIndex, info.ChapIndex) {
		return
	}

	if strings.HasPrefix(info.URL, "javascript:") {
		info.URL = global.Link.GetAndRemoveURL(info.VolIndex, info.ChapIndex)
		if info.URL == "" {
			log.Warnf("no valid URL found for %s, cache it for latter use", info.GetLogName(info.Title))
			global.Link.SetVolInfo(info.VolIndex, info.ChapIndex, &info.VolumeInfo)
			return
		}
	}

	// check skip
	existingTitle := checkShouldSkipChapter(db, &info)
	if existingTitle != "" {
		log.Infof("skip chapter: %s", info.GetLogName(existingTitle))
		return
	}

	// downloading
	resultChan := make(chan PageContent, 5)
	dlCtx := makeChapterPageContext(info, resultChan, global.Target.Options.RetryCnt)

	collector.Request("GET", info.URL, nil, dlCtx, r.Headers.Clone())

	waitResult := waitPages(info.Title, timeout, resultChan)
	if waitResult.Err != nil {
		onWaitPagesError(&info, waitResult.Err)
		return
	}

	pageCnt := waitResult.PageList.Len()
	if pageCnt > 0 {
		waitResult.PageList.PushFront(PageContent{
			Content: "<h1 class=\"chapter-title\">" + waitResult.Title + "</h1>\n",
		})

		// save content to file
		outputName := info.GetChapterOutputPath(waitResult.Title)
		if err := saveChapterContent(waitResult.PageList, outputName); err == nil {
			saveChapterFileEntry(db, global.Target.ChapterNameMapFile, &info, waitResult.Title)
			log.Infof("save chapter (%dp): %s", pageCnt, info.GetLogName(waitResult.Title))
		} else {
			log.Warnf("error occured during saving %s: %s", outputName, err)
			return
		}
	}

	tryGoToNextChapter(r, timeout, info, waitResult)
}

// Checks if downloading of a chapter can be skipped. If yes, then title name
// used by downloaded file will be return, else empty string will be returned.
func checkShouldSkipChapter(db *gorm.DB, info *ChapterInfo) string {
	if db == nil {
		return ""
	}

	entry := data_model.FileEntry{}
	db.Limit(1).Find(&entry, "url = ?", info.URL)
	if entry.FileName == "" {
		return ""
	}

	outputName := info.GetChapterOutputPath(entry.FileName)
	_, err := os.Stat(outputName)
	if err != nil {
		return ""
	}

	return entry.FileName
}

// Makes a context variable with necessary download state infomation in it.
func makeChapterPageContext(info ChapterInfo, resultChan chan PageContent, retryCnt int64) *colly.Context {
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

	onResponse := func(resp *colly.Response) {
		global := resp.Ctx.GetAny("global").(*CtxGlobal)
		respState := resp.Ctx.GetAny("downloadState").(*ChapterDownloadState)
		global.Link.MarkVisited(respState.Info.VolIndex, respState.Info.ChapIndex)
	}

	onError := func(resp *colly.Response, err error) {
		leftRetryCnt := resp.Ctx.GetAny("leftRetryCnt").(int64)
		if leftRetryCnt <= 0 {
			resultChan <- PageContent{
				Err: fmt.Errorf("failed after all retry: %s", err),
			}
			close(resultChan)
			return
		}

		resp.Ctx.Put("leftRetryCnt", leftRetryCnt-1)
		if err = resp.Request.Retry(); err != nil {
			resultChan <- PageContent{
				Err: fmt.Errorf("unable to retry request: %s", err),
			}
			close(resultChan)
		}

		// signaling continuation
		resultChan <- PageContent{}
	}

	ctx := colly.NewContext()
	ctx.Put("downloadState", &state)
	ctx.Put("leftRetryCnt", retryCnt)
	ctx.Put("onResponse", colly.ResponseCallback(onResponse))
	ctx.Put("onError", colly.ErrorCallback(onError))

	return ctx
}

type WaitPagesResult struct {
	PageList       *list.List
	Title          string
	NextChapterURL string // An absolute URL to the first page of next chapter
	Err            error
}

// Collects all pages sent from colly jobs with timeout.
func waitPages(title string, timeout time.Duration, resultChan chan PageContent) WaitPagesResult {
	pageList := list.New()
	pageList.Init()

	waitResult := WaitPagesResult{
		PageList: pageList,
		Title:    title,
	}

loop:
	for {
		select {
		case data, ok := <-resultChan:
			if !ok {
				break loop
			}

			if data.Err != nil {
				waitResult.Err = data.Err
				continue
			}

			if data.Content != "" {
				insertChapterPage(pageList, data)
			}

			if data.Title != "" {
				waitResult.Title = data.Title
			}

			if data.NextChapterURL != "" {
				waitResult.NextChapterURL = data.NextChapterURL
			}
		case <-time.After(timeout):
			waitResult.Err = fmt.Errorf("download timeout after %s", timeout.String())
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

	log.Warnf("failed to download %s: %s", info.GetLogName(info.Title), err)
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
func saveChapterFileEntry(db *gorm.DB, saveTo string, info *ChapterInfo, fileTitle string) {
	if db != nil {
		entry := data_model.FileEntry{
			URL:      info.URL,
			Book:     info.Book,
			Volume:   info.VolumeInfo.Title,
			FileName: fileTitle,
		}

		db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
	}
}

// tryGoToNextChapter tries to create request for next chapter with infomation gathered
// during downloading current chapter.
func tryGoToNextChapter(r *colly.Request, timeout time.Duration, info ChapterInfo, waitResult WaitPagesResult) {
	global := r.Ctx.GetAny("global").(*CtxGlobal)

	var nextVolInfo *VolumeInfo
	nextVolIndex := info.VolIndex
	nextChapIndex := info.ChapIndex + 1
	nextURL := waitResult.NextChapterURL

	if nextURL == "" {
		// pass
		nextVolIndex = info.VolIndex + 1
		nextChapIndex = 1
	} else if info.ChapIndex >= info.TotalChapterCnt {
		// this is the last chapter of current volume
		nextVolIndex = info.VolIndex + 1
		nextChapIndex = 1

		nextVolInfo = global.Link.GetAndRemoveVolInfo(nextVolIndex, nextChapIndex)
		if nextVolInfo != nil {
			log.Infof("reuse stored volume info: Vol.%03d - %s", nextVolInfo.VolIndex, nextVolInfo.Title)
		}
	} else {
		nextVolInfo = &info.VolumeInfo
	}

	if nextVolInfo == nil {
		global.Link.SetURL(nextVolIndex, nextChapIndex, nextURL)
	} else if !global.Link.CheckVisited(nextVolInfo.VolIndex, nextChapIndex) {
		CollectChapterPages(r, timeout, ChapterInfo{
			VolumeInfo: *nextVolInfo,
			ChapIndex:  nextChapIndex,
			URL:        nextURL,
		})
	}
}

type ImageTask struct {
	Ctx        context.Context
	URL        string
	OutputName string
}

type ImgDlWorkerFunc = func(collator *colly.Collector, task ImageTask, resultChan chan bool)

// StartImageDlWorker starts a new goroutine waiting for in coming download tasks.
// And returns a channel for submitting new image task. When all tasks has been
// submitted, task channel should be closed.
// After all task are handled, background goroutine will close chapter result channel,
func StartImageDlWorker(collector *colly.Collector, chapterLogName string, pageResultChan chan PageContent, dlFunc ImgDlWorkerFunc) chan ImageTask {
	taskChan := make(chan ImageTask, 5)
	dlResultChan := make(chan bool, 5)

	lock := sync.Mutex{}
	taskCnt := 0
	taskClosed := false

	go func() {
		for task := range taskChan {
			lock.Lock()
			taskCnt++
			lock.Unlock()

			dlFunc(collector, task, dlResultChan)
		}

		lock.Lock()
		taskClosed = true
		lock.Unlock()

		dlResultChan <- true
	}()

	go func() {
		finishedCnt := 0
		allOk := true

		for dlOk := range dlResultChan {
			allOk = allOk && dlOk
			finishedCnt++
			pageResultChan <- PageContent{}

			lock.Lock()
			isEnded := taskClosed && finishedCnt >= taskCnt
			lock.Unlock()

			if isEnded {
				break
			}
		}

		var finalErr error
		if !allOk {
			finalErr = fmt.Errorf("failed to complete %s", chapterLogName)
		}

		pageResultChan <- PageContent{
			Err: finalErr,
		}

		close(pageResultChan)
	}()

	return taskChan
}

// GetImageDlWorkerChanFromCtx retrives image download task channel from request
// context. If such channel has not been saved to context, this function will
// start a worker goroutine and save the channel binded to that goroutine to
// context before returning it.
func GetImageDlWorkerChanFromCtx(ctx *colly.Context, key string, dlFunc ImgDlWorkerFunc) chan ImageTask {
	global := ctx.GetAny("global").(*CtxGlobal)
	state := ctx.GetAny("downloadState").(*ChapterDownloadState)

	dlChan, getOk := ctx.GetAny(key).(chan ImageTask)
	if !getOk {
		logName := state.Info.GetLogName(state.Info.Title)
		dlChan = StartImageDlWorker(global.Collector, logName, state.ResultChan, dlFunc)
		ctx.Put(key, dlChan)
	}

	return dlChan
}
