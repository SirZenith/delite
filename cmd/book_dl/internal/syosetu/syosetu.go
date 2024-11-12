package syosetu

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/database/data_model"
	"github.com/SirZenith/delite/network"
	collect "github.com/SirZenith/delite/page_collect"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"gorm.io/gorm/clause"
)

const defaultDelay = 50
const defaultTimeOut = 10_000

// Setups collector callbacks for collecting novel content from desktop novel page.
func SetupCollector(c *colly.Collector, target collect.DlTarget) error {
	if len(target.Options.LimitRules) > 0 {
		c.Limits(target.Options.LimitRules)
	} else {
		c.Limit(&colly.LimitRule{
			DomainGlob: "*.syosetu.com",
			Delay:      defaultDelay * time.Millisecond,
		})
	}

	timeout := common.GetDurationOr(target.Options.Timeout, defaultTimeOut)
	c.SetRequestTimeout(timeout * time.Millisecond)

	c.OnHTML("article.p-novel", onNovelPage)

	return nil
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

	novelContents := e.DOM.Find("div.p-novel__text")
	if len(novelContents.Nodes) > 0 {
		onPageContent(e.Request, novelContents)
	}
}

// ----------------------------------------------------------------------------
// Episode list

func onEpisodeList(req *colly.Request, episodeList *goquery.Selection) {
	ctx := req.Ctx
	global := ctx.GetAny("global").(*collect.CtxGlobal)

	record, ok := ctx.GetAny("volumeInfo").(volumeRecord)
	if !ok {
		record = volumeRecord{}
	}

	chapterList := []collect.ChapterInfo{}

	episodeList.Children().Filter("div").Each(func(_ int, child *goquery.Selection) {
		cls, _ := child.Attr("class")

		switch cls {
		case "p-eplist__chapter-title":
			// new volume
			if len(chapterList) > 0 {
				onVolumeEntry(req, record, chapterList, global)
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

				chapterList = append(chapterList, collect.ChapterInfo{
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
		onVolumeEntry(req, record, chapterList, global)
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

	global := req.Ctx.GetAny("global").(*collect.CtxGlobal)
	global.Collector.Request("GET", url, nil, newCtx, req.Headers.Clone())
}

// Handles one volume block found in desktop volume list.
func onVolumeEntry(r *colly.Request, record volumeRecord, chapterList []collect.ChapterInfo, global *collect.CtxGlobal) {
	volumeInfo := makeVolumeInfo(record, global.Target)
	os.MkdirAll(volumeInfo.OutputDir, 0o777)

	if record.chapterOffset == 0 {
		log.Infof("volume %d: %s", record.volIndex, volumeInfo.Title)
	}

	timeout := common.GetDurationOr(global.Target.Options.Timeout, defaultTimeOut)

	volumeInfo.TotalChapterCnt = len(chapterList)

	for _, chapter := range chapterList {
		chapter.VolumeInfo = volumeInfo
		go collect.CollectChapterPages(r, timeout*time.Millisecond, chapter)
	}
}

// Extracts volume info from desktop page element.
func makeVolumeInfo(record volumeRecord, target *collect.DlTarget) collect.VolumeInfo {
	outputTitle := common.InvalidPathCharReplace(record.title)
	if outputTitle == "" {
		outputTitle = fmt.Sprintf("Vol.%03d", record.volIndex)
	} else {
		outputTitle = fmt.Sprintf("%03d - %s", record.volIndex, outputTitle)
	}

	return collect.VolumeInfo{
		Book:     target.Title,
		VolIndex: record.volIndex,
		Title:    record.title,

		OutputDir:    filepath.Join(target.OutputDir, outputTitle),
		ImgOutputDir: filepath.Join(target.ImgOutputDir, outputTitle),
	}
}

// ----------------------------------------------------------------------------
// Chapter content

// Handles novel chapter content page encountered during collecting.
func onPageContent(req *colly.Request, novelContents *goquery.Selection) {
	ctx := req.Ctx
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	content := getContentText(novelContents)
	page := collect.PageContent{
		PageNumber: state.CurPageNumber,
		Content:    content,
	}

	downloadChapterImages(req, novelContents)

	state.ResultChan <- page

	close(state.ResultChan)
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
func getContentText(containers *goquery.Selection) string {
	buffer := []string{}

	containers.Each(func(_ int, container *goquery.Selection) {
		container.Children().Each(func(_ int, child *goquery.Selection) {
			if html, err := goquery.OuterHtml(child); err == nil {
				buffer = append(buffer, html)
			}
		})
	})

	return strings.Join(buffer, "\n")
}

// Downloads all illustrations found in given chapter content page.
func downloadChapterImages(req *colly.Request, containers *goquery.Selection) {
	ctx := req.Ctx
	global := ctx.GetAny("global").(*collect.CtxGlobal)
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	outputDir := state.Info.ImgOutputDir
	if err := os.MkdirAll(outputDir, 0o777); err != nil {
		log.Errorf("failed to create imge output directory %s: %s", outputDir, err)
		return
	}

	containers.Find("img").Each(func(_ int, img *goquery.Selection) {
		url, _ := img.Attr("data-src")
		if url == "" {
			url, _ = img.Attr("src")
		}

		if url == "" {
			return
		}

		url = req.AbsoluteURL(url)

		basename := common.ReplaceFileExt(path.Base(url), ".png")
		outputName := filepath.Join(outputDir, basename)
		if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
			log.Debugf("skip image: Vol.%03d - Chap.%04d - %s", state.Info.VolIndex, state.Info.ChapIndex, basename)
			return
		}

		if global.Db != nil {
			entry := data_model.FileEntry{
				URL:      url,
				Book:     state.Info.Book,
				Volume:   state.Info.Title,
				FileName: basename,
			}
			global.Db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		}

		dlContext := colly.NewContext()
		dlContext.Put("onResponse", network.MakeSaveImageBodyCallback(outputName, common.ImageFormatPng))

		global.Collector.Request("GET", url, nil, dlContext, map[string][]string{
			"Referer": {"https://ncode.syosetu.com/"},
		})
	})
}
