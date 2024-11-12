package download

import (
	"bytes"
	"context"
	"errors"
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

const defaultRetryCnt = 3

func Cmd() *cli.Command {
	var libFilePath string
	var libIndex int64

	cmd := &cli.Command{
		Name:    "download",
		Aliases: []string{"dl"},
		Usage:   "find all image reference in downloadeded books, and make sure they are downloaded",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "retry",
				Usage: "retry count for page download request",
				Value: defaultRetryCnt,
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

type options struct {
	timeout    time.Duration
	retry      int
	limitRules []*colly.LimitRule
}

type headerMaker func(hostname string) http.Header

type outputNameMaker func(ctx context.Context, srcURL *url.URL, pageIndex int, format string) string

type target struct {
	title      string
	targetURL  string
	rawTextDir string
	textDir    string
	imageDir   string

	parsedURL *url.URL
	hostInfo  *hostInfo

	isLocal bool
	dbPath  string
}

type workload struct {
	target    *target
	collector *colly.Collector
}

type hostInfo struct {
	headerMaker        headerMaker
	imageFormat        string
	imageBasenameMaker outputNameMaker
}

func getOptionsFromCmd(cmd *cli.Command, libFilePath string, libIndex int) (options, []target, error) {
	options := options{
		timeout: cmd.Duration("timeout"),
		retry:   int(cmd.Int("retry")),
	}

	targets := []target{}

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
func loadLibraryInfo(options *options, libInfoPath string) ([]target, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	for _, rule := range info.LimitRules {
		options.limitRules = append(options.limitRules, rule.ToCollyLimitRule())
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
			dbPath:  info.DatabasePath,
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

		target.hostInfo = getHostInfo(target.parsedURL.Hostname())

		ctx := context.WithValue(context.Background(), "maxRetryCnt", options.retry)

		handlingBook(ctx, target, collector)
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
	tocHostInfoMap     map[string]hostInfo
	onceTocHostInfoMap sync.Once
)

func initTocHostInfoMap() {
	onceTocHostInfoMap.Do(func() {
		tocHostInfoMap = map[string]hostInfo{
			"bilinovel.com": hostInfo{
				imageFormat: common.ImageFormatPng,
				headerMaker: makeCopyHeaderMaker(map[string][]string{
					"Referer": {"https://www.bilinovel.com"},
				}),
				imageBasenameMaker: getSrcURLBasename,
			},
			"linovelib.com": hostInfo{
				imageFormat: common.ImageFormatPng,
				headerMaker: makeCopyHeaderMaker(map[string][]string{
					"Referer": {"https://www.linovelib.com/"},
				}),
				imageBasenameMaker: getSrcURLBasename,
			},
			"syosetu.com": hostInfo{
				imageFormat: common.ImageFormatPng,
				headerMaker: makeCopyHeaderMaker(map[string][]string{
					"Referer": {"https://ncode.syosetu.com/"},
				}),
				imageBasenameMaker: getSrcURLBasename,
			},
			"bilimanga.net": hostInfo{
				imageFormat: common.ImageFormatAvif,
				headerMaker: func(hostname string) http.Header {
					return map[string][]string{
						"Accept":          {"image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5"},
						"Accept-Encoding": {"deflate, br, zstd"},
						"Accept-Language": {"zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2"},
						"Connection":      {"keep-alive"},
						"Host":            {hostname},
						"Priority":        {"u=5, i"},
						"Referer":         {"https://www.bilimanga.net/"},
						"Sec-Fetch-Dest":  {"image"},
						"Sec-Fetch-Mode":  {"no-cors"},
						"Sec-Fetch-Site":  {"cross-site"},
					}
				},
				imageBasenameMaker: getMangaBasename,
			},
			"senmanga.com": hostInfo{
				imageFormat: common.ImageFormatAvif,
				headerMaker: func(hostname string) http.Header {
					return map[string][]string{
						"Accept":          {"image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5"},
						"Accept-Encoding": {"deflate, br, zstd"},
						"Connection":      {"keep-alive"},
						"Host":            {hostname},
						"Priority":        {"u=5, i"},
						"Referer":         {"https://raw.senmanga.com/"},
					}
				},
				imageBasenameMaker: getMangaBasename,
			},
		}
	})
}

func makeCopyHeaderMaker(header http.Header) headerMaker {
	return func(_ string) http.Header {
		result := http.Header(map[string][]string{})
		for k, v := range header {
			result[k] = v
		}

		return result
	}
}

func getSrcURLBasename(_ context.Context, srcURL *url.URL, pageIndex int, format string) string {
	return common.ReplaceFileExt(path.Base(srcURL.Path), "."+format)
}

func getMangaBasename(ctx context.Context, srcURL *url.URL, pageIndex int, format string) string {
	chapterIndex := ctx.Value("chapterIndex").(int)
	return common.GetMangaPageOutputBasename(chapterIndex, pageIndex, format)
}

func getHostInfo(hostname string) *hostInfo {
	initTocHostInfoMap()

	var result *hostInfo

	for suffix, info := range tocHostInfoMap {
		if strings.HasSuffix(hostname, suffix) {
			result = &info
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

	if len(options.limitRules) > 0 {
		c.Limits(options.limitRules)
	} else {
		c.Limits([]*colly.LimitRule{
			{
				DomainGlob:  "img3.readpai.com",
				Parallelism: 3,
			},
			{
				DomainGlob: "*.motiezw.com",
				Delay:      125 * time.Millisecond,
			},
			{
				DomainGlob:  "*.kumacdn.club",
				Parallelism: 5,
			},
		})
	}

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

func handlingBook(ctx context.Context, target target, collector *colly.Collector) error {
	entryList, err := os.ReadDir(target.rawTextDir)
	if err != nil {
		return fmt.Errorf("failed to read raw text directory %s: %s", target.rawTextDir, err)
	}

	err = os.MkdirAll(target.imageDir, 0o777)
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

	volumeIndex := 1
	for _, entry := range entryList {
		if !entry.IsDir() {
			continue
		}

		ctx = context.WithValue(ctx, "target", &target)
		ctx = context.WithValue(ctx, "collector", collector)
		ctx = context.WithValue(ctx, "db", db)
		ctx = context.WithValue(ctx, "volumeIndex", volumeIndex)

		err := handlingVolume(ctx, entry.Name())
		if err != nil {
			log.Warn(err.Error())
		}

		volumeIndex++
	}

	return nil
}

func handlingVolume(ctx context.Context, volumeName string) error {
	target := ctx.Value("target").(*target)

	imgDir := filepath.Join(target.imageDir, volumeName)
	err := os.MkdirAll(imgDir, 0o777)
	if err != nil {
		return fmt.Errorf("failed to create volume image directory %s: %s", imgDir, err)
	}

	voluemDir := filepath.Join(target.rawTextDir, volumeName)

	entryList, err := os.ReadDir(voluemDir)
	if err != nil {
		return fmt.Errorf("failed to read volume directory %s: %s", voluemDir, err)
	}

	chapterIndex := 1
	for _, entry := range entryList {
		if !entry.Type().IsRegular() {
			continue
		}

		ctx = context.WithValue(ctx, "chapterIndex", chapterIndex)

		_, err := handlingRawTextFile(ctx, volumeName, entry.Name())
		if err != nil {
			log.Warn(err.Error())
		}

		chapterIndex++
	}

	return nil
}

func handlingRawTextFile(ctx context.Context, volumeName, basename string) (map[string]string, error) {
	target := ctx.Value("target").(*target)
	collector := ctx.Value("collector").(*colly.Collector)
	db, dbOk := ctx.Value("db").(*gorm.DB)
	maxRetryCnt, _ := ctx.Value("maxRetryCnt").(int)

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
	hostInfo := target.hostInfo

	doc.Find("img").Each(func(imageIndex int, img *goquery.Selection) {
		src, ok := img.Attr("data-src")
		if !ok {
			src, _ = img.Attr("src")
		}

		if src == "" {
			return
		}

		parsedSrc, err := common.ConvertBookSrcURLToAbs(target.parsedURL, src)
		if err != nil {
			log.Warn(err)
			return
		}

		if !parsedSrc.IsAbs() {
			log.Warnf("skip %s, non-absolute URL handling has not yet been implemented", src)
			return
		}

		basename := hostInfo.imageBasenameMaker(ctx, parsedSrc, imageIndex+1, hostInfo.imageFormat)
		outputName := filepath.Join(imgDir, basename)

		fullSrc := parsedSrc.String()
		if dbOk && db != nil && checkShouldSkipImage(db, fullSrc, basename, outputName) {
			log.Debugf("skip: %s", fullSrc)
			return
		}

		dlContext := colly.NewContext()
		dlContext.Put("outputName", outputName)
		dlContext.Put("outputFormat", hostInfo.imageFormat)
		dlContext.Put("bookName", target.title)
		dlContext.Put("volumeName", volumeName)
		dlContext.Put("db", db)
		dlContext.Put("maxRetryCnt", maxRetryCnt)
		dlContext.Put("onResponse", colly.ResponseCallback(saveResponseAsImage))
		dlContext.Put("onError", colly.ErrorCallback(func(resp *colly.Response, err error) {
			retryCnt, retryErr := network.RetryRequest(resp.Request)
			if retryErr == nil {
				log.Warnf("retry(%d) %s: %s", retryCnt, resp.Request.URL, err)
			} else if errors.Is(retryErr, network.ErrMaxRetry) {
				log.Errorf("request failed after %d time(s) of retry %s: %s", retryCnt, resp.Request.URL, err)
			} else {
				log.Errorf("failed to retry request %s: %s", resp.Request.URL, retryErr)
			}
		}))

		var header http.Header
		if hostInfo.headerMaker != nil {
			header = hostInfo.headerMaker(parsedSrc.Hostname())
		}

		collector.Request("GET", src, nil, dlContext, header)
	})

	return nameMap, nil
}

func checkShouldSkipImage(db *gorm.DB, url, basename, outputName string) bool {
	entry := data_model.FileEntry{}
	db.Limit(1).Find(&entry, "url = ?", url)

	if entry.FileName == basename {
		if _, err := os.Stat(outputName); err == nil {
			return true
		}
	}

	return false
}

func saveResponseAsImage(resp *colly.Response) {
	ctx := resp.Ctx

	data, err := network.DecompressResponseBody(resp)
	if err != nil {
		if retryCnt, err := network.RetryRequest(resp.Request); err == nil {
			// pass
		} else if errors.Is(err, network.ErrMaxRetry) {
			log.Errorf("failed to decode response body after %d time(s) of retry %s: %s", retryCnt, resp.Request.URL, err)
		}

		return
	}

	outputName := ctx.Get("outputName")
	if outputName == "" {
		log.Warnf("no image output name found in response context")
		return
	}

	outputFormat := ctx.Get("outputFormat")
	if outputFormat == "" {
		log.Debug("no output format specified, fallback to PNG")
		outputFormat = common.ImageFormatPng
	}

	err = common.SaveImageAs(data, outputName, outputFormat)
	if err != nil {
		log.Warnf("failed to save image with format %s, %s: %s", outputFormat, outputName, err)
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
