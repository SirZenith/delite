package book_dl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
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
				Value: 1000 * time.Millisecond,
				Usage: "page request delay in milisecond",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Value: 5 * time.Second,
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

	c.Limit(&colly.LimitRule{
		DomainGlob: "*.bilinovel.com",
		Delay:      options.requestDelay,
		// Parallelism: 2,
	})
	c.Limit(&colly.LimitRule{
		DomainGlob: "*.linovelib.com",
		Delay:      options.requestDelay,
	})

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

	// Mobile page
	// c.OnHTML("li.chapter-li a.chapter-li-a", onChapterAddress) // pattern for www.bilinovel.com (mobile page)
	// c.OnHTML("body#aread", onMobilePageContent) // patter for www.bili

	// Desktop page
	c.OnHTML("div#volume-list", onVolumeList)
	c.OnHTML("div.mlfy_main", onPageContent)

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
