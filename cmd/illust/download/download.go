package download

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/database"
	"github.com/SirZenith/delite/database/data_model"
	"github.com/SirZenith/delite/network"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"github.com/urfave/cli/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func Cmd() *cli.Command {
	var libIndex int64

	cmd := &cli.Command{
		Name:    "download",
		Aliases: []string{"dl"},
		Usage:   "find all image reference in downloadeded novel, make sure all images are downloaded and stored as PNG.",
		Flags: []cli.Flag{
			&cli.DurationFlag{
				Name:  "delay",
				Usage: "page request delay in milisecond",
				Value: -1,
			},
			&cli.StringFlag{
				Name:  "info-file",
				Usage: "path of book info JSON, if given command will try to download with option written in info file",
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library info JSON",
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
			&cli.IntArg{
				Name:        "library-index",
				UsageText:   "<index>",
				Destination: &libIndex,
				Value:       -1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, targets, err := getOptionsFromCmd(cmd, int(libIndex))
			if err != nil {
				return err
			}

			return cmdMain(options, targets)
		},
	}

	return cmd
}

type options struct {
	delay   time.Duration
	timeout time.Duration
	retry   int
}

type target struct {
	title      string
	targetURL  string
	rawTextDir string
	textDir    string
	imageDir   string

	parsedURL        *url.URL
	imgRequestHeader http.Header

	isLocal bool
	dbPath  string
}

type workload struct {
	target    *target
	collector *colly.Collector
}

func getOptionsFromCmd(cmd *cli.Command, libIndex int) (options, []target, error) {
	options := options{
		delay:   cmd.Duration("delay"),
		timeout: cmd.Duration("timeout"),
		retry:   int(cmd.Int("retry")),
	}

	targets := []target{}

	if target, err := getTargetFromCmd(cmd); err != nil {
		return options, nil, err
	} else if target.targetURL != "" {
		targets = append(targets, target)
	}

	libraryInfoPath := cmd.String("library")
	if libraryInfoPath != "" {
		targetList, err := loadLibraryTargets(libraryInfoPath)
		if err != nil {
			return options, nil, err
		}

		if 0 <= libIndex && libIndex < len(targetList) {
			targets = append(targets, targetList[libIndex])
		} else {
			targets = append(targets, targetList...)
		}
	}

	return options, targets, nil
}

func getTargetFromCmd(cmd *cli.Command) (target, error) {
	target := target{}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := book_mgr.ReadBookInfo(infoFile)
		if err != nil {
			return target, err
		}

		target.title = bookInfo.Title
		target.targetURL = bookInfo.TocURL
		target.rawTextDir = bookInfo.RawDir
		target.textDir = bookInfo.TextDir
		target.imageDir = bookInfo.ImgDir
		target.dbPath = bookInfo.DatabasePath
	}

	return target, nil
}

// loadLibraryTargets reads book list from library info JSON and returns them
// as a list of DlTarget.
func loadLibraryTargets(libInfoPath string) ([]target, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	targets := []target{}
	for _, book := range info.Books {
		targets = append(targets, target{
			title:      book.Title,
			targetURL:  book.TocURL,
			rawTextDir: book.RawDir,
			textDir:    book.TextDir,
			imageDir:   book.ImgDir,

			isLocal: book.LocalInfo != nil,
			dbPath:  book.DatabasePath,
		})
	}

	return targets, nil
}

func cmdMain(options options, targets []target) error {
	if len(targets) <= 0 {
		return fmt.Errorf("no download target found")
	}

	collector, err := makeCollector(options)
	if err != nil {
		return fmt.Errorf("failed to create collector: %s", err)
	}

	for _, target := range targets {
		logBookDlBeginBanner(target)

		if target.isLocal {
			log.Infof("skip local book")
		}

		target.parsedURL, err = url.Parse(target.targetURL)
		if err != nil {
			log.Warnf("invalid TOC URL: %s", err)
			continue
		}

		target.imgRequestHeader = getImageRequestHeader(target.parsedURL.Host)

		handlingBook(target, collector)
	}

	collector.Wait()

	return nil
}

// logBookDlBeginBanner prints a banner indicating a new download of book starts.
func logBookDlBeginBanner(target target) {
	msgs := []string{
		fmt.Sprintf("%-12s: %s", "handling", target.title),
		fmt.Sprintf("%-12s: %s", "raw text", target.rawTextDir),
		fmt.Sprintf("%-12s: %s", "text", target.textDir),
		fmt.Sprintf("%-12s: %s", "image output", target.imageDir),
	}

	common.LogBannerMsg(msgs, 5)
}

var (
	tocHeaderMap     map[string]http.Header
	onceTocHeaderMap sync.Once
)

func getImageRequestHeader(hostname string) http.Header {
	onceTocHeaderMap.Do(func() {
		tocHeaderMap = map[string]http.Header{
			"bilinovel.com": map[string][]string{
				"Referer": {"https://www.bilinovel.com"},
			},
			"linovelib.com": map[string][]string{
				"Referer": {"https://www.linovelib.com/"},
			},
			"syosetu.com": map[string][]string{
				"Referer": {"https://ncode.syosetu.com/"},
			},
		}
	})

	result := http.Header(map[string][]string{})

	for suffix, header := range tocHeaderMap {
		if strings.HasSuffix(hostname, suffix) {
			for k, v := range header {
				result[k] = v
			}
			break
		}
	}

	return result
}

// Returns collector used for novel downloading.
func makeCollector(options options) (*colly.Collector, error) {
	c := colly.NewCollector(
		colly.Async(true),
	)

	c.Limits([]*colly.LimitRule{
		{
			DomainGlob:  "img3.readpai.com",
			Parallelism: 5,
		},
	})

	// c.OnRequest(func(r *colly.Request) { })
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

func handlingBook(target target, collector *colly.Collector) error {
	entryList, err := os.ReadDir(target.rawTextDir)
	if err != nil {
		return fmt.Errorf("failed to read raw text directory %s: %s", target.rawTextDir, err)
	}

	err = os.MkdirAll(target.imageDir, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create image output directory %s: %s", target.imageDir, err)
	}

	var db *gorm.DB
	if target.dbPath != "" {
		db, err = database.Open(target.dbPath)
		if err != nil {
			return fmt.Errorf("failed to open book database %s: %s", target.dbPath, err)
		}
	}

	for _, entry := range entryList {
		if !entry.IsDir() {
			continue
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "target", &target)
		ctx = context.WithValue(ctx, "collector", collector)
		ctx = context.WithValue(ctx, "db", db)

		err := handlingVolume(ctx, entry.Name())
		if err != nil {
			log.Warn(err.Error())
		}
	}

	return nil
}

func handlingVolume(ctx context.Context, volumeName string) error {
	target := ctx.Value("target").(*target)

	imgDir := filepath.Join(target.imageDir, volumeName)
	err := os.MkdirAll(imgDir, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create volume image directory %s: %s", imgDir, err)
	}

	voluemDir := filepath.Join(target.rawTextDir, volumeName)

	entryList, err := os.ReadDir(voluemDir)
	if err != nil {
		return fmt.Errorf("failed to read volume directory %s: %s", voluemDir, err)
	}

	for _, entry := range entryList {
		if !entry.Type().IsRegular() {
			continue
		}

		_, err := handlingRawTextFile(ctx, volumeName, entry.Name())
		if err != nil {
			log.Warn(err.Error())
		}
	}

	return nil
}

func handlingRawTextFile(ctx context.Context, volumeName, basename string) (map[string]string, error) {
	target := ctx.Value("target").(*target)
	collector := ctx.Value("collector").(*colly.Collector)
	db, dbOk := ctx.Value("db").(*gorm.DB)

	filename := filepath.Join(target.rawTextDir, volumeName, basename)
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read text file %s: %s", data, err)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse raw text file %s: %s", filename, err)
	}

	nameMap := map[string]string{}
	imgDir := filepath.Join(target.imageDir, volumeName)

	doc.Find("img").Each(func(_ int, img *goquery.Selection) {
		src, ok := img.Attr("data-src")
		if !ok {
			src, _ = img.Attr("src")
		}

		if src == "" {
			return
		}

		parsedSrc, err := url.Parse(src)
		if err != nil {
			log.Warnf("invalid source URL %q: %s", parsedSrc, err)
			return
		}

		if parsedSrc.Scheme == "" {
			parsedSrc.Scheme = target.parsedURL.Scheme
		}

		if parsedSrc.Host == "" {
			parsedSrc.Host = target.parsedURL.Host
		}

		if !parsedSrc.IsAbs() {
			log.Warnf("skip %s, non-absolute URL handling has not yet been implemented", src)
			return
		}

		basename := path.Base(src)
		ext := path.Ext(basename)
		stem := basename[:len(basename)-len(ext)]
		basename = stem + ".png"

		if dbOk && db != nil {
			fullSrc := parsedSrc.String()
			entry := data_model.FileEntry{}
			db.Limit(1).Find(&entry, "url = ?", fullSrc)

			if entry.FileName == basename {
				log.Infof("skip: %s", fullSrc)
				return
			}
		}

		outputName := filepath.Join(imgDir, basename)

		dlContext := colly.NewContext()
		dlContext.Put("outputName", outputName)
		dlContext.Put("bookName", target.title)
		dlContext.Put("volumeName", volumeName)
		dlContext.Put("db", db)
		dlContext.Put("onResponse", colly.ResponseCallback(saveResponseAsPNG))

		collector.Request("GET", src, nil, dlContext, target.imgRequestHeader)
	})

	return nameMap, nil
}

func saveResponseAsPNG(resp *colly.Response) {
	ctx := resp.Ctx

	outputName := ctx.Get("outputName")
	if outputName == "" {
		log.Warnf("no image output name found in response context")
		return
	}

	err := network.SaveBodyAsPNG(resp, outputName)
	if err != nil {
		log.Warn(err.Error())
		return
	}

	if db, ok := ctx.GetAny("db").(*gorm.DB); ok && db != nil {
		entry := data_model.FileEntry{
			URL:      resp.Request.URL.String(),
			Book:     ctx.Get("bookName"),
			Volume:   ctx.Get("volumeName"),
			FileName: filepath.Base(outputName),
		}

		db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
	}

	log.Infof("image save to: %s", outputName)
}
