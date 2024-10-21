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
	"github.com/SirZenith/bilinovel/base"
	"github.com/SirZenith/bilinovel/book_dl/internal/common"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
)

const defaultDelay = 1500
const defaultTimeOut = 8000

// Setups collector callbacks for collecting content from mobile novel page.
func SetupCollector(c *colly.Collector, options common.Options) {
	delay := options.RequestDelay
	if delay < 0 {
		delay = defaultDelay
	}

	c.Limit(&colly.LimitRule{
		DomainGlob: "*.bilinovel.com",
		Delay:      time.Duration(delay) * time.Millisecond,
	})

	c.OnHTML("div#volumes", onVolumeList)
	c.OnHTML("body#aread", onPageContent)
}

func onVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.catalog-volume", onVolumeEntry)
}

// Handles one volume block found in mobile volume list.
func onVolumeEntry(volIndex int, e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	options := ctx.GetAny("options").(*common.Options)

	volumeInfo := getVolumeInfo(volIndex+1, e, options)
	os.MkdirAll(volumeInfo.OutputDir, 0o755)

	log.Infof("volume %d: %s", volumeInfo.VolIndex, volumeInfo.Title)

	e.ForEach("a.chapter-li-a", func(chapIndex int, e *colly.HTMLElement) {
		onChapterEntry(chapIndex+1, e, volumeInfo)
	})
}

// Extracts volume info from mobile page element.
func getVolumeInfo(volIndex int, e *colly.HTMLElement, options *common.Options) common.VolumeInfo {
	title := e.DOM.Find("li.chapter-bar").First().Text()
	title = strings.TrimSpace(title)

	outputTitle := base.InvalidPathCharReplace(title)
	if outputTitle == "" {
		outputTitle = fmt.Sprintf("Vol.%03d", volIndex)
	} else {
		outputTitle = fmt.Sprintf("%03d - %s", volIndex, outputTitle)
	}

	return common.VolumeInfo{
		VolIndex: volIndex,
		Title:    title,

		OutputDir:    filepath.Join(options.OutputDir, outputTitle),
		ImgOutputDir: filepath.Join(options.ImgOutputDir, outputTitle),
	}
}

// Handles one chapter link found in mobile chapter entry.
func onChapterEntry(chapIndex int, e *colly.HTMLElement, volumeInfo common.VolumeInfo) {
	options := e.Request.Ctx.GetAny("options").(*common.Options)

	timeout := options.Timeout
	if timeout < 0 {
		timeout = defaultTimeOut
	}

	title := strings.TrimSpace(e.Text)
	url := e.Attr("href")
	url = e.Request.AbsoluteURL(url)

	common.CollectChapterPages(e.Request, timeout, common.ChapterInfo{
		VolumeInfo: volumeInfo,
		ChapIndex:  chapIndex,
		Title:      title,

		URL: url,
	})
}

func onPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*common.ChapterDownloadState)

	content := getContentText(e)
	isFinished := checkChapterIsFinished(e)
	state.ResultChan <- common.PageContent{
		PageNumber: state.CurPageNumber,
		Content:    content,
		IsFinished: isFinished,
	}

	downloadChapterImages(e)

	if !isFinished {
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
		if text == common.NextPageTextSC || text == common.NextPageTextTC {
			isFinished = false
		}

		return isFinished
	})

	return isFinished
}

func downloadChapterImages(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	collector := ctx.GetAny("collector").(*colly.Collector)
	state := ctx.GetAny("downloadState").(*common.ChapterDownloadState)

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

		basename := path.Base(url)
		outputName := filepath.Join(outputDir, basename)
		if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
			log.Infof("skip: %s", outputName)
			return
		}

		dlContext := colly.NewContext()
		dlContext.Put("dlFileTo", outputName)

		collector.Request("GET", url, nil, dlContext, map[string][]string{
			"Referer": {"https://www.bilinovel.com"},
		})
	})
}

func requestNextPage(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*common.ChapterDownloadState)
	state.CurPageNumber++

	dir := path.Dir(e.Request.URL.Path)
	nextFile := fmt.Sprintf("%s_%d%s", state.RootNameStem, state.CurPageNumber, state.RootNameExt)
	nextUrl := path.Join(dir, nextFile)
	e.Request.Visit(nextUrl)
}
