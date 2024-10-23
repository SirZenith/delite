package book_dl

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

// Setups collector callbacks for collection novel content from desktop novel page.
func setupDesktopCollector(c *colly.Collector, options options) {
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.linovelib.com",
		Delay:      options.requestDelay,
	})

	c.OnHTML("div#volume-list", onDesktopVolumeList)
	c.OnHTML("div.mlfy_main", onDesktopPageContent)
}

// Handles volume list found on novel's desktop TOC page.
func onDesktopVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.volume", onDesktopVolumeEntry)
}

// Handles one volume block found in desktop volume list.
func onDesktopVolumeEntry(volIndex int, e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	options := ctx.GetAny("options").(*options)

	fmt.Println("volume", volIndex)

	volumeInfo := getVolumeInfo(volIndex, e, options)
	os.MkdirAll(volumeInfo.outputDir, 0o755)

	e.ForEach("ul.chapter-list li a", func(chapIndex int, e *colly.HTMLElement) {
		onDesktopChapterEntry(chapIndex, e, volumeInfo)
	})
}

// Extracts volume info from desktop page element.
func getVolumeInfo(volIndex int, _ *colly.HTMLElement, options *options) volumeInfo {
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
		outputDir:    path.Join(options.outputDir, outputTitle),
		imgOutputDir: path.Join(options.imgOutputDir, outputTitle),
	}
}

// Handles one chapter link found in desktop chapter entry.
func onDesktopChapterEntry(chapIndex int, e *colly.HTMLElement, volumeInfo volumeInfo) {
	title := strings.TrimSpace(e.Text)
	url := e.Attr("href")

	var outputTitle string
	if title == "" {
		outputTitle = fmt.Sprintf("Chap.%04d.html", chapIndex)
	} else {
		outputTitle = fmt.Sprintf("%04d - %s.html", chapIndex, title)
	}

	collectChapterPages(e, chapterInfo{
		url:          url,
		title:        title,
		outputName:   path.Join(volumeInfo.outputDir, outputTitle),
		imgOutputDir: volumeInfo.imgOutputDir,
	})
}

func onDesktopPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	result := ctx.GetAny("resultChannel").(chan pageContent)
	pageNumber := ctx.GetAny("pageNumber").(int)

	content := getDesktopContentText(e)
	isFinished := checkDesktopChapterIsFinished(e)

	downloadDesktopChapterImages(e)

	result <- pageContent{
		pageNumber: pageNumber,
		content:    content,
		isFinished: isFinished,
	}

	if !isFinished {
		requestNextPage(e)
	}
}

func getDesktopContentText(e *colly.HTMLElement) string {
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

func checkDesktopChapterIsFinished(e *colly.HTMLElement) bool {
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

func downloadDesktopChapterImages(e *colly.HTMLElement) {
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
		outputName := path.Join(options.imgOutputDir, basename)
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

func requestNextPage(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	pageNumber := ctx.GetAny("pageNumber").(int)

	nextPage := pageNumber + 1
	ctx.Put("pageNumber", nextPage)

	dir := path.Dir(e.Request.URL.Path)
	nextFile := ctx.Get("chapterRootStem") + "_" + strconv.Itoa(nextPage) + ctx.Get("chapterRootExt")
	nextUrl := path.Join(dir, nextFile)
	e.Request.Visit(nextUrl)
}
