package bilimanga

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/database/data_model"
	collect "github.com/SirZenith/delite/page_collect"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultDelay = 1500
const defaultImgDelay = 125
const defaultTimeOut = 10_000

const keyImgDlWorkerChan = "imageDlChan"

var patternNextChapterParam = regexp.MustCompile(`url_next:\s*'(.+?)'`)

// Setups collector callbacks for collecting manga content.
func SetupCollector(c *colly.Collector, target collect.DlTarget) error {
	delay := common.GetDurationOr(target.Options.RequestDelay, defaultDelay)
	imgDelay := common.GetDurationOr(target.Options.ImgRequestDelay, defaultImgDelay)

	c.Limits([]*colly.LimitRule{
		{
			DomainGlob: "*.bilimanga.net",
			Delay:      delay * time.Millisecond,
		},
		{
			DomainGlob: "*.motiezw.com",
			Delay:      imgDelay * time.Millisecond,
		},
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
		Book:     target.Title,
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
	global := ctx.GetAny("global").(*collect.CtxGlobal)
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	dlChan := collect.GetImageDlWorkerChanFromCtx(ctx, keyImgDlWorkerChan, downloadImage)

	content, tasks := getContentText(e)
	page := collect.PageContent{
		PageNumber:     state.CurPageNumber,
		Content:        content,
		NextChapterURL: getNextChapterURL(e),
	}

	if state.CurPageNumber == 1 {
		page.Title = getChapterTitle(e)
	}

	state.ResultChan <- page

	if tasks != nil {
		taskCtx := context.WithValue(context.Background(), "db", global.Db)
		taskCtx = context.WithValue(taskCtx, "book", state.Info.Book)
		taskCtx = context.WithValue(taskCtx, "volume", state.Info.Title)

		for _, task := range tasks {
			task.Ctx = taskCtx
			dlChan <- task
		}
	}

	close(dlChan)
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
func getContentText(e *colly.HTMLElement) (string, []collect.ImageTask) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	outputDir := state.Info.ImgOutputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Errorf("failed to create imge output directory %s: %s", outputDir, err)
		return "", nil
	}

	container := e.DOM.Find("div#acontentz")
	children := container.Children().Filter("img[src]")
	segments := []string{}
	tasks := []collect.ImageTask{}

	children.Each(func(imgIndex int, child *goquery.Selection) {
		src, ok := child.Attr("data-src")
		if !ok {
			src, _ = child.Attr("src")
		}

		if src == "" {
			return
		}

		url := e.Request.AbsoluteURL(src)

		basename := fmt.Sprintf("%04d - %03d.%s", state.Info.ChapIndex, imgIndex+1, common.ImageFormatAvif)
		outputName := filepath.Join(outputDir, basename)

		child.SetAttr("src", url)
		child.SetAttr("data-src", url)
		if html, err := goquery.OuterHtml(child); err == nil {
			segments = append(segments, html)
		}

		tasks = append(tasks, collect.ImageTask{
			URL:        url,
			OutputName: outputName,
		})
	})

	return strings.Join(segments, "\n"), tasks
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
func downloadImage(collator *colly.Collector, task collect.ImageTask, resultChan chan bool) {
	urlStr := task.URL
	outputName := task.OutputName

	if _, err := os.Stat(outputName); err == nil {
		log.Infof("skip image: %s", outputName)
		resultChan <- true
		return
	}

	dlContext := colly.NewContext()
	dlContext.Put("onResponse", colly.ResponseCallback(func(resp *colly.Response) {
		saveImageEntryInfo(task)

		err := common.SaveImageAs(resp.Body, outputName, common.ImageFormatAvif)
		if err == nil {
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
		"Accept-Language": {"zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2"},
		"Connection":      {"keep-alive"},
		"Host":            {host},
		"Priority":        {"u=5, i"},
		"Referer":         {"https://www.bilimanga.net/"},
		"Sec-Fetch-Dest":  {"image"},
		"Sec-Fetch-Mode":  {"no-cors"},
		"Sec-Fetch-Site":  {"cross-site"},
	})
}

func saveImageEntryInfo(task collect.ImageTask) {
	db := task.Ctx.Value("db").(*gorm.DB)
	if db == nil {
		return
	}

	book := task.Ctx.Value("book").(string)
	volume := task.Ctx.Value("volume").(string)

	entry := data_model.FileEntry{
		URL:      task.URL,
		Book:     book,
		Volume:   volume,
		FileName: filepath.Base(task.OutputName),
	}
	db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
}
