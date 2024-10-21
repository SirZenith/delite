package syosetu

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/SirZenith/bilinovel/base"
	"github.com/SirZenith/bilinovel/book_dl/internal/common"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
)

const defaultDelay = 50
const defaultTimeOut = 30_000

// Setups collector callbacks for collecting novel content from desktop novel page.
func SetupCollector(c *colly.Collector, options common.Options) {
	delay := options.RequestDelay
	if delay < 0 {
		delay = defaultDelay
	}

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*.syosetu.com",
		Delay:       time.Duration(delay) * time.Millisecond,
		Parallelism: 5,
	})

	c.OnHTML("article.p-novel", onNovelPage)
}

// A struct used to pass volume information between different TOC page content
// handling callbacks.
type volumeRecord struct {
	volIndex      int
	title         string
	chapterOffset int // how many chapters has been handled before current callback
}

func onNovelPage(e *colly.HTMLElement) {
	episodeList := e.DOM.Find("div.p-eplist").First()
	if len(episodeList.Nodes) > 0 {
		onEpisodeList(e.Request, episodeList)
	}

	novelContent := e.DOM.Find("div.p-novel__text").First()
	if len(novelContent.Nodes) > 0 {
		onPageContent(e.Request, novelContent)
	}
}

// ----------------------------------------------------------------------------
// Episode list

func onEpisodeList(req *colly.Request, episodeList *goquery.Selection) {
	ctx := req.Ctx
	options := ctx.GetAny("options").(*common.Options)

	record, ok := ctx.GetAny("volumeInfo").(volumeRecord)
	if !ok {
		record = volumeRecord{}
	}

	chapterList := []common.ChapterInfo{}

	episodeList.Children().Filter("div").Each(func(_ int, child *goquery.Selection) {
		cls, _ := child.Attr("class")

		switch cls {
		case "p-eplist__chapter-title":
			// new volume
			if len(chapterList) > 0 {
				onVolumeEntry(req, record, chapterList, options)
			}

			title := child.Text()
			title = strings.TrimSpace(title)

			record = volumeRecord{
				volIndex: record.volIndex + 1,
				title:    title,
			}
			chapterList = chapterList[:0]
		case "p-eplist__sublist":
			// new chapter
			aTag := child.Find("a.p-eplist__subtitle[href]").First()
			if url, ok := aTag.Attr("href"); ok {
				title := aTag.Text()
				title = strings.TrimSpace(title)

				chapterList = append(chapterList, common.ChapterInfo{
					ChapIndex: record.chapterOffset + len(chapterList) + 1,
					Title:     title,
					URL:       req.AbsoluteURL(url),
				})
			}
		}
	})

	// handling left over chapters
	letftCnt := len(chapterList)
	if letftCnt > 0 {
		onVolumeEntry(req, record, chapterList, options)
	}
	record.chapterOffset = letftCnt

	tryGoToNextEpisodeListPage(req, episodeList, record)
}

func tryGoToNextEpisodeListPage(req *colly.Request, episodeList *goquery.Selection, record volumeRecord) {
	pager := episodeList.Siblings().Filter("div.c-pager").First()
	aTag := pager.Find("a.c-pager__item.c-pager__item--next").First()
	href, ok := aTag.Attr("href")
	if !ok {
		return
	}

	url := req.AbsoluteURL(href)

	newCtx := colly.NewContext()
	newCtx.Put("volumeInfo", record)

	collector := req.Ctx.GetAny("collector").(*colly.Collector)
	collector.Request("GET", url, nil, newCtx, req.Headers.Clone())
}

// Handles one volume block found in desktop volume list.
func onVolumeEntry(r *colly.Request, record volumeRecord, chapterList []common.ChapterInfo, options *common.Options) {
	volumeInfo := makeVolumeInfo(record, options)
	os.MkdirAll(volumeInfo.OutputDir, 0o755)

	if record.chapterOffset == 0 {
		log.Infof("volume %d: %s", record.volIndex, volumeInfo.Title)
	}

	volumeInfo.TotalChapterCnt = len(chapterList)

	timeout := options.Timeout
	if timeout < 0 {
		timeout = defaultTimeOut
	}

	for _, chapter := range chapterList {
		chapter.VolumeInfo = volumeInfo
		go common.CollectChapterPages(r, timeout, chapter)
	}
}

// Extracts volume info from desktop page element.
func makeVolumeInfo(record volumeRecord, options *common.Options) common.VolumeInfo {
	outputTitle := base.InvalidPathCharReplace(record.title)
	if outputTitle == "" {
		outputTitle = fmt.Sprintf("Vol.%03d", record.volIndex)
	} else {
		outputTitle = fmt.Sprintf("%03d - %s", record.volIndex, outputTitle)
	}

	return common.VolumeInfo{
		VolIndex: record.volIndex,
		Title:    record.title,

		OutputDir:    filepath.Join(options.OutputDir, outputTitle),
		ImgOutputDir: filepath.Join(options.ImgOutputDir, outputTitle),
	}
}

// ----------------------------------------------------------------------------
// Chapter content

// Handles novel chapter content page encountered during collecting.
func onPageContent(req *colly.Request, novelContent *goquery.Selection) {
	ctx := req.Ctx
	state := ctx.GetAny("downloadState").(*common.ChapterDownloadState)

	content := getContentText(novelContent)
	page := common.PageContent{
		PageNumber: state.CurPageNumber,
		Content:    content,
		IsFinished: true,
	}

	state.ResultChan <- page
}

// Extracts chapter title from page element.
func getChapterTitle(e *colly.HTMLElement) string {
	title := e.DOM.Find("#mlfy_main_text h1").First().Text()
	title = strings.TrimSpace(title)
	return title
}

// Extracts chapter content from page element.
// This function will do text decypher by font descramble map before returning
// page content.
func getContentText(container *goquery.Selection) string {
	children := container.Children()
	segments := children.Map(func(_ int, child *goquery.Selection) string {
		if html, err := goquery.OuterHtml(child); err == nil {
			return html
		}
		return ""
	})
	return strings.Join(segments, "\n")
}
