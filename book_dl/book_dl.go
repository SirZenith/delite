package book_dl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/SirZenith/litnovel-dl/base"
	"github.com/SirZenith/litnovel-dl/book_dl/internal/common"
	"github.com/SirZenith/litnovel-dl/book_dl/internal/linovelib"
	"github.com/SirZenith/litnovel-dl/book_dl/internal/syosetu"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:    "download",
		Aliases: []string{"dl"},
		Usage:   "download book from www.bilinovel.com or www.linovelib.com with book's TOC page URL",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "url",
				Value: "",
				Usage: "url of book's table of contents page",
			},
			&cli.StringFlag{
				Name:  "output",
				Value: "",
				Usage: fmt.Sprintf("output directory for downloaded HTML (default: %s)", common.DefaultHtmlOutput),
			},
			&cli.StringFlag{
				Name:  "img-output",
				Value: "",
				Usage: fmt.Sprintf("output directory for downloaded images (default: %s)", common.DefaultImgOutput),
			},
			&cli.IntFlag{
				Name:  "delay",
				Value: -1,
				Usage: "page request delay in milisecond",
			},
			&cli.IntFlag{
				Name:  "timeout",
				Value: -1,
				Usage: "request timeout for content page",
			},
			&cli.StringFlag{
				Name:  "header-file",
				Value: "",
				Usage: "a JSON file containing header info, headers is given in form of Array<{ name: string, value: string }>",
			},
			&cli.StringFlag{
				Name:  "name-map",
				Value: "",
				Usage: "a JSON file containing name mapping between chapter title and actual output file, in form of Array<{ title: string, file: string }>",
			},
			&cli.StringFlag{
				Name:  "info-file",
				Value: "",
				Usage: "path of book info JSON, if given command will try to download with option written in info file",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			if options.TargetURL == "" {
				return fmt.Errorf("no TOC URL is given, please use --url flag to specify one or use --info-file flag to give a book info JSON")
			}

			return cmdMain(options)
		},
	}

	return cmd
}

func getOptionsFromCmd(cmd *cli.Command) (common.Options, error) {
	options := common.Options{
		TargetURL:    cmd.String("url"),
		OutputDir:    cmd.String("output"),
		ImgOutputDir: cmd.String("img-output"),

		HeaderFile:         cmd.String("header-file"),
		ChapterNameMapFile: cmd.String("name-map"),

		RequestDelay: cmd.Int("delay"),
		Timeout:      cmd.Int("timeout"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := base.ReadBookInfo(infoFile)
		if err != nil {
			return options, err
		}

		options.TargetURL = base.GetStrOr(options.TargetURL, bookInfo.TocURL)
		options.OutputDir = base.GetStrOr(options.OutputDir, bookInfo.RawHTMLOutput)
		options.ImgOutputDir = base.GetStrOr(options.ImgOutputDir, bookInfo.ImgOutput)

		options.HeaderFile = base.GetStrOr(options.HeaderFile, bookInfo.HeaderFile)
		options.ChapterNameMapFile = base.GetStrOr(options.ChapterNameMapFile, bookInfo.NameMapFile)
	}

	options.OutputDir = base.GetStrOr(options.OutputDir, common.DefaultHtmlOutput)
	options.ImgOutputDir = base.GetStrOr(options.ImgOutputDir, common.DefaultImgOutput)

	options.ChapterNameMapFile = base.GetStrOr(options.ChapterNameMapFile, common.DefaultNameMapPath)

	return options, nil
}

func cmdMain(options common.Options) error {
	log.Infof("download    : %s", options.TargetURL)
	log.Infof("text  output: %s", options.OutputDir)
	log.Infof("image output: %s", options.ImgOutputDir)

	c, err := makeCollector(options)
	if err != nil {
		return err
	}

	c.Visit(options.TargetURL)
	c.Wait()

	return nil
}

// Returns collector used for novel downloading.
func makeCollector(options common.Options) (*colly.Collector, error) {
	// ensure output directory
	if stat, err := os.Stat(options.OutputDir); errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(options.OutputDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %s", err)
		}
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("An file with name %s already exists", options.OutputDir)
	}

	// load headers
	headers := map[string]string{}
	if options.HeaderFile != "" {
		err := readHeaderFile(options.HeaderFile, headers)
		if err != nil {
			return nil, err
		}
	}

	// load name map
	nameMap := &common.GardedNameMap{NameMap: make(map[string]common.NameMapEntry)}
	if options.ChapterNameMapFile != "" {
		err := nameMap.ReadNameMap(options.ChapterNameMapFile)
		if err != nil {
			return nil, err
		}
	}

	c := colly.NewCollector(
		colly.Headers(headers),
		colly.Async(true),
	)
	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("options", &options)
		r.Ctx.Put("collector", c)
		r.Ctx.Put("nameMap", nameMap)
	})
	c.OnResponse(func(r *colly.Response) {
		if data, err := decompressResponseBody(r); err == nil {
			r.Body = data
		} else {
			log.Error(err)
		}

		ctx := r.Ctx

		if onResponse, ok := ctx.GetAny("onResponse").(colly.ResponseCallback); ok {
			onResponse(r)
		}
	})
	c.OnError(func(r *colly.Response, err error) {
		ctx := r.Ctx

		if onError, ok := ctx.GetAny("onError").(colly.ErrorCallback); ok {
			onError(r, err)
		} else {
			log.Errorf("error requesting %s: %s", r.Request.URL, err)
		}
	})

	if url, err := url.Parse(options.TargetURL); err == nil {
		hostname := url.Hostname()

		if strings.HasSuffix(hostname, "bilinovel.com") {
			log.Fatal("mobile support is closed for now")
		} else if strings.HasSuffix(hostname, "linovelib.com") {
			linovelib.SetupCollector(c, options)
		} else if strings.HasSuffix(hostname, "syosetu.com") {
			syosetu.SetupCollector(c, options)
		}
	}

	return c, nil
}

type headerValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Reads header value from file and stores then into the map passed as argument.
// Header file should a JSON containing array of header objects. Each header
// objects should be object with tow string field `name` and `value`.
func readHeaderFile(path string, result map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %s", path, err)
	}

	list := []headerValue{}
	err = json.Unmarshal(data, &list)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %s", path, err)
	}

	for _, entry := range list {
		result[entry.Name] = entry.Value
	}

	return nil
}
