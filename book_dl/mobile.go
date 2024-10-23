package book_dl

import (
	"bytes"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

func setupMobileCollector(c *colly.Collector, options options) {
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.bilinovel.com",
		Delay:      options.requestDelay,
	})

	c.OnHTML("li.chapter-li a.chapter-li-a", onMobileChapterListElement)
	c.OnHTML("body#aread", onMobilePageContent)

}

func onMobileVolumeList(e *colly.HTMLElement) {}

func onMobileChapterListElement(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	title := strings.TrimSpace(e.Text)
	url := e.Attr("href")

	options := ctx.GetAny("options").(*options)
	baseName := path.Base(url)
	outputName := filepath.Join(options.outputDir, baseName)

	collectChapterPages(e, chapterInfo{
		url:        url,
		title:      title,
		outputName: outputName,
	})
}

func onMobilePageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	result := ctx.GetAny("resultChannel").(chan pageContent)
	pageNumber := ctx.GetAny("pageNumber").(int)

	buffer := bytes.NewBufferString("")

	mainContent := e.DOM.Find("div.bcontent")
	mainContent.Children().Each(func(_ int, child *goquery.Selection) {
		if child.Is("div.cgo") {
			return
		}

		if html, err := goquery.OuterHtml(child); err == nil {
			buffer.WriteString(html)
			buffer.WriteString("\n")
		}
	})

	isFinished := checkMobileChapterIsFinished(e)
	result <- pageContent{
		pageNumber: pageNumber,
		content:    buffer.String(),
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

// Check if given html document is the last page of a chapter.
func checkMobileChapterIsFinished(e *colly.HTMLElement) bool {
	footer := e.DOM.Find("div#footlink")

	isFinished := true
	footer.Children().EachWithBreak(func(_ int, element *goquery.Selection) bool {
		text := element.Text()
		if text == nextPageTextSC || text == nextPageTextTC {
			isFinished = false
		}

		return isFinished
	})

	return isFinished
}
