package book_dl

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

// Setups collector callbacks for collecting novel content from desktop novel page.
func desktopSetupCollector(c *colly.Collector, options options) {
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.linovelib.com",
		Delay:      options.requestDelay,
	})

	c.OnHTML("div#volume-list", desktopOnVolumeList)
	c.OnHTML("div.mlfy_main", desktopOnPageContent)
}

// Handles volume list found on novel's desktop TOC page.
func desktopOnVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.volume", desktopOnVolumeEntry)
}

// Handles one volume block found in desktop volume list.
func desktopOnVolumeEntry(volIndex int, e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	options := ctx.GetAny("options").(*options)

	fmt.Println("volume", volIndex)

	volumeInfo := desktopGetVolumeInfo(volIndex, e, options)
	os.MkdirAll(volumeInfo.outputDir, 0o755)

	e.ForEach("ul.chapter-list li a", func(chapIndex int, e *colly.HTMLElement) {
		desktopOnChapterEntry(chapIndex, e, volumeInfo)
	})
}

// Extracts volume info from desktop page element.
func desktopGetVolumeInfo(volIndex int, _ *colly.HTMLElement, options *options) volumeInfo {
	// TODO: add acutal implementation
	title := ""

	var outputTitle string
	if title == "" {
		outputTitle = fmt.Sprintf("Vol.%03d", volIndex+1)
	} else {
		outputTitle = fmt.Sprintf("%03d - %s", volIndex+1, title)
	}

	return volumeInfo{
		title:        title,
		outputDir:    filepath.Join(options.outputDir, outputTitle),
		imgOutputDir: filepath.Join(options.imgOutputDir, outputTitle),
	}
}

// Handles one chapter link found in desktop chapter entry.
func desktopOnChapterEntry(chapIndex int, e *colly.HTMLElement, volumeInfo volumeInfo) {
	title := strings.TrimSpace(e.Text)
	url := e.Attr("href")

	var outputTitle string
	if title == "" {
		outputTitle = fmt.Sprintf("Chap.%04d.html", chapIndex+1)
	} else {
		outputTitle = fmt.Sprintf("%04d - %s.html", chapIndex+1, title)
	}

	collectChapterPages(e, chapterInfo{
		url:          url,
		title:        title,
		outputName:   filepath.Join(volumeInfo.outputDir, outputTitle),
		imgOutputDir: volumeInfo.imgOutputDir,
	})
}

func desktopOnPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	result := ctx.GetAny("resultChannel").(chan pageContent)
	pageNumber := ctx.GetAny("pageNumber").(int)

	content := desktopGetContentText(e)
	isFinished := desktopCheckChapterIsFinished(e)

	desktopDownloadChapterImages(e)

	result <- pageContent{
		pageNumber: pageNumber,
		content:    content,
		isFinished: isFinished,
	}

	if !isFinished {
		desktopRequestNextPage(e)
	}
}

func desktopGetContentText(e *colly.HTMLElement) string {
	container := e.DOM.Find("div#TextContent")
	children := container.Children().Not("div.dag")
	segments := children.Map(func(_ int, child *goquery.Selection) string {
		if html, err := goquery.OuterHtml(child); err == nil {
			return html
		}
		return ""
	})
	return strings.Join(segments, "\n")
}

func desktopCheckChapterIsFinished(e *colly.HTMLElement) bool {
	isFinished := true

	footer := e.DOM.NextAll().Filter("div.mlfy_page").First()
	footer.Children().EachWithBreak(func(_ int, element *goquery.Selection) bool {
		text := strings.TrimSpace(element.Text())
		if text == nextPageTextSC || text == nextPageTextTC {
			isFinished = false
		}

		return isFinished
	})

	return isFinished
}

func desktopDownloadChapterImages(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	options := ctx.GetAny("options").(*options)
	collector := ctx.GetAny("collector").(*colly.Collector)

	if err := os.MkdirAll(options.imgOutputDir, 0o755); err != nil {
		log.Printf("failed to create imge output directory %s: %s", options.imgOutputDir, err)
		return
	}

	e.ForEach("div#TextContent img", func(_ int, img *colly.HTMLElement) {
		var url = img.Attr("data-src")
		if url == "" {
			url = img.Attr("src")
		}

		if url == "" {
			return
		}

		// TODO: group image output by volume
		basename := path.Base(url)
		outputName := filepath.Join(options.imgOutputDir, basename)
		if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
			log.Println("file already exists, skip:", outputName)
		}

		dlContext := colly.NewContext()
		dlContext.Put("dlFileTo", outputName)

		collector.Request("GET", url, nil, dlContext, map[string][]string{
			"Referer": {"https://www.linovelib.com/"},
		})
	})
}

func desktopRequestNextPage(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	pageNumber := ctx.GetAny("pageNumber").(int)

	nextPage := pageNumber + 1
	ctx.Put("pageNumber", nextPage)

	dir := path.Dir(e.Request.URL.Path)
	nextFile := ctx.Get("chapterRootStem") + "_" + strconv.Itoa(nextPage) + ctx.Get("chapterRootExt")
	nextUrl := path.Join(dir, nextFile)
	e.Request.Visit(nextUrl)
}
