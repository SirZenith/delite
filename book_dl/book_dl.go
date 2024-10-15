package book_dl

import (
	"bufio"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/bilinovel/base"
	"github.com/gocolly/colly/v2"
	"github.com/urfave/cli/v3"
)

const DEFAULT_HTML_OUTPUT = "./text"
const DEFAULT_IMG_OUTPUT = "./image"

type Options struct {
	targetURL    string
	outputDir    string
	imgOutputDir string
	requestDelay time.Duration
	timeout      time.Duration
	cookie       string
	headerFile   string
}

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:    "download",
		Aliases: []string{"dl"},
		Usage:   "download book",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "url",
				Value: "",
				Usage: "url of book's table of contents page",
			},
			&cli.StringFlag{
				Name:  "output",
				Value: "",
				Usage: fmt.Sprintf("output directory for downloaded HTML (default: %s)", DEFAULT_HTML_OUTPUT),
			},
			&cli.StringFlag{
				Name:  "img-output",
				Value: "",
				Usage: fmt.Sprintf("output directory for downloaded images (default: %s)", DEFAULT_IMG_OUTPUT),
			},
			&cli.DurationFlag{
				Name:  "delay",
				Value: 1500 * time.Millisecond,
				Usage: "page request delay in milisecond",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Value: 8 * time.Second,
				Usage: "request timeout for content page",
			},
			&cli.StringFlag{
				Name:  "header-file",
				Value: "",
				Usage: "a JSON file containing header info, headers is given in form of Array<{ name: string, value: string }>",
			},
			&cli.StringFlag{
				Name:  "info-file",
				Value: "",
				Usage: "path of book info JSON, if given command will try to download with option written in info file",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getDLOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			if options.targetURL == "" {
				return fmt.Errorf("no TOC URL is given, please use --url flag to specify one or use --info-file flag to give a book info JSON")
			}

			return bookDl(options)
		},
	}

	return cmd
}

func getDLOptionsFromCmd(cmd *cli.Command) (Options, error) {
	options := Options{
		targetURL:    cmd.String("url"),
		outputDir:    cmd.String("output"),
		imgOutputDir: cmd.String("img-output"),
		requestDelay: cmd.Duration("delay"),
		timeout:      cmd.Duration("timeout"),
		headerFile:   cmd.String("header-file"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := base.ReadBookInfo(infoFile)
		if err != nil {
			return options, err
		}

		if options.targetURL == "" {
			options.targetURL = bookInfo.TocURL
		}

		if options.outputDir == "" {
			options.outputDir = bookInfo.RawHTMLOutput
		}

		if options.imgOutputDir == "" {
			options.imgOutputDir = bookInfo.ImgOutput
		}
	}

	if options.outputDir == "" {
		options.outputDir = DEFAULT_HTML_OUTPUT
	}

	if options.imgOutputDir == "" {
		options.imgOutputDir = DEFAULT_IMG_OUTPUT
	}

	return options, nil
}

func bookDl(options Options) error {
	fmt.Println("download    :", options.targetURL)
	fmt.Println("text  output:", options.outputDir)
	fmt.Println("image output:", options.imgOutputDir)

	c, err := makeCollector(options)
	if err != nil {
		return err
	}

	c.Visit(options.targetURL)

	return nil
}

func makeCollector(options Options) (*colly.Collector, error) {
	// ensure output directory
	if stat, err := os.Stat(options.outputDir); errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(options.outputDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %s", err)
		}
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("An file with name %s already exists", options.outputDir)
	}

	// load headers
	headers := map[string]string{}
	if options.headerFile != "" {
		err := readHeaderFile(options.headerFile, headers)
		if err != nil {
			return nil, fmt.Errorf("failed to read header file: %s", err)
		}
	}

	c := colly.NewCollector(
		colly.Headers(headers),
	)
	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("options", &options)
	})
	c.OnResponse(func(r *colly.Response) {
		encoding := r.Headers.Get("content-encoding")
		decompressFunc := getBodyDecompressFunc(encoding)

		if data, err := decompressFunc(r.Body); err == nil {
			r.Body = data
		} else {
			log.Println(err)
		}
	})
	c.OnError(func(r *colly.Response, err error) {
		if onError, ok := r.Ctx.GetAny("onError").(colly.ErrorCallback); ok {
			onError(r, err)
		} else {
			log.Printf("error requesting %s: %s", r.Request.URL, err)
		}
	})

	if url, err := url.Parse(options.targetURL); err == nil {
		hostname := url.Hostname()

		if strings.HasSuffix(hostname, "bilinovel.com") {
			setupMobileCollector(c, options)
		} else if strings.HasSuffix(hostname, "linovelib.com") {
			setupDesktopCollector(c, options)
		}
	}

	return c, nil
}

func makeCookie(targetURL, cookie string) (http.CookieJar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	cookies := []*http.Cookie{}

	pairs := strings.Split(cookie, ";")
	for _, pair := range pairs {
		pair = strings.TrimLeft(pair, " ")
		parts := strings.SplitN(pair, "=", 2)

		if len(parts) == 2 {
			cookies = append(cookies, &http.Cookie{
				Name:  parts[0],
				Value: parts[1],
			})
		}
	}

	hostURL, err := url.Parse(targetURL)
	jar.SetCookies(hostURL, cookies)

	return jar, nil
}

type HeaderValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func readHeaderFile(path string, result map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	list := []HeaderValue{}
	json.Unmarshal(data, &list)

	for _, entry := range list {
		result[entry.Name] = entry.Value
	}

	return nil
}

// ----------------------------------------------------------------------------
// Book content handling

type ChapterContent struct {
	pageNumber int    // page number of this content in this chapter
	content    string // page content
	isFinished bool   // this should be true if current content is the last page of this chapter
}

const nextPageTextTC = "下一頁"
const nextPageTextSC = "下一页"

type ChapterInfo struct {
	url        string
	title      string
	outputName string
}

func collectChapterPages(e *colly.HTMLElement, info ChapterInfo) {
	ctx := e.Request.Ctx

	options := ctx.GetAny("options").(*Options)

	if _, err := os.Stat(info.outputName); err == nil {
		log.Printf("skip chapter %s, output file already exists: %s", info.title, info.outputName)
		return
	}

	result := make(chan ChapterContent, 5)

	updateChapterCtx(ctx, info.title, info.url, result)

	go e.Request.Visit(info.url)

	pageList, err := waitPages(result, options.timeout)
	if err != nil {
		log.Printf("failed to download %s: %s\n", info.title, err)
		return
	}

	pageCnt := pageList.Len()
	pageList.PushFront(ChapterContent{
		pageNumber: -1,
		content:    "<h1 class=\"chapter-title\">" + info.title + "</h1>\n",
		isFinished: false,
	})

	if err = saveChapterContent(pageList, info.outputName); err == nil {
		log.Printf("chapter %s (with page %d) saved to: %s\n", info.title, pageCnt, info.outputName)
	} else {
		log.Printf("error occured during saving %s: %s", info.title, err)
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
