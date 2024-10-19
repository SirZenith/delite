package book_dl

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SirZenith/bilinovel/base"
	"github.com/gocolly/colly/v2"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
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

	volumeInfo := desktopGetVolumeInfo(volIndex, e, options)
	os.MkdirAll(volumeInfo.outputDir, 0o755)

	log.Printf("volume %d: %s\n", volIndex+1, volumeInfo.title)

	e.ForEach("ul.chapter-list li a", func(chapIndex int, e *colly.HTMLElement) {
		desktopOnChapterEntry(chapIndex, e, volumeInfo)
	})
}

// Extracts volume info from desktop page element.
func desktopGetVolumeInfo(volIndex int, e *colly.HTMLElement, options *options) volumeInfo {
	title := e.DOM.Find("div.volume-info").Text()
	title = strings.TrimSpace(title)

	var outputTitle string
	if title == "" {
		outputTitle = fmt.Sprintf("Vol.%03d", volIndex+1)
	} else {
		outputTitle = fmt.Sprintf("%03d - %s", volIndex+1, title)
	}

	return volumeInfo{
		volIndex: volIndex,
		title:    title,

		outputDir:    filepath.Join(options.outputDir, outputTitle),
		imgOutputDir: filepath.Join(options.imgOutputDir, outputTitle),
	}
}

// Handles one chapter link found in desktop chapter entry.
func desktopOnChapterEntry(chapIndex int, e *colly.HTMLElement, volumeInfo volumeInfo) {
	title := strings.TrimSpace(e.Text)
	url := e.Attr("href")

	collectChapterPages(e, chapterInfo{
		volumeInfo: volumeInfo,
		chapIndex:  chapIndex,
		title:      title,

		url: url,
	})
}

// Handles novel chapter content page encountered during collecting.
func desktopOnPageContent(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*chapterDownloadState)

	content := desktopGetContentText(e)
	isFinished := desktopCheckChapterIsFinished(e)
	page := pageContent{
		pageNumber: state.curPageNumber,
		content:    content,
		isFinished: isFinished,
	}

	if state.curPageNumber == 1 {
		page.title = desktopGetChapterTitle(e)
	}

	state.resultChan <- page

	desktopDownloadChapterImages(e)

	if !isFinished {
		desktopRequestNextPage(e)
	}
}

// Extracts chapter title from page element.
func desktopGetChapterTitle(e *colly.HTMLElement) string {
	title := e.DOM.Find("#mlfy_main_text h1").First().Text()
	title = strings.TrimSpace(title)
	return title
}

// Extracts chapter content from page element.
// This function will do text decypher by font descramble map before returning
// page content.
func desktopGetContentText(e *colly.HTMLElement) string {
	desktopMarkFontDescrambleTargets(e.DOM)

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

// Desktop content page has some random cypher implement with font scrambling
// And CSS selector. All element set to use `fomt-family: "read"` should be
// translated with decypher map.
func desktopMarkFontDescrambleTargets(node *goquery.Selection) {
	root := node.Parents().Last()
	if len(root.Nodes) == 0 {
		root = node
	}

	targetMap := desktopFindDecypherTargets(root)
	for selector := range targetMap {
		root.Find(selector).Each(func(_ int, target *goquery.Selection) {
			target.SetAttr(base.FontDecypherAttr, "true")
		})
	}
}

// Gathers all selectors that should be handled in font decyphering.
func desktopFindDecypherTargets(root *goquery.Selection) map[string]bool {
	targetMap := map[string]bool{}

	root.Find("head style").Each(func(_ int, styleTag *goquery.Selection) {
		cssText := styleTag.Text()
		reader := strings.NewReader(cssText)
		input := parse.NewInput(reader)
		parser := css.NewParser(input, false)

		selector := ""
	outter:
		for {
			gt, _, data := parser.Next()

			switch gt {
			case css.BeginRulesetGrammar:
				// add new selector
				for _, val := range parser.Values() {
					selector += string(val.Data)
				}
			case css.EndRulesetGrammar:
				// clear selector
				selector = ""
			case css.DeclarationGrammar:
				// search for font-family attr
				declName := string(data)
				if declName != "font-family" {
					break
				}

				foundTarget := false
				for _, val := range parser.Values() {
					str := strings.TrimSpace(string(val.Data))
					str = strings.Trim(str, "\"")
					if str == "read" {
						foundTarget = true
						break
					}
				}

				if foundTarget && selector != "" {
					targetMap[selector] = true
				}
			case css.ErrorGrammar:
				break outter
			}
		}
	})

	return targetMap
}

// Checks if given chapter page element is the last page of this chapter.
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

// Downloads all illustrations found in given chapter content page.
func desktopDownloadChapterImages(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	collector := ctx.GetAny("collector").(*colly.Collector)
	state := ctx.GetAny("downloadState").(*chapterDownloadState)

	outputDir := state.info.imgOutputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Printf("failed to create imge output directory %s: %s", outputDir, err)
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

		basename := path.Base(url)
		outputName := filepath.Join(outputDir, basename)
		if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
			log.Println("skip:", outputName)
			return
		}

		dlContext := colly.NewContext()
		dlContext.Put("dlFileTo", outputName)

		collector.Request("GET", url, nil, dlContext, map[string][]string{
			"Referer": {"https://www.linovelib.com/"},
		})
	})
}

// Makes a new collect request to next page of given chapter page.
func desktopRequestNextPage(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*chapterDownloadState)
	state.curPageNumber++

	dir := path.Dir(e.Request.URL.Path)
	nextFile := fmt.Sprintf("%s_%d%s", state.rootNameStem, state.curPageNumber, state.rootNameExt)
	nextUrl := path.Join(dir, nextFile)
	e.Request.Visit(nextUrl)
}
