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

// Setups collector callbacks for collecting content from mobile novel page.
func mobileSetupCollector(c *colly.Collector, options options) {
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.bilinovel.com",
		Delay:      options.requestDelay,
	})

	c.OnHTML("div#volumes", mobileOnVolumeList)
	c.OnHTML("body#aread", mobileOnPageContent)
}

func mobileOnVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.catalog-volume", mobileOnVolumeEntry)
}

// Handles one volume block found in mobile volume list.
func mobileOnVolumeEntry(volIndex int, e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	options := ctx.GetAny("options").(*options)

	fmt.Println("volume", volIndex)

	volumeInfo := mobileGetVolumeInfo(volIndex, e, options)
	os.MkdirAll(volumeInfo.outputDir, 0o755)

	e.ForEach("a.chapter-li-a", func(chapIndex int, e *colly.HTMLElement) {
		mobileOnChapterEntry(chapIndex, e, volumeInfo)
	})
}

// Extracts volume info from mobile page element.
func mobileGetVolumeInfo(volIndex int, e *colly.HTMLElement, options *options) volumeInfo {
	title := e.DOM.Find("li.chapter-bar").First().Text()
	title = strings.TrimSpace(title)

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

// Handles one chapter link found in mobile chapter entry.
func mobileOnChapterEntry(chapIndex int, e *colly.HTMLElement, volumeInfo volumeInfo) {
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

func mobileOnPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	result := ctx.GetAny("resultChannel").(chan pageContent)
	pageNumber := ctx.GetAny("pageNumber").(int)

	content := mobileGetContentText(e)
	isFinished := mobileCheckChapterIsFinished(e)

	mobileDownloadChapterImages(e)

	result <- pageContent{
		pageNumber: pageNumber,
		content:    content,
		isFinished: isFinished,
	}

	if !isFinished {
		mobileRequestNextPage(e)
	}
}

func mobileGetContentText(e *colly.HTMLElement) string {
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
func mobileCheckChapterIsFinished(e *colly.HTMLElement) bool {
	isFinished := true

	footer := e.DOM.Find("div#footlink")
	footer.Children().EachWithBreak(func(_ int, element *goquery.Selection) bool {
		text := element.Text()
		if text == nextPageTextSC || text == nextPageTextTC {
			isFinished = false
		}

		return isFinished
	})

	return isFinished
}

func mobileDownloadChapterImages(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	options := ctx.GetAny("options").(*options)
	collector := ctx.GetAny("collector").(*colly.Collector)

	if err := os.MkdirAll(options.imgOutputDir, 0o755); err != nil {
		log.Printf("failed to create imge output directory %s: %s", options.imgOutputDir, err)
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

		// TODO: group image output by volume
		basename := path.Base(url)
		outputName := filepath.Join(options.imgOutputDir, basename)
		if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
			log.Println("file already exists, skip:", outputName)
		}

		dlContext := colly.NewContext()
		dlContext.Put("dlFileTo", outputName)

		collector.Request("GET", url, nil, dlContext, map[string][]string{
			"Referer": {"https://www.bilinovel.com"},
		})
	})
}

func mobileRequestNextPage(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	pageNumber := ctx.GetAny("pageNumber").(int)

	nextPage := pageNumber + 1
	ctx.Put("pageNumber", nextPage)

	dir := path.Dir(e.Request.URL.Path)
	nextFile := ctx.Get("chapterRootStem") + "_" + strconv.Itoa(nextPage) + ctx.Get("chapterRootExt")
	nextUrl := path.Join(dir, nextFile)
	e.Request.Visit(nextUrl)
}
