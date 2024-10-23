package book_dl

import (
	"fmt"
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

	var outputDir string
	if title == "" {
		outputDir = fmt.Sprintf("Vol.%03d", volIndex+1)
	} else {
		outputDir = fmt.Sprintf("%03d - %s", volIndex+1, title)
	}
	outputDir = path.Join(options.outputDir, outputDir)

	return volumeInfo{
		title:     title,
		outputDir: outputDir,
	}
}

// Handles one chapter link found in desktop chapter entry.
func onDesktopChapterEntry(chapIndex int, e *colly.HTMLElement, volumeInfo volumeInfo) {
	title := strings.TrimSpace(e.Text)
	url := e.Attr("href")

	var outputName string
	if title == "" {
		outputName = fmt.Sprintf("Chap.%04d.html", chapIndex)
	} else {
		outputName = fmt.Sprintf("%04d - %s.html", chapIndex, title)
	}
	outputName = path.Join(volumeInfo.outputDir, outputName)

	collectChapterPages(e, chapterInfo{
		url:        url,
		title:      title,
		outputName: outputName,
	})
}

func onDesktopPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	result := ctx.GetAny("resultChannel").(chan pageContent)
	pageNumber := ctx.GetAny("pageNumber").(int)

	container := e.DOM.Find("div#TextContent")
	children := container.Children().Not("div.dag")
	segments := children.Map(func(_ int, child *goquery.Selection) string {
		if html, err := goquery.OuterHtml(child); err == nil {
			return html
		}
		return ""
	})
	content := strings.Join(segments, "\n")

	isFinished := checkDesktopChapterIsFinished(e)
	result <- pageContent{
		pageNumber: pageNumber,
		content:    content,
		isFinished: isFinished,
	}

	if !isFinished {
		nextPage := pageNumber + 1
		ctx.Put("pageNumber", nextPage)

		dir := path.Dir(e.Request.URL.Path)
		nextFile := ctx.Get("chapterRootStem") + "_" + strconv.Itoa(nextPage) + ctx.Get("chapterRootExt")
		nextUrl := path.Join(dir, nextFile)
		e.Request.Visit(nextUrl)
	}
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
