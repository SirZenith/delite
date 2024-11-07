package bilinovel

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	dl_common "github.com/SirZenith/delite/cmd/book_dl/internal/common"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/database/data_model"
	"github.com/SirZenith/delite/network"
	collect "github.com/SirZenith/delite/page_collect"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"gorm.io/gorm/clause"
)

const defaultDelay = 1500
const defaultTimeOut = 10_000

// Setups collector callbacks for collecting content from mobile novel page.
func SetupCollector(c *colly.Collector, target collect.DlTarget) {
	delay := common.GetDurationOr(target.Options.RequestDelay, defaultDelay)
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.bilinovel.com",
		Delay:      delay * time.Millisecond,
	})

	timeout := common.GetDurationOr(target.Options.RequestDelay, defaultTimeOut)
	c.SetRequestTimeout(timeout * time.Millisecond)

	c.OnHTML("div#volumes", onVolumeList)
	c.OnHTML("body#aread", onPageContent)
}

func onVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.catalog-volume", onVolumeEntry)
}

// Handles one volume block found in mobile volume list.
func onVolumeEntry(volIndex int, e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*collect.CtxGlobal)

	volumeInfo := getVolumeInfo(volIndex+1, e, global.Target)
	os.MkdirAll(volumeInfo.OutputDir, 0o755)

	log.Infof("volume %d: %s", volumeInfo.VolIndex, volumeInfo.Title)

	e.ForEach("a.chapter-li-a", func(chapIndex int, e *colly.HTMLElement) {
		onChapterEntry(chapIndex+1, e, volumeInfo)
	})
}

// Extracts volume info from mobile page element.
func getVolumeInfo(volIndex int, e *colly.HTMLElement, target *collect.DlTarget) collect.VolumeInfo {
	title := e.DOM.Find("li.chapter-bar").First().Text()
	title = strings.TrimSpace(title)

	outputTitle := common.InvalidPathCharReplace(title)
	if outputTitle == "" {
		outputTitle = fmt.Sprintf("Vol.%03d", volIndex)
	} else {
		outputTitle = fmt.Sprintf("%03d - %s", volIndex, outputTitle)
	}

	return collect.VolumeInfo{
		Book:     target.Title,
		VolIndex: volIndex,
		Title:    title,

		OutputDir:    filepath.Join(target.OutputDir, outputTitle),
		ImgOutputDir: filepath.Join(target.ImgOutputDir, outputTitle),
	}
}

// Handles one chapter link found in mobile chapter entry.
func onChapterEntry(chapIndex int, e *colly.HTMLElement, volumeInfo collect.VolumeInfo) {
	global := e.Request.Ctx.GetAny("global").(*collect.CtxGlobal)

	timeout := common.GetDurationOr(global.Target.Options.Timeout, defaultTimeOut)
	timeout *= time.Duration(global.Target.Options.RetryCnt)

	title := strings.TrimSpace(e.Text)
	url := e.Attr("href")
	url = e.Request.AbsoluteURL(url)

	collect.CollectChapterPages(e.Request, timeout*time.Millisecond, collect.ChapterInfo{
		VolumeInfo: volumeInfo,
		ChapIndex:  chapIndex,
		Title:      title,

		URL: url,
	})
}

func onPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	content := getContentText(e)
	state.ResultChan <- collect.PageContent{
		PageNumber: state.CurPageNumber,
		Content:    content,
	}

	downloadChapterImages(e)

	if checkChapterIsFinished(e) {
		close(state.ResultChan)
	} else {
		requestNextPage(e)
	}
}

func getContentText(e *colly.HTMLElement) string {
	container := e.DOM.Find("div.bcontent")
	children := container.Children().Not("div.cgo")
	segments := children.Map(func(_ int, child *goquery.Selection) string {
		if html, err := goquery.OuterHtml(child); err == nil {
			return html
		}
		return ""
	})
	return strings.Join(segments, "\n")
}

// Check if given html document is the last page of a chapter.
func checkChapterIsFinished(e *colly.HTMLElement) bool {
	isFinished := true

	footer := e.DOM.Find("div#footlink")
	footer.Children().EachWithBreak(func(_ int, element *goquery.Selection) bool {
		text := element.Text()
		if text == dl_common.NextPageTextSC || text == dl_common.NextPageTextTC {
			isFinished = false
		}

		return isFinished
	})

	return isFinished
}

func downloadChapterImages(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*collect.CtxGlobal)
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	outputDir := state.Info.ImgOutputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Infof("failed to create imge output directory %s: %s", outputDir, err)
		return
	}

	e.ForEach("div.bcontent img", func(_ int, img *colly.HTMLElement) {
		var url = img.Attr("data-src")
		if url == "" {
			url = img.Attr("src")
		}

		if url == "" {
			return
		}

		basename := common.ReplaceFileExt(path.Base(url), ".png")
		if global.Db != nil {
			entry := data_model.FileEntry{
				URL:      url,
				Book:     state.Info.Book,
				Volume:   state.Info.Title,
				FileName: basename,
			}
			global.Db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		}

		outputName := filepath.Join(outputDir, basename)
		if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
			log.Infof("skip: %s", outputName)
			return
		}

		dlContext := colly.NewContext()
		dlContext.Put("onResponse", network.MakeSaveImageBodyCallback(outputName, common.ImageFormatPng))

		global.Collector.Request("GET", url, nil, dlContext, map[string][]string{
			"Referer": {"https://www.bilinovel.com"},
		})
	})
}

func requestNextPage(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)
	state.CurPageNumber++

	dir := path.Dir(e.Request.URL.Path)
	nextFile := fmt.Sprintf("%s_%d%s", state.RootNameStem, state.CurPageNumber, state.RootNameExt)
	nextUrl := path.Join(dir, nextFile)
	e.Request.Visit(nextUrl)
}
