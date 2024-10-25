package linovelib

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	dl_common "github.com/SirZenith/delite/cmd/book_dl/internal/common"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/network"
	collect "github.com/SirZenith/delite/page_collect"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

const defaultDelay = 1500
const defaultTimeOut = 10_000

// Setups collector callbacks for collecting novel content from desktop novel page.
func SetupCollector(c *colly.Collector, target collect.DlTarget) error {
	delay := common.GetDurationOr(target.Options.RequestDelay, defaultDelay)
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.linovelib.com",
		Delay:      time.Duration(delay) * time.Millisecond,
	})

	timeout := common.GetDurationOr(target.Options.RequestDelay, defaultTimeOut)

	c.SetRequestTimeout(timeout * time.Millisecond)
	c.OnHTML("div#volume-list", onVolumeList)
	c.OnHTML("div.mlfy_main", onPageContent)

	return nil
}

// Handles volume list found on novel's desktop TOC page.
func onVolumeList(e *colly.HTMLElement) {
	e.ForEach("div.volume", onVolumeEntry)
}

// Handles one volume block found in desktop volume list.
func onVolumeEntry(volIndex int, e *colly.HTMLElement) {
	global := e.Request.Ctx.GetAny("global").(*collect.CtxGlobal)

	volumeInfo := getVolumeInfo(volIndex+1, e, global.Target)
	os.MkdirAll(volumeInfo.OutputDir, 0o755)

	log.Infof("volume %d: %s", volumeInfo.VolIndex, volumeInfo.Title)

	chapterList := []*colly.HTMLElement{}
	e.ForEach("ul.chapter-list li a", func(chapIndex int, e *colly.HTMLElement) {
		chapterList = append(chapterList, e)
	})

	volumeInfo.TotalChapterCnt = len(chapterList)

	for chapIndex, chapter := range chapterList {
		onChapterEntry(chapIndex+1, chapter, volumeInfo)
	}
}

// Extracts volume info from desktop page element.
func getVolumeInfo(volIndex int, e *colly.HTMLElement, target *collect.DlTarget) collect.VolumeInfo {
	title := e.DOM.Find("div.volume-info").Text()
	title = strings.TrimSpace(title)

	outputTitle := common.InvalidPathCharReplace(title)
	if outputTitle == "" {
		outputTitle = fmt.Sprintf("Vol.%03d", volIndex)
	} else {
		outputTitle = fmt.Sprintf("%03d - %s", volIndex, outputTitle)
	}

	return collect.VolumeInfo{
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
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	page := collect.PageContent{
		PageNumber:     state.CurPageNumber,
		Content:        getContentText(e),
		NextChapterURL: getNextChapterURL(e),
	}

	if state.CurPageNumber == 1 {
		page.Title = getChapterTitle(e)
	}

	state.ResultChan <- page

	downloadChapterImages(e)

	if checkChapterIsFinished(e) {
		close(state.ResultChan)
	} else {
		requestNextPage(e)
	}
}

// Extracts chapter title from page element.
func getChapterTitle(e *colly.HTMLElement) string {
	title := e.DOM.Find("#mlfy_main_text h1").First().Text()
	title = strings.TrimSpace(title)
	return title
}

// Extracts chapter content from page element.
// This function will do text decypher by font descramble map before returning
// page content.
func getContentText(e *colly.HTMLElement) string {
	markFontDescrambleTargets(e.DOM)

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
func markFontDescrambleTargets(node *goquery.Selection) {
	root := node.Parents().Last()
	if len(root.Nodes) == 0 {
		root = node
	}

	targetMap := findDecypherTargets(root)
	for selector := range targetMap {
		root.Find(selector).Each(func(_ int, target *goquery.Selection) {
			target.SetAttr(common.FontDecypherAttr, "true")
		})
	}
}

// Gathers all selectors that should be handled in font decyphering.
func findDecypherTargets(root *goquery.Selection) map[string]bool {
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

// Checks if given chapter page element is the last page of this chapter. If
func checkChapterIsFinished(e *colly.HTMLElement) bool {
	isFinished := true

	footer := e.DOM.NextAll().Filter("div.mlfy_page").First()
	footer.Children().EachWithBreak(func(_ int, element *goquery.Selection) bool {
		text := strings.TrimSpace(element.Text())
		if text == dl_common.NextPageTextSC || text == dl_common.NextPageTextTC {
			isFinished = false
		}

		return isFinished
	})

	return isFinished
}

// Looks for anchor pointing to page of next chapter, if found, return it's href.
func getNextChapterURL(e *colly.HTMLElement) string {
	href := ""

	footer := e.DOM.NextAll().Filter("div.mlfy_page").First()
	footer.Children().EachWithBreak(func(index int, element *goquery.Selection) bool {
		text := strings.TrimSpace(element.Text())
		if text == dl_common.NextChapterText {
			href, _ = element.Attr("href")
			href = e.Request.AbsoluteURL(href)
		}

		return href == ""
	})

	return href
}

// Downloads all illustrations found in given chapter content page.
func downloadChapterImages(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*collect.CtxGlobal)
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)

	outputDir := state.Info.ImgOutputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Errorf("failed to create imge output directory %s: %s", outputDir, err)
		return
	}

	e.ForEach("div#TextContent img", func(_ int, img *colly.HTMLElement) {
		url := img.Attr("data-src")
		if url == "" {
			url = img.Attr("src")
		}

		if url == "" {
			return
		}

		basename := path.Base(url)
		outputName := filepath.Join(outputDir, basename)
		if _, err := os.Stat(outputName); !errors.Is(err, os.ErrNotExist) {
			log.Infof("skip image: Vol.%03d - Chap.%04d - %s", state.Info.VolIndex, state.Info.ChapIndex, basename)
			return
		}

		dlContext := colly.NewContext()
		dlContext.Put("onResponse", network.MakeSaveBodyCallback(outputName))

		global.Collector.Request("GET", url, nil, dlContext, map[string][]string{
			"Referer": {"https://www.linovelib.com/"},
		})
	})
}

// Makes a new collect request to next page of given chapter page.
func requestNextPage(e *colly.HTMLElement) {
	ctx := e.Request.Ctx
	state := ctx.GetAny("downloadState").(*collect.ChapterDownloadState)
	state.CurPageNumber++

	dir := path.Dir(e.Request.URL.Path)
	nextFile := fmt.Sprintf("%s_%d%s", state.RootNameStem, state.CurPageNumber, state.RootNameExt)
	nextUrl := path.Join(dir, nextFile)
	e.Request.Visit(nextUrl)
}
