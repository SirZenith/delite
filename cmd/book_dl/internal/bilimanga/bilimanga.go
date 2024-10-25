package bilimanga

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/network"
	collect "github.com/SirZenith/delite/page_collect"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
)

const defaultDelay = 1500
const defaultImgDelay = 125
const defaultTimeOut = 10_000

var patternNextChapterParam = regexp.MustCompile(`url_next:\s*'(.+?)'`)

// Setups collector callbacks for collecting manga content.
func SetupCollector(c *colly.Collector, target collect.DlTarget) error {
	delay := common.GetDurationOr(target.Options.RequestDelay, defaultDelay)
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.bilimanga.net",
		Delay:      delay * time.Millisecond,
	})

	imgDelay := common.GetDurationOr(target.Options.ImgRequestDelay, defaultImgDelay)
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.motiezw.com",
		Delay:      imgDelay * time.Millisecond,
	})

	timeout := common.GetDurationOr(target.Options.RequestDelay, defaultTimeOut)

	c.SetRequestTimeout(timeout * time.Millisecond)
	c.OnHTML("div#volumes", onVolumeList)
	c.OnHTML("div.apage", onPageContent)

	return nil
}

// Handles volume list found on novel's desktop TOC page.
func onVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.catalog-volume", onVolumeEntry)
}

// Handles one volume block found in desktop volume list.
func onVolumeEntry(volIndex int, e *colly.HTMLElement) {
	global := e.Request.Ctx.GetAny("global").(*collect.CtxGlobal)

	volumeInfo := getVolumeInfo(volIndex+1, e, global.Target)
	os.MkdirAll(volumeInfo.OutputDir, 0o755)

	log.Infof("volume %d: %s", volumeInfo.VolIndex, volumeInfo.Title)

	chapterList := []*colly.HTMLElement{}
	e.ForEach("ul.volume-chapters li.chapter-li.jsChapter a", func(chapIndex int, e *colly.HTMLElement) {
		chapterList = append(chapterList, e)
	})

	volumeInfo.TotalChapterCnt = len(chapterList)

	for chapIndex, chapter := range chapterList {
		onChapterEntry(chapIndex+1, chapter, volumeInfo)
	}
}

// Extracts volume info from desktop page element.
func getVolumeInfo(volIndex int, e *colly.HTMLElement, target *collect.DlTarget) collect.VolumeInfo {
	title := e.DOM.Find("li.chapter-bar h3").Text()
	title = strings.TrimSpace(title)

	outputTitle := common.InvalidPathCharReplace(title)
	if outputTitle == "" {
		outputTitle = fmt.Sprintf("Vol.%03d", volIndex)
	} else {
		outputTitle = fmt.Sprintf("%03d - %s", volIndex, outputTitle)
	}

	return collect.VolumeInfo{
		VolIndex: volIndex,
		Title:    title,

		OutputDir:    filepath.Join(target.OutputDir, outputTitle),
		ImgOutputDir: filepath.Join(target.ImgOutputDir, outputTitle),
	}
}

// Handles one chapter link found in desktop chapter entry.
func onChapterEntry(chapIndex int, e *colly.HTMLElement, volumeInfo collect.VolumeInfo) {
	global := e.Request.Ctx.GetAny("global").(*collect.CtxGlobal)

	title := strings.TrimSpace(e.Text)
	url := e.Attr("href")
	url = e.Request.AbsoluteURL(url)

	timeout := common.GetDurationOr(global.Target.Options.Timeout, defaultTimeOut)
	timeout *= time.Duration(global.Target.Options.RetryCnt)

	collect.CollectChapterPages(e.Request, timeout*time.Millisecond, collect.ChapterInfo{
		VolumeInfo: volumeInfo,
		ChapIndex:  chapIndex,
		Title:      title,

		URL: url,
	})
}

// Handles novel chapter content page encountered during collecting.
func onPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	page := collect.PageContent{
		PageNumber:     state.CurPageNumber,
		Content:        getContentText(e),
		NextChapterURL: getNextChapterURL(e),
	}

	if state.CurPageNumber == 1 {
		page.Title = getChapterTitle(e)
	}

	state.ResultChan <- page

	if checkChapterIsFinished(e) {
		close(state.ResultChan)
	} else {
		requestNextPage(e)
	}
}

// Extracts chapter title from page element.
func getChapterTitle(e *colly.HTMLElement) string {
	title := e.DOM.Find("div.atitle #atitle").First().Text()
	title = strings.TrimSpace(title)
	return title
}

// Extracts chapter content from page element.
// This function will do text decypher by font descramble map before returning
// page content.
func getContentText(e *colly.HTMLElement) string {
	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*collect.CtxGlobal)
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	outputDir := state.Info.ImgOutputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Errorf("failed to create imge output directory %s: %s", outputDir, err)
		return ""
	}

	container := e.DOM.Find("div#acontentz")
	children := container.Children().Filter("img[src]")

	children.Each(func(imgIndex int, child *goquery.Selection) {
		src, ok := child.Attr("data-src")
		if !ok {
			src, _ = child.Attr("src")
		}

		if src == "" {
			return
		}

		url := e.Request.AbsoluteURL(src)

		ext := path.Ext(src)
		basename := fmt.Sprintf("%04d - %03d%s", state.Info.ChapIndex, imgIndex+1, ext)
		outputName := filepath.Join(outputDir, basename)

		downloadImage(global.Collector, url, outputName)
	})

	return ""
}

// Checks if given chapter page element is the last page of this chapter. If
func checkChapterIsFinished(_ *colly.HTMLElement) bool {
	isFinished := true

	return isFinished
}

// Looks for anchor pointing to page of next chapter, if found, return it's href.
func getNextChapterURL(e *colly.HTMLElement) string {
	href := ""

	node := e.DOM
	root := node.Parents().Last()
	if len(root.Nodes) == 0 {
		root = node
	}

	script := root.Find("body#aread script").First().Text()
	match := patternNextChapterParam.FindStringSubmatch(script)
	if len(match) == 2 {
		href = e.Request.AbsoluteURL(match[1])
	}

	return href
}

// downloadImage downloads data from given url to given path.
func downloadImage(collator *colly.Collector, url, outputName string) {
	if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
		log.Infof("skip image: %s", outputName)
		return
	}

	dlContext := colly.NewContext()
	dlContext.Put("onResponse", network.MakeSaveBodyCallback(outputName))
	dlContext.Put("onError", colly.ErrorCallback(func(resp *colly.Response, err error) {
		log.Warnf("failed to download %s:\n\t%s - %s", outputName, url, err)
	}))

	collator.Request("GET", url, nil, dlContext, map[string][]string{
		"Accept":          {"image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5"},
		"Accept-Encoding": {"deflate, br, zstd"},
		"Accept-Language": {"zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2"},
		"Connection":      {"keep-alive"},
		"Host":            {"w.motiezw.com"},
		"Priority":        {"u=5, i"},
		"Referer":         {"https://www.bilimanga.net/"},
		"Sec-Fetch-Dest":  {"image"},
		"Sec-Fetch-Mode":  {"no-cors"},
		"Sec-Fetch-Site":  {"cross-site"},
	})
}

// Makes a new collect request to next page of given chapter page.
func requestNextPage(_ *colly.HTMLElement) {}
