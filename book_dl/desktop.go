package book_dl

import (
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

func setupDesktopCollector(c *colly.Collector, options Options) {
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.linovelib.com",
		Delay:      options.requestDelay,
	})

	c.OnHTML("div#volume-list", onDesktopVolumeList)
	c.OnHTML("div.mlfy_main", onDesktopPageContent)
}

func onDesktopVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.volume", onDesktopVolumeEntry)
}

func onDesktopVolumeEntry(volIndex int, e *colly.HTMLElement) {
	fmt.Println("volume", volIndex)
	// TODO: get volume info

	ctx := e.Request.Ctx
	options := ctx.GetAny("options").(*Options)

	e.ForEach("ul.chapter-list li a", func(chapIndex int, e *colly.HTMLElement) {
		title := strings.TrimSpace(e.Text)
		url := e.Attr("href")

		baseName := path.Base(url)
		outputName := path.Join(options.outputDir, baseName)

		collectChapterPages(e, ChapterInfo{
			url:        url,
			title:      title,
			outputName: outputName,
		})
	})
}

func onDesktopPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	result := ctx.GetAny("resultChannel").(chan ChapterContent)
	pageNumber := ctx.GetAny("pageNumber").(int)

	buffer := bytes.NewBufferString("")

	mainContent := e.DOM.Find("div#TextContent")
	mainContent.Children().Each(func(_ int, child *goquery.Selection) {
		if child.Is("div.dag") {
			return
		}

		if html, err := goquery.OuterHtml(child); err == nil {
			buffer.WriteString(html)
			buffer.WriteString("\n")
		}
	})

	isFinished := checkDesktopChapterIsFinished(e)
	result <- ChapterContent{
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
