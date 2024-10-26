package senmanga

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/SirZenith/delite/common"
	collect "github.com/SirZenith/delite/page_collect"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
)

const defaultDelay = 50
const defaultImgDelay = 125
const defaultTimeOut = 10_000

const keyImgDlWorkerChan = "imageDlChan"

var patternNextChapterParam = regexp.MustCompile(`url_next:\s*'(.+?)'`)

// Setups collector callbacks for collecting manga content.
func SetupCollector(c *colly.Collector, target collect.DlTarget) error {
	c.Limits([]*colly.LimitRule{
		{
			DomainGlob:  "*.senmanga.com",
			Parallelism: 5,
		},
		{
			DomainGlob:  "*.kumacdn.club",
			Parallelism: 5,
		},
	})

	timeout := common.GetDurationOr(target.Options.RequestDelay, defaultTimeOut)

	c.SetRequestTimeout(timeout * time.Millisecond)
	c.OnHTML("body div.container div.content", onVolumeList)
	c.OnHTML("div.reader.text-center", onPageContent)

	return nil
}

// Handles volume list found on novel's desktop TOC page.
func onVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.widget ul.chapter-list", onVolumeEntry)
}

// Handles one volume block found in desktop volume list.
func onVolumeEntry(volIndex int, e *colly.HTMLElement) {
	global := e.Request.Ctx.GetAny("global").(*collect.CtxGlobal)

	volumeInfo := getVolumeInfo(volIndex+1, e, global.Target)
	os.MkdirAll(volumeInfo.OutputDir, 0o755)

	log.Infof("volume %d: %s", volumeInfo.VolIndex, volumeInfo.Title)

	chapterList := []*colly.HTMLElement{}
	e.ForEach("li>a.series", func(chapIndex int, e *colly.HTMLElement) {
		chapterList = append(chapterList, e)
	})

	totalCnt := len(chapterList)
	volumeInfo.TotalChapterCnt = totalCnt

	for i := totalCnt - 1; i >= 0; i-- {
		onChapterEntry(totalCnt-i, chapterList[i], volumeInfo)
	}
}

// Extracts volume info from desktop page element.
func getVolumeInfo(volIndex int, _ *colly.HTMLElement, target *collect.DlTarget) collect.VolumeInfo {
	outputTitle := fmt.Sprintf("Vol.%03d", volIndex)

	return collect.VolumeInfo{
		VolIndex: volIndex,

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

	dlChan := collect.GetImageDlWorkerChanFromCtx(ctx, keyImgDlWorkerChan, downloadImage)

	content, tasks := getContentText(e)
	nextPageURL, nextChapterURL := getNextURL(e)

	state.ResultChan <- collect.PageContent{
		PageNumber:     state.CurPageNumber,
		Content:        content,
		NextChapterURL: nextChapterURL,
	}

	if tasks != nil {
		for _, task := range tasks {
			dlChan <- task
		}
	}

	if nextPageURL == "" {
		close(dlChan)
	} else {
		state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)
		state.CurPageNumber++
		e.Request.Visit(nextPageURL)
	}
}

// getContentText extracts chapter content from page element.
// This function will do text decypher by font descramble map before returning
// page content.
func getContentText(e *colly.HTMLElement) (string, []collect.ImageTask) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	outputDir := state.Info.ImgOutputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Errorf("failed to create imge output directory %s: %s", outputDir, err)
		return "", nil
	}

	container := e.DOM.Find("img.picture")
	segments := []string{}
	tasks := []collect.ImageTask{}

	container.Each(func(imgIndex int, child *goquery.Selection) {
		src, ok := child.Attr("src")
		if !ok || src == "" {
			return
		}

		url := e.Request.AbsoluteURL(src)

		ext := path.Ext(src)
		basename := fmt.Sprintf("%04d - %03d - %03d%s", state.Info.ChapIndex, state.CurPageNumber, imgIndex+1, ext)
		outputName := filepath.Join(outputDir, basename)

		child.SetAttr("src", outputName)
		if html, err := goquery.OuterHtml(child); err == nil {
			segments = append(segments, html)
		}

		tasks = append(tasks, collect.ImageTask{URL: url, OutputName: outputName})
	})

	return strings.Join(segments, "\n"), tasks
}

// getNextURL tries to find URL of next page and next chapter in current page
func getNextURL(e *colly.HTMLElement) (string, string) {
	href, ok := e.DOM.Find("a").First().Attr("href")
	if !ok || href == "" {
		return "", ""
	}

	href = e.Request.AbsoluteURL(href)
	parsed, err := url.Parse(href)
	if err != nil {
		return "", ""
	}

	currentSegments := strings.Split(e.Request.URL.Path, "/")
	hrefSegments := strings.Split(parsed.Path, "/")

	lenCur := len(currentSegments)
	lenHref := len(hrefSegments)
	if lenCur > lenHref {
		return "", href
	}

	return href, ""
}

// downloadImage downloads data from given url to given path.
func downloadImage(collator *colly.Collector, task collect.ImageTask, resultChan chan bool) {
	urlStr := task.URL
	outputName := task.OutputName

	if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
		log.Infof("skip image: %s", outputName)
		resultChan <- true
		return
	}

	dlContext := colly.NewContext()
	dlContext.Put("onResponse", colly.ResponseCallback(func(resp *colly.Response) {
		if err := resp.Save(outputName); err == nil {
			log.Infof("file downloaded: %s", outputName)
			resultChan <- true
		} else {
			log.Warnf("failed to save file %s: %s\n", outputName, err)
			resultChan <- false
		}
	}))
	dlContext.Put("onError", colly.ErrorCallback(func(resp *colly.Response, err error) {
		log.Warnf("failed to download %s:\n\t%s - %s", outputName, urlStr, err)
		resultChan <- false
	}))

	host := ""
	if parsed, err := url.Parse(urlStr); err == nil {
		host = parsed.Hostname()
	}

	collator.Request("GET", urlStr, nil, dlContext, map[string][]string{
		"Accept":          {"image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5"},
		"Accept-Encoding": {"deflate, br, zstd"},
		"Connection":      {"keep-alive"},
		"Host":            {host},
		"Priority":        {"u=5, i"},
		"Referer":         {"https://raw.senmanga.com/"},
	})
}
