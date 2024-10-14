package book_dl

import (
	"bufio"
	"bytes"
	"container/list"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

type ChapterContent struct {
	pageNumber int    // page number of this content in this chapter
	content    string // page content
	isFinished bool   // this should be true if current content is the last page of this chapter
}

const nextPageTextTC = "下一頁"
const nextPageTextSC = "下一页"

func onVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.volume", func(volumeIndex int, volume *colly.HTMLElement) {
		fmt.Println("volume", volumeIndex)
		volume.ForEach("ul.chapter-list li a", func(_ int, chapter *colly.HTMLElement) {
			onChapterListElement(chapter)
		})
	})
}

func onChapterListElement(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	chapterName := strings.TrimSpace(e.Text)
	chapterRoot := e.Attr("href")

	options := ctx.GetAny("options").(*Options)
	baseName := path.Base(chapterRoot)
	outputName := path.Join(options.outputDir, baseName)
	if _, err := os.Stat(outputName); err == nil {
		log.Printf("skip chapter %s, output file already exists: %s", chapterName, outputName)
		return
	}

	result := make(chan ChapterContent, 5)

	updateChapterCtx(ctx, chapterName, chapterRoot, result)

	go e.Request.Visit(chapterRoot)

	pageList, err := waitPages(result, options.timeout)
	if err != nil {
		log.Printf("failed to download %s: %s\n", chapterName, err)
		return
	}

	pageCnt := pageList.Len()
	pageList.PushFront(ChapterContent{
		pageNumber: -1,
		content:    "<h1 class=\"chapter-title\">" + chapterName + "</h1>\n",
		isFinished: false,
	})

	if err = saveChapterContent(pageList, outputName); err == nil {
		log.Printf("chapter %s (with page %d) saved to: %s\n", chapterName, pageCnt, outputName)
	} else {
		log.Printf("error occured during saving %s: %s", chapterName, err)
	}
}

func onPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	result := ctx.GetAny("resultChannel").(chan ChapterContent)
	pageNumber := ctx.GetAny("pageNumber").(int)

	content, err := e.DOM.Html()
	if err != nil {
		content = fmt.Sprintf("<h2>Failed to download page %d</h2>", pageNumber)
	}

	isFinished := checkChapterIsFinished(e)
	result <- ChapterContent{
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

func onMobilePageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	result := ctx.GetAny("resultChannel").(chan ChapterContent)
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

func updateChapterCtx(ctx *colly.Context, chapterName, chapterRoot string, resultChannel chan ChapterContent) {
	ctx.Put("resultChannel", resultChannel)
	ctx.Put("pageNumber", 1)

	ctx.Put("onError", func(_ *colly.Response, err error) {
		log.Printf("failed to complete downloading %s: %s", chapterName, err)
		close(resultChannel)
	})

	rootBaseName := path.Base(chapterRoot)
	rootExt := path.Ext(rootBaseName)
	rootBaseStem := rootBaseName[:len(rootBaseName)-len(rootExt)]
	ctx.Put("chapterRootExt", rootExt)
	ctx.Put("chapterRootStem", rootBaseStem)
}

func checkChapterIsFinished(e *colly.HTMLElement) bool {
	isFinished := true
	e.ForEachWithBreak("div.mlfy_page a", func(_ int, element *colly.HTMLElement) bool {
		// TODO: fix search process
		text := element.Text
		if text == nextPageTextSC || text == nextPageTextTC {
			isFinished = false
		}

		return isFinished
	})

	return isFinished
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

// Collect all pages sent from colly jobs with timeout.
func waitPages(result chan ChapterContent, timeout time.Duration) (*list.List, error) {
	pageList := list.New()
	pageList.Init()

	var err error
loop:
	for {
		select {
		case data, ok := <-result:
			if !ok {
				break loop
			}

			insertChapterPage(pageList, data)

			if data.isFinished {
				break loop
			}
		case <-time.After(timeout):
			err = fmt.Errorf("download timeout")
			break loop
		}
	}

	return pageList, err
}

// Insert newly fetched page content into page list according its page number.
func insertChapterPage(list *list.List, data ChapterContent) {
	target := list.Front()
	if target == nil {
		list.PushFront(data)
		return
	}

	for target.Value.(ChapterContent).pageNumber < data.pageNumber {
		next := target.Next()
		if next == nil {
			break
		}

		target = next
	}

	list.InsertAfter(data, target)
}

// Write content of chapter pages to file.
func saveChapterContent(list *list.List, outputName string) error {
	file, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to open output file %s: %s", outputName, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for element := list.Front(); element != nil; element = element.Next() {
		writer.WriteString(element.Value.(ChapterContent).content)
	}

	if err = writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush chapter file %s: %s", outputName, err)
	}

	return nil
}
