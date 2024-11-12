package book_dl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/cmd/book_dl/internal/bilimanga"
	"github.com/SirZenith/delite/cmd/book_dl/internal/linovelib"
	"github.com/SirZenith/delite/cmd/book_dl/internal/senmanga"
	"github.com/SirZenith/delite/cmd/book_dl/internal/syosetu"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/database"
	"github.com/SirZenith/delite/network"
	"github.com/SirZenith/delite/page_collect"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"github.com/urfave/cli/v3"
	"gorm.io/gorm"
)

func Cmd() *cli.Command {
	var libFilePath string
	var libIndex int64

	cmd := &cli.Command{
		Name:    "download",
		Aliases: []string{"dl"},
		Usage:   "download book from www.bilinovel.com or www.linovelib.com with book's TOC page URL",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "ignore-taken-down-flag",
				Usage: "also download books with `is_taken_down` flag",
			},
			&cli.IntFlag{
				Name:  "retry",
				Usage: "retry count for page download request",
				Value: 3,
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "request timeout for content page in milisecond",
				Value: -1,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "library-file",
				UsageText:   "<lib-file>",
				Destination: &libFilePath,
				Min:         1,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "library-index",
				UsageText:   " <index>",
				Destination: &libIndex,
				Value:       -1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, targets, err := getOptionsFromCmd(cmd, libFilePath, int(libIndex))
			if err != nil {
				return err
			}

			return cmdMain(options, targets)
		},
	}

	return cmd
}

func getOptionsFromCmd(cmd *cli.Command, libFilePath string, libIndex int) (page_collect.Options, []page_collect.DlTarget, error) {
	options := page_collect.Options{
		Timeout:  cmd.Duration("timeout"),
		RetryCnt: cmd.Int("retry"),

		IgnoreTakenDownFlag: cmd.Bool("ignore-taken-down-flag"),
	}

	targets := []page_collect.DlTarget{}

	targetList, err := loadLibraryInfo(&options, libFilePath)
	if err != nil {
		return options, nil, err
	}

	if 0 <= libIndex && libIndex < len(targetList) {
		targets = append(targets, targetList[libIndex])
	} else {
		targets = append(targets, targetList...)
	}

	return options, targets, nil
}

// loadLibraryInfo reads book list from library info JSON and returns them
// as a list of DlTarget.
func loadLibraryInfo(options *page_collect.Options, libInfoPath string) ([]page_collect.DlTarget, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	for _, rule := range info.LimitRules {
		options.LimitRules = append(options.LimitRules, rule.ToCollyLimitRule())
	}

	targets := []page_collect.DlTarget{}
	for _, book := range info.Books {
		targets = append(targets, page_collect.DlTarget{
			Title:  book.Title,
			Author: book.Author,

			TargetURL:    book.TocURL,
			OutputDir:    book.RawDir,
			ImgOutputDir: book.ImgDir,

			HeaderFile: book.HeaderFile,
			DbPath:     info.DatabasePath,

			IsTakenDown: book.IsTakenDown,
			IsLocal:     book.LocalInfo != nil,
		})
	}

	return targets, nil
}

func cmdMain(options page_collect.Options, targets []page_collect.DlTarget) error {
	if len(targets) <= 0 {
		return fmt.Errorf("no download target found")
	}

	for _, target := range targets {
		logBookDlBeginBanner(target)
		if target.IsLocal {
			log.Infof("skip local book")
			continue
		}

		if target.TargetURL == "" {
			log.Infof("this book provides no URL")
			continue
		}

		if !options.IgnoreTakenDownFlag && target.IsTakenDown {
			log.Infof("skip book due to DMCA takedown")
			continue
		}

		target.Options = &options

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
func logBookDlBeginBanner(target page_collect.DlTarget) {
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

	common.LogBannerMsg(msgs, 5)
}

// Returns collector used for novel downloading.
func makeCollector(target page_collect.DlTarget) (*colly.Collector, error) {
	// ensure output directory
	if stat, err := os.Stat(target.OutputDir); errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(target.OutputDir, 0o777); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %s", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to access output directory %s: %s", target.OutputDir, err)
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

	var db *gorm.DB
	if target.DbPath != "" {
		var err error
		db, err = database.Open(target.DbPath)
		if err != nil {
			return nil, err
		}
	}

	c := colly.NewCollector(
		colly.Headers(headers),
		colly.Async(true),
	)

	global := page_collect.NewCtxGlobal()
	global.Target = &target
	global.Collector = c
	global.Db = db

	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("global", global)
	})
	c.OnResponse(func(r *colly.Response) {
		if data, err := network.DecompressResponseBody(r); err == nil {
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
func setupCollectorCallback(collector *colly.Collector, target page_collect.DlTarget) error {
	url, err := url.Parse(target.TargetURL)
	if err != nil {
		return fmt.Errorf("unable to parse target URL: %s", target.TargetURL)
	}

	hostname := url.Hostname()
	hostMap := map[string]func(*colly.Collector, page_collect.DlTarget) error{
		"bilinovel.com": func(_ *colly.Collector, _ page_collect.DlTarget) error {
			return fmt.Errorf("mobile support is closed for now")
		},
		"bilimanga.net": bilimanga.SetupCollector,
		"linovelib.com": linovelib.SetupCollector,
		"senmanga.com":  senmanga.SetupCollector,
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
