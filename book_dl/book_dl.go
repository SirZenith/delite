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
				Usage: "url of book's table of contents page",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: fmt.Sprintf("output directory for downloaded HTML (default: %s)", common.DefaultHtmlOutput),
			},
			&cli.StringFlag{
				Name:  "img-output",
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
				Usage: "a JSON file containing header info, headers is given in form of Array<{ name: string, value: string }>",
			},
			&cli.StringFlag{
				Name:  "name-map",
				Usage: "a JSON file containing name mapping between chapter title and actual output file, in form of Array<{ title: string, file: string }>",
			},
			&cli.StringFlag{
				Name:  "info-file",
				Usage: "path of book info JSON, if given command will try to download with option written in info file",
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library info JSON",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			return cmdMain(options)
		},
	}

	return cmd
}

func getOptionsFromCmd(cmd *cli.Command) (common.Options, error) {
	options := common.Options{
		RequestDelay: cmd.Int("delay"),
		Timeout:      cmd.Int("timeout"),

		Targets: []common.DlTarget{},
	}

	if target, err := getDlTargetFromCmd(cmd); err != nil {
		return options, err
	} else if target.TargetURL != "" {
		options.Targets = append(options.Targets, target)
	}

	libraryInfoPath := cmd.String("library")
	if libraryInfoPath != "" {
		targetList, err := loadLibraryTargets(libraryInfoPath)
		if err != nil {
			return options, err
		}

		options.Targets = append(options.Targets, targetList...)
	}

	return options, nil
}

func getDlTargetFromCmd(cmd *cli.Command) (common.DlTarget, error) {
	target := common.DlTarget{
		TargetURL:    cmd.String("url"),
		OutputDir:    cmd.String("output"),
		ImgOutputDir: cmd.String("img-output"),

		HeaderFile:         cmd.String("header-file"),
		ChapterNameMapFile: cmd.String("name-map"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := base.ReadBookInfo(infoFile)
		if err != nil {
			return target, err
		}

		target.Title = bookInfo.Title
		target.Author = bookInfo.Author

		target.TargetURL = base.GetStrOr(target.TargetURL, bookInfo.TocURL)
		target.OutputDir = base.GetStrOr(target.OutputDir, bookInfo.RawDir)
		target.ImgOutputDir = base.GetStrOr(target.ImgOutputDir, bookInfo.ImgDir)

		target.HeaderFile = base.GetStrOr(target.HeaderFile, bookInfo.HeaderFile)
		target.ChapterNameMapFile = base.GetStrOr(target.ChapterNameMapFile, bookInfo.NameMapFile)
	}

	target.OutputDir = base.GetStrOr(target.OutputDir, common.DefaultHtmlOutput)
	target.ImgOutputDir = base.GetStrOr(target.ImgOutputDir, common.DefaultImgOutput)

	target.ChapterNameMapFile = base.GetStrOr(target.ChapterNameMapFile, common.DefaultNameMapPath)

	return target, nil
}

// loadLibraryTargets reads book list from library info JSON and returns them
// as a list of DlTarget.
func loadLibraryTargets(libInfoPath string) ([]common.DlTarget, error) {
	info, err := base.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	targets := []common.DlTarget{}
	for _, book := range info.Books {
		targets = append(targets, common.DlTarget{
			Title:  book.Title,
			Author: book.Author,

			TargetURL:    book.TocURL,
			OutputDir:    book.RawDir,
			ImgOutputDir: book.ImgDir,

			HeaderFile:         book.HeaderFile,
			ChapterNameMapFile: book.NameMapFile,
		})
	}

	return targets, nil
}

func cmdMain(options common.Options) error {
	if len(options.Targets) <= 0 {
		return fmt.Errorf("no download target found")
	}

	for _, target := range options.Targets {
		target.RequestDelay = options.RequestDelay
		target.Timeout = options.Timeout

		logBookDlBeginBanner(target)

		c, err := makeCollector(target)
		if err != nil {
			log.Errorf("failed to create collector for %s:\n\t%s", target.TargetURL, err)
			continue
		}

		err = setupCollectorCallback(c, target)
		if err != nil {
			log.Errorf("unable to setup collector for %s:\n\t%s", target.TargetURL, err)
			continue
		}

		c.Visit(target.TargetURL)
		c.Wait()
	}

	return nil
}

// logBookDlBeginBanner prints a banner indicating a new download of book starts.
func logBookDlBeginBanner(target common.DlTarget) {
	msgs := []string{
		fmt.Sprintf("%-12s: %s", "download", target.TargetURL),
		fmt.Sprintf("%-12s: %s", "text  output", target.OutputDir),
		fmt.Sprintf("%-12s: %s", "image output", target.ImgOutputDir),
	}

	if target.Title != "" {
		msgs = append(msgs, fmt.Sprintf("%-12s: %s", "title", target.Title))
	}
	if target.Author != "" {
		msgs = append(msgs, fmt.Sprintf("%-12s: %s", "author", target.Author))
	}

	base.LogBannerMsg(msgs, 5)
}

// Returns collector used for novel downloading.
func makeCollector(target common.DlTarget) (*colly.Collector, error) {
	// ensure output directory
	if stat, err := os.Stat(target.OutputDir); errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(target.OutputDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %s", err)
		}
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("An file with name %s already exists", target.OutputDir)
	}

	// load headers
	headers := map[string]string{}
	if target.HeaderFile != "" {
		err := readHeaderFile(target.HeaderFile, headers)
		if err != nil {
			return nil, err
		}
	}

	// load name map
	nameMap := &common.GardedNameMap{NameMap: make(map[string]common.NameMapEntry)}
	if target.ChapterNameMapFile != "" {
		err := nameMap.ReadNameMap(target.ChapterNameMapFile)
		if err != nil {
			return nil, err
		}
	}

	c := colly.NewCollector(
		colly.Headers(headers),
		colly.Async(true),
	)

	global := &common.CtxGlobal{
		Target:    &target,
		Collector: c,
		NameMap:   nameMap,
	}

	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("global", global)
	})
	c.OnResponse(func(r *colly.Response) {
		if data, err := decompressResponseBody(r); err == nil {
			r.Body = data
		} else {
			log.Error(err)
		}

		if onResponse, ok := r.Ctx.GetAny("onResponse").(colly.ResponseCallback); ok {
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

	return c, nil
}

// setupCollectorCallback sets collector HTML callback for collecting novel pages.
func setupCollectorCallback(collector *colly.Collector, target common.DlTarget) error {
	url, err := url.Parse(target.TargetURL)
	if err != nil {
		return fmt.Errorf("unable to parse target URL: %s", target.TargetURL)
	}

	hostname := url.Hostname()
	hostMap := map[string]func(*colly.Collector, common.DlTarget) error{
		"bilinovel.com": func(_ *colly.Collector, _ common.DlTarget) error {
			return fmt.Errorf("mobile support is closed for now")
		},
		"linovelib.com": linovelib.SetupCollector,
		"syosetu.com":   syosetu.SetupCollector,
	}

	for suffix, setupFunc := range hostMap {
		if strings.HasSuffix(hostname, suffix) {
			err = setupFunc(collector, target)
			break
		}
	}

	return err
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
