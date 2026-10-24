package gelbooru

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	urlmod "net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/database"
	"github.com/SirZenith/delite/database/data_model"
	"github.com/SirZenith/delite/network"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v3"
	"gorm.io/gorm"
)

const imgCntPerPage = 42
const gelbooruBaseURL = "https://gelbooru.com/index.php"
const defaultDbName = "library.db"
const imageOutputFormat = common.ImageFormatAvif

// stop advancing post page page number when the number of unfinished task is
// greater then this value.
const postPageProgressThreshold = 500

var targetExtensions = []string{".jpg", ".png", ".jpeg", ".gif", ".mp4", ".webm"}
var noConversionExtensions = []string{".gif", ".mp4", ".webm"}
var targetImageDomain = []string{
	"img4.gelbooru.com",
	"img2.gelbooru.com",
}

var gelbooruImageHeader = map[string][]string{
	"Accept":          {"image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5"},
	"Accept-Encoding": {"deflate, br, zstd"},
	"Priority":        {"u=0, i"},
	"Referer":         {"https://gelbooru.com/"},
}

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "gelbooru",
		Usage: "handling Gelbooru downloads",
		Commands: []*cli.Command{
			subCmdDownloadTag(),
			subCmdDownloadLib(),
			subCmdRetryFailed(),
		},
	}

	return cmd
}

type options struct {
	proxyURL string
	jobCnt   int
	retryCnt int
	timeout  time.Duration
	delay    time.Duration

	limitRule []*colly.LimitRule // a list of requeest limit rule.

	ignoreFalied bool // When set to true, all database entry marked as dl_failed will not be retried
	doTagMigrant bool
}

type tagInfo struct {
	options *options
	db      *gorm.DB

	outputDir string
	tagName   string
	fromPage  int
	toPage    int
}

func subCmdDownloadTag() *cli.Command {
	var tagName string
	var fromPage int64
	var toPage int64

	return &cli.Command{
		Name:  "tag",
		Usage: "download images from gelbooru.com, download page range can be specified by starting and ending page number or ending page number alone.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "db",
				Usage: "path to library database file",
			},
			&cli.DurationFlag{
				Name:  "delay",
				Usage: "request delay",
				Value: 20 * time.Millisecond,
			},
			&cli.IntFlag{
				Name:  "job",
				Usage: "concurrent download job count",
			},
			&cli.StringFlag{
				Name:  "name-map",
				Usage: "name of name map JSON file",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "path to output directory",
			},
			&cli.StringFlag{
				Name:  "proxy",
				Usage: "proxy url, e.g. http://127.0.0.1:1080",
			},
			&cli.IntFlag{
				Name:  "retry",
				Usage: "retry count for each download",
				Value: 3,
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "request timeout",
				Value: 30 * time.Second,
			},
			&cli.BoolFlag{
				Name:  "update",
				Usage: "indicating this download is doing update for existing image collection",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "tag-migrant",
				Usage: "conduct tag migrant update for existing data",
				Value: false,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "tag-name",
				UsageText:   "<tag>",
				Destination: &tagName,
				Min:         1,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "page-st",
				UsageText:   " <page num>",
				Destination: &fromPage,
				Min:         1,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "page-ed",
				UsageText:   " <page num>",
				Destination: &toPage,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			isUpdate := cmd.Bool("update")

			options := options{
				proxyURL: cmd.String("proxy"),
				jobCnt:   int(cmd.Int("job")),
				retryCnt: int(cmd.Int("retry")),
				timeout:  cmd.Duration("timeout"),
				delay:    cmd.Duration("delay"),

				ignoreFalied: isUpdate,
				doTagMigrant: cmd.Bool("tag-migrant"),
			}

			outputDir := cmd.String("output")
			if outputDir == "" {
				outputDir = common.InvalidPathCharReplace(tagName)
			}
			if err := os.MkdirAll(outputDir, 0o777); err != nil {
				return fmt.Errorf("failed to create output directory %s: %s", outputDir, err)
			}

			dbPath := common.GetStrOr(cmd.String("db"), filepath.Join(outputDir, defaultDbName))
			db, err := openDatabase(dbPath)
			if err != nil {
				return err
			}

			if options.jobCnt <= 0 {
				options.jobCnt = runtime.NumCPU()
			}

			if toPage <= 0 {
				if fromPage > 0 {
					toPage = fromPage
					fromPage = 0
				} else {
					toPage = 1
				}
			}
			if fromPage <= 0 {
				fromPage = 1
			}
			if fromPage > toPage {
				fromPage, toPage = toPage, fromPage
			}

			domains := targetImageDomain

			for _, domain := range domains {
				options.limitRule = append(options.limitRule, &colly.LimitRule{
					DomainGlob:  domain,
					Delay:       options.delay,
					Parallelism: options.jobCnt,
				})
			}

			options.limitRule = append(options.limitRule, &colly.LimitRule{
				DomainRegexp: "^gelbooru.com",
				Delay:        100 * time.Millisecond,
			})

			target := tagInfo{
				options: &options,
				db:      db,

				outputDir: common.GetStrOr(outputDir, common.InvalidPathCharReplace(tagName)),
				tagName:   tagName,
				fromPage:  int(fromPage),
				toPage:    int(toPage),
			}

			return downloadPosts(target)
		},
	}
}

func subCmdDownloadLib() *cli.Command {
	var rawKeyword string
	var fromPage int64
	var toPage int64

	return &cli.Command{
		Name:  "lib",
		Usage: "download images from gelbooru.com with information provided in library info file.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path to library info file",
				Value: "./library.json",
			},
			&cli.StringFlag{
				Name:  "proxy",
				Usage: "proxy url, e.g. http://127.0.0.1:1080",
			},
			&cli.IntFlag{
				Name:  "retry",
				Usage: "retry count for each download",
				Value: 3,
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "request timeout",
				Value: 30 * time.Second,
			},
			&cli.BoolFlag{
				Name:  "update",
				Usage: "indicating this download is doing update for existing image collection",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "tag-migrant",
				Usage: "conduct tag migrant update for existing data",
				Value: false,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "tag-keyword",
				UsageText:   "<keyword>",
				Destination: &rawKeyword,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "page-st",
				UsageText:   " <page num>",
				Destination: &fromPage,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "page-ed",
				UsageText:   " <page num>",
				Destination: &toPage,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			isUpdate := cmd.Bool("update")

			options := options{
				proxyURL: cmd.String("proxy"),
				retryCnt: int(cmd.Int("retry")),
				timeout:  cmd.Duration("timeout"),

				ignoreFalied: isUpdate,
				doTagMigrant: cmd.Bool("tag-migrant"),
			}

			libFilePath := cmd.String("library")
			info, err := book_mgr.ReadLibraryInfo(libFilePath)
			if err != nil {
				return err
			}

			for _, rule := range info.LimitRules {
				options.limitRule = append(options.limitRule, rule.ToCollyLimitRule())
			}

			dbPath := common.GetStrOr(info.DatabasePath, filepath.Join(info.RootDir, defaultDbName))
			if err := os.MkdirAll(info.RootDir, 0o777); err != nil {
				return fmt.Errorf("failed to create library root directory %s: %s", info.RootDir, err)
			}
			db, err := openDatabase(dbPath)
			if err != nil {
				return err
			}

			keyword := book_mgr.NewSearchKeyword(rawKeyword)
			targets := []tagInfo{}
			for i, tag := range info.TaggedPosts {
				if !keyword.MatchTaggedPost(i, tag) {
					continue
				}

				tagName := tag.Tag

				target := tagInfo{
					options: &options,
					db:      db,

					outputDir: common.ResolveRelativePath(common.GetStrOr(tag.Title, common.InvalidPathCharReplace(tagName)), info.RootDir),
					tagName:   tagName,
					fromPage:  int(fromPage),
					toPage:    int(toPage),
				}

				if target.toPage <= 0 {
					if target.fromPage > 0 {
						target.toPage = target.fromPage
						target.fromPage = 0
					} else {
						target.toPage = tag.PageCnt
					}
				}

				if target.fromPage <= 0 {
					target.fromPage = 1
				}

				if target.fromPage > target.toPage {
					target.fromPage, target.toPage = target.toPage, target.fromPage
				}

				targets = append(targets, target)
			}

			var errs []error
			for _, target := range targets {
				if err := downloadPosts(target); err != nil {
					log.Warn(err.Error())
					errs = append(errs, fmt.Errorf("%s: %w", target.tagName, err))
				}
			}

			return errors.Join(errs...)
		},
	}
}

func subCmdRetryFailed() *cli.Command {
	var rawKeyword string

	return &cli.Command{
		Name:  "retry-failed",
		Usage: "retry all failed download",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path to library info file",
				Value: "./library.json",
			},
			&cli.StringFlag{
				Name:  "proxy",
				Usage: "proxy url, e.g. http://127.0.0.1:1080",
			},
			&cli.IntFlag{
				Name:  "retry",
				Usage: "retry count for each download",
				Value: 3,
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "request timeout",
				Value: 30 * time.Second,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "tag-keyword",
				UsageText:   "<keyword>",
				Destination: &rawKeyword,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			isUpdate := cmd.Bool("update")

			options := options{
				proxyURL: cmd.String("proxy"),
				retryCnt: int(cmd.Int("retry")),
				timeout:  cmd.Duration("timeout"),

				ignoreFalied: isUpdate,
				doTagMigrant: cmd.Bool("tag-migrant"),
			}

			libFilePath := cmd.String("library")
			info, err := book_mgr.ReadLibraryInfo(libFilePath)
			if err != nil {
				return err
			}

			for _, rule := range info.LimitRules {
				options.limitRule = append(options.limitRule, rule.ToCollyLimitRule())
			}

			dbPath := common.GetStrOr(info.DatabasePath, filepath.Join(info.RootDir, defaultDbName))
			if err := os.MkdirAll(info.RootDir, 0o777); err != nil {
				return fmt.Errorf("failed to create library root directory %s: %s", info.RootDir, err)
			}
			db, err := openDatabase(dbPath)
			if err != nil {
				return err
			}

			keyword := book_mgr.NewSearchKeyword(rawKeyword)
			targets := []tagInfo{}
			for i, tag := range info.TaggedPosts {
				if !keyword.MatchTaggedPost(i, tag) {
					continue
				}

				tagName := tag.Tag

				target := tagInfo{
					options: &options,
					db:      db,

					outputDir: common.ResolveRelativePath(common.GetStrOr(tag.Title, common.InvalidPathCharReplace(tagName)), info.RootDir),
					tagName:   tagName,
				}

				targets = append(targets, target)
			}

			var errs []error
			for _, target := range targets {
				if err := retryAllFailedDownloadForTarget(target); err != nil {
					log.Warnf("error occured while retrying %s: %s\n", target.tagName, err)
					errs = append(errs, fmt.Errorf("%s: %w", target.tagName, err))
				}
			}

			return errors.Join(errs...)
		},
	}
}

func openDatabase(dbPath string) (*gorm.DB, error) {
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database %s: %s", dbPath, err)
	}
	return db, nil
}

func downloadPosts(target tagInfo) error {
	log.Infof("%s: [%d, %d] -> %s", target.tagName, target.fromPage, target.toPage, target.outputDir)

	if err := os.MkdirAll(target.outputDir, 0o777); err != nil {
		return fmt.Errorf("failed to crate output directory %s: %s", target.outputDir, err)
	}

	collector, global, err := makeCollector(&target)
	if err != nil {
		return err
	}
	setupCollectorCallback(collector)

	err = visitPostPage(collector, target.tagName, target.fromPage, target.options.retryCnt)
	if err != nil {
		return fmt.Errorf("can't start collecting: %s", err)
	}

	collector.Wait()
	fmt.Fprint(os.Stderr, "\n")

	if err := global.pageError(); err != nil {
		return err
	}

	successCnt := global.successCnt.Load()
	noSourceCnt := global.noSourceCnt.Load()
	log.Infof("%s: downloaded %d image(s), %d post(s) without available source",
		target.tagName, successCnt, noSourceCnt)

	if successCnt == 0 && noSourceCnt > 0 {
		return fmt.Errorf("failed to download any image for %s: %d post(s) had no available source",
			target.tagName, noSourceCnt)
	}

	return nil
}

type ctxGlobal struct {
	collector *colly.Collector
	target    *tagInfo
	bar       *progressbar.ProgressBar

	unfinishedTaskCnt  int64
	lockUnfinishedTask sync.Mutex

	lockPageError sync.Mutex
	pageErrors    []error

	successCnt  atomic.Int64
	noSourceCnt atomic.Int64
}

// addPageError records an error raised while collecting a post page.
func (g *ctxGlobal) addPageError(pageNum int, err error) {
	g.lockPageError.Lock()
	defer g.lockPageError.Unlock()

	g.pageErrors = append(g.pageErrors, fmt.Errorf("page %d: %w", pageNum, err))
}

// pageError returns all errors recorded while collecting post pages.
func (g *ctxGlobal) pageError() error {
	g.lockPageError.Lock()
	defer g.lockPageError.Unlock()

	return errors.Join(g.pageErrors...)
}

// changeProgressMax add delta to max number of task to progress bar.
func changeProgressMax(bar *progressbar.ProgressBar, delta int64) {
	state := bar.State()

	newMax := state.Max + delta
	bar.ChangeMax64(newMax)

	if state.CurrentNum == state.Max {
		bar.Reset()

		if newMax > state.CurrentNum {
			bar.Set64(state.CurrentNum)
		} else {
			bar.Set64(newMax)
		}
	}
}

// changeUnfinishedTaskCnt updates unfinished task counter with given difference.
func changeUnfinishedTaskCnt(global *ctxGlobal, delta int64) {
	global.lockUnfinishedTask.Lock()
	defer global.lockUnfinishedTask.Unlock()

	global.unfinishedTaskCnt += delta

	bar := global.bar
	if delta > 0 {
		changeProgressMax(bar, 1)
	} else if delta < 0 {
		bar.Add(1)
	}
}

func setPageNumDescription(bar *progressbar.ProgressBar, pageNum int) {
	bar.Describe(fmt.Sprintf("page: %d |", pageNum))
}

func makeCollector(target *tagInfo) (*colly.Collector, *ctxGlobal, error) {
	c := colly.NewCollector(
		colly.Async(true),
		// colly limits response bodies to 10MB by default, which truncates
		// large images. 0 means no limit.
		colly.MaxBodySize(0),
	)
	c.SetRequestTimeout(target.options.timeout)

	// Clone the default transport so that HTTP_PROXY / HTTPS_PROXY / NO_PROXY
	// keep working when no proxy is given explicitly.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if target.options.proxyURL != "" {
		proxyURL, err := urlmod.Parse(target.options.proxyURL)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid proxy url %q: %s", target.options.proxyURL, err)
		}

		if proxyURL.Scheme == "" {
			proxyURL.Scheme = "http"
		}
		if proxyURL.Host == "" {
			return nil, nil, fmt.Errorf("invalid proxy url %q: missing host", target.options.proxyURL)
		}

		switch proxyURL.Scheme {
		case "http", "https", "socks5":
		default:
			return nil, nil, fmt.Errorf("unsupported proxy scheme %q in %q",
				proxyURL.Scheme, target.options.proxyURL)
		}

		transport.Proxy = http.ProxyURL(proxyURL)
		log.Debugf("using proxy %s", proxyURL.Redacted())
	}
	c.WithTransport(transport)

	if len(target.options.limitRule) > 0 {
		c.Limits(target.options.limitRule)
	}

	bar := progressbar.NewOptions64(
		0,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(5),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)

	global := &ctxGlobal{
		collector: c,
		target:    target,
		bar:       bar,
	}

	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("global", global)
	})
	c.OnResponse(func(r *colly.Response) {
		if data, err := network.DecompressResponseBody(r); err == nil {
			r.Body = data
		} else {
			bar.Describe(err.Error() + "\n")
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
			bar.Describe(fmt.Sprintf("error requesting %s: %s\n", r.Request.URL, err))
		}
	})

	return c, global, nil
}

// setupCollectorCallback registers callbacks for collecting web page elements.
func setupCollectorCallback(c *colly.Collector) {
	c.OnHTML("div.thumbnail-container", onPostPage)
}

// genPageRequest sends requests to each target page through channel. Channel gets
// closed after all requests are sent.
func visitPostPage(collector *colly.Collector, tagName string, pageNum, retryCnt int) error {
	u, err := urlmod.Parse(gelbooruBaseURL)
	if err != nil {
		return fmt.Errorf("failed to parse base url: %s", err)
	}

	query := u.Query()
	query.Add("page", "post")
	query.Add("s", "list")
	query.Set("tags", tagName)

	pid := (pageNum - 1) * imgCntPerPage
	query.Set("pid", strconv.Itoa(pid))

	u.RawQuery = query.Encode()

	newCtx := colly.NewContext()
	newCtx.Put("tagName", tagName)
	newCtx.Put("pageNum", pageNum)

	newCtx.Put("leftRetryCnt", retryCnt)
	newCtx.Put("onError", colly.ErrorCallback(func(resp *colly.Response, err error) {
		atPage := resp.Ctx.GetAny("pageNum").(int)
		leftRetryCnt := resp.Ctx.GetAny("leftRetryCnt").(int)
		global := resp.Ctx.GetAny("global").(*ctxGlobal)

		if leftRetryCnt <= 0 {
			log.Warnf("failed to advance post at page %d: %s", atPage, err)
			global.addPageError(atPage, err)
			return
		}

		resp.Ctx.Put("leftRetryCnt", leftRetryCnt-1)
		if err = resp.Request.Retry(); err != nil {
			log.Warnf("failed to advance post at page %d: %s", atPage, err)
			global.addPageError(atPage, err)
		}
	}))

	if err := collector.Request("GET", u.String(), nil, newCtx, nil); err != nil {
		return fmt.Errorf("failed to request post page %d: %s", pageNum, err)
	}

	return nil
}

// onPostPage handles post page fetched by colly collector.
func onPostPage(e *colly.HTMLElement) {
	runtime.GC()

	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	pageNum := ctx.GetAny("pageNum").(int)
	setPageNumDescription(global.bar, pageNum)

	e.ForEach("article.thumbnail-preview a img[src]", onThumbnailEntry)

	if pageNum < global.target.toPage {
		// When there are too many unfinished tasks, stop adding new ones.
		// Since adding tasks is often musch faster then consuming them, this loop
		// prevents image downloading goroutines get starved and from this program
		// taking up too much memory.
		for true {
			if global.unfinishedTaskCnt > postPageProgressThreshold {
				time.Sleep(time.Second)
			} else {
				break
			}
		}

		tagName := ctx.GetAny("tagName").(string)
		if err := visitPostPage(global.collector, tagName, pageNum+1, global.target.options.retryCnt); err != nil {
			global.addPageError(pageNum+1, err)
		}
	}
}

// onThumbnailEntry handles thumbnail element found by colly collector.
func onThumbnailEntry(imgIndex int, e *colly.HTMLElement) {
	src := e.Attr("src")
	if src == "" {
		return
	}

	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	urlList, err := genTargetListWithThumbnailURL(src)
	if err != nil {
		global.bar.Describe(fmt.Sprintf("failed to generate target list for:\n\t%s\n\t%s\n", src, err))
	}

	db := global.target.db
	entry := &data_model.TaggedPostEntry{}
	db.Limit(1).Find(entry, "thumbnail_url = ?", src)

	if checkNameEntryValid(entry, global.target.outputDir, global.target.options) {
		if global.target.options.doTagMigrant {
			db.Model(entry).Update("tag", ctx.Get("tagName"))
		}

		bar := global.bar
		setPageNumDescription(bar, ctx.GetAny("pageNum").(int))
		changeProgressMax(bar, 1)
		bar.Add64(1)
		return
	}

	if entry.ContentURL != "" {
		urlList = slices.Insert(urlList, 0, entry.ContentURL)
	}

	newCtx := colly.NewContext()
	newCtx.Put("global", ctx.GetAny("global"))
	newCtx.Put("tagName", ctx.Get("tagName"))
	newCtx.Put("pageNum", ctx.GetAny("pageNum"))
	newCtx.Put("thumbnailURL", src)
	newCtx.Put("imgIndex", imgIndex)
	newCtx.Put("urlList", urlList)
	newCtx.Put("curIndex", int(0))
	if global.target.options.doTagMigrant {
		newCtx.Put("onResponse", colly.ResponseCallback(tagMigrantDummyImageDownload))
	} else {
		newCtx.Put("onResponse", colly.ResponseCallback(sendImageDownloadRequest))
	}
	newCtx.Put("onError", colly.ErrorCallback(onTargetImageHeadCheckFailed))

	changeUnfinishedTaskCnt(global, 1)

	targetImageHeadCheck(newCtx)
}

// genTargetListWithThumbnailURL generates a list of potential target URL with
// given thumbnail URL.
func genTargetListWithThumbnailURL(thumbnailURL string) ([]string, error) {
	url, err := urlmod.Parse(thumbnailURL)
	if err != nil {
		return nil, err
	}

	// thumbnail URLs are absolute, first element of segments will be empty string
	segments := strings.Split(url.Path, "/")
	if len(segments) <= 2 {
		return nil, nil
	}

	domains := make([]string, 0, len(targetImageDomain)+1)
	if url.Host != "" {
		domains = append(domains, url.Host)
	}
	for _, domain := range targetImageDomain {
		if domain != url.Host {
			domains = append(domains, domain)
		}
	}

	baseURLList := []string{}

	// prefer pictures from *images*, they are better on quality
	baseURLList = thumbnailURLToImageURL(url, segments, baseURLList, domains)
	baseURLList = thumbnailURLToSampleURL(url, segments, baseURLList, domains)

	result := []string{}
	for _, baseURL := range baseURLList {
		ext := path.Ext(baseURL)
		if ext != "" {
			baseURL = baseURL[:len(baseURL)-len(ext)]
		}

		for _, targetExt := range targetExtensions {
			result = append(result, baseURL+targetExt)
		}
	}

	return result, nil
}

// thumbnailURLToSampleURL converts a thumbnail URL to a samples URL on one of
// the given domains, this function assume `segments` has more then 2 elements.
func thumbnailURLToSampleURL(url *urlmod.URL, segments, result, domains []string) []string {
	mainEndPoint := segments[1]
	if mainEndPoint == "thumbnails" {
		mainEndPoint = "samples"
	}

	segmentCnt := len(segments)
	filename := segments[segmentCnt-1]
	if strings.HasPrefix(filename, "thumbnail") {
		filename = "sample" + filename[9:]
	}

	newSegments := []string{}
	newSegments = append(newSegments, "", mainEndPoint)
	newSegments = append(newSegments, segments[2:segmentCnt-1]...)
	newSegments = append(newSegments, filename)
	newPath := strings.Join(newSegments, "/")

	for _, domain := range domains {
		scheme := url.Scheme
		if scheme == "" {
			scheme = "https"
		}
		target := scheme + "://" + domain + newPath
		result = append(result, target)
	}

	return result
}

// thumbnailURLToImageURL converts a thumbnail URL to an images URL on one of the
// given domains, this function assume `segments` has more then 2 elements.
func thumbnailURLToImageURL(url *urlmod.URL, segments, result, domains []string) []string {
	mainEndPoint := segments[1]
	if mainEndPoint == "thumbnails" {
		mainEndPoint = "images"
	}

	segmentCnt := len(segments)
	filename := segments[segmentCnt-1]
	if strings.HasPrefix(filename, "thumbnail_") {
		filename = filename[10:]
	}

	newSegments := []string{}
	newSegments = append(newSegments, "", mainEndPoint)
	newSegments = append(newSegments, segments[2:segmentCnt-1]...)
	newSegments = append(newSegments, filename)
	newPath := strings.Join(newSegments, "/")

	for _, domain := range domains {
		scheme := url.Scheme
		if scheme == "" {
			scheme = "https"
		}
		target := scheme + "://" + domain + newPath
		result = append(result, target)
	}

	return result
}

// targetImageHeadCheck recursively check existance of a list of image URLs by
// sending HEAD request to target URL. An `onResponse` callback should be provided
// through context to determine what to do when on of URL in target list is valid.
func targetImageHeadCheck(ctx *colly.Context) {
	ctxGlobal := ctx.GetAny("global").(*ctxGlobal)
	urlList := ctx.GetAny("urlList").([]string)
	index := ctx.GetAny("curIndex").(int)

	if index >= len(urlList) {
		changeUnfinishedTaskCnt(ctxGlobal, -1)
		ctxGlobal.noSourceCnt.Add(1)

		pageNum := ctx.GetAny("pageNum").(int)
		imgIndex := ctx.GetAny("imgIndex").(int)
		ctxGlobal.bar.Describe(fmt.Sprintf("failed to find available source for p%d-%d\n", pageNum, imgIndex))

		return
	}

	url := urlList[index]

	ctxGlobal.collector.Request("HEAD", url, nil, ctx, gelbooruImageHeader)
}

// onTargetImageHeadCheckFailed advances head check target index by one and resend
// head check request.
func onTargetImageHeadCheckFailed(checkResp *colly.Response, err error) {
	checkCtx := checkResp.Ctx
	oldIndex := checkCtx.GetAny("curIndex").(int)
	checkCtx.Put("curIndex", oldIndex+1)
	targetImageHeadCheck(checkCtx)
}

// sendImageDownloadRequest makes a new request again to the same URL of current
// request, and body of new request will be save to file. Output file name is
// will be determined by request's URL path or Last-Modified value in response
// header if available.
func sendImageDownloadRequest(r *colly.Response) {
	ctx := r.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)
	pageNum := ctx.GetAny("pageNum").(int)
	thumbnailURL := ctx.Get("thumbnailURL")

	urlList := ctx.GetAny("urlList").([]string)
	index := ctx.GetAny("curIndex").(int)
	contentUrl := urlList[index]

	outputName, basename := getImageOutputName(r, contentUrl)
	if outputName == "" {
		changeUnfinishedTaskCnt(global, -1)
		return
	}

	entry := &data_model.TaggedPostEntry{
		ThumbnailURL: thumbnailURL,
		ContentURL:   contentUrl,
		FileName:     basename,
		Tag:          ctx.Get("tagName"),
	}
	global.target.db.Save(entry)

	newCtx := makeImageDownloadContext(ctx, global, pageNum, outputName, contentUrl, func(ok bool) {
		changeUnfinishedTaskCnt(global, -1)

		global.target.db.Model(entry).Update("DlFailed", !ok)
	})

	global.collector.Request("GET", contentUrl, nil, newCtx, gelbooruImageHeader)
}

// makeImageDownloadContext makes new contenxt for initiate image download.
func makeImageDownloadContext(parentCtx *colly.Context, global *ctxGlobal, pageNum int, outputName string, contentUrl string, onFinished func(ok bool)) *colly.Context {
	bar := global.bar

	finish := func(ok bool) {
		if ok {
			global.successCnt.Add(1)
		}
		onFinished(ok)
	}

	newCtx := colly.NewContext()
	newCtx.Put("parentCtx", parentCtx)
	newCtx.Put("global", global)
	newCtx.Put("leftRetryCnt", global.target.options.retryCnt)

	newCtx.Put("onResponse", colly.ResponseCallback(func(resp *colly.Response) {
		if len(resp.Body) == 0 {
			bar.Describe(fmt.Sprintf("image data has no byte %s\n", contentUrl))
			finish(false)
			return
		}

		err := saveImageDownloadResult(resp.Body, outputName)
		if err == nil {
			setPageNumDescription(bar, pageNum)
		} else {
			bar.Describe(err.Error() + "\n")
		}

		finish(err == nil)
	}))

	newCtx.Put("onError", colly.ErrorCallback(func(resp *colly.Response, err error) {
		leftRetryCnt := resp.Ctx.GetAny("leftRetryCnt").(int)
		if leftRetryCnt <= 0 {
			bar.Describe(fmt.Sprintf("error requesting %s: %s\n", contentUrl, err))
			finish(false)

			checkCtx := resp.Ctx.GetAny("parentCtx").(*colly.Context)
			if checkCtx != nil {
				oldIndex := checkCtx.GetAny("curIndex").(int)
				checkCtx.Put("curIndex", oldIndex+1)
				targetImageHeadCheck(checkCtx)
			}
			return
		}

		resp.Ctx.Put("leftRetryCnt", leftRetryCnt-1)
		if err = resp.Request.Retry(); err != nil {
			bar.Describe(fmt.Sprintf("failed to retry %s:\n\t%s\n", contentUrl, err))
			finish(false)
		}
	}))

	return newCtx
}

func saveImageDownloadResult(data []byte, outputName string) error {
	ext := filepath.Ext(outputName)
	if !slices.Contains(noConversionExtensions, ext) {
		return common.SaveImageAs(data, outputName, imageOutputFormat)
	}

	file, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %s", outputName, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	_, err = writer.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data to file: %s", err)
	}

	return nil
}

func tagMigrantDummyImageDownload(r *colly.Response) {
	ctx := r.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	thumbnailURL := ctx.Get("thumbnailURL")

	urlList := ctx.GetAny("urlList").([]string)
	index := ctx.GetAny("curIndex").(int)
	contentUrl := urlList[index]

	outputName, basename := getImageOutputName(r, contentUrl)
	if outputName != "" {
		db := global.target.db
		entry := &data_model.TaggedPostEntry{
			ThumbnailURL: thumbnailURL,
			ContentURL:   r.Request.URL.String(),
			FileName:     basename,
			Tag:          ctx.Get("tagName"),
		}
		db.Save(entry)
	}

	changeUnfinishedTaskCnt(global, -1)
}

// getImageOutputName checks if the image given response should be downloaded.
// When the answer is yes, this function returns output path to be used for
// downloading, else empty string will be returned.
func getImageOutputName(r *colly.Response, contentUrl string) (string, string) {
	ctx := r.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	basename := path.Base(contentUrl)
	ext := filepath.Ext(basename)

	// try to use modified time as file name
	mStr := r.Headers.Get("Last-Modified")
	mTime, timeErr := time.Parse(time.RFC1123, mStr)
	if timeErr == nil {
		basename = strconv.FormatInt(mTime.Unix(), 10) + ext
	}

	if !slices.Contains(noConversionExtensions, ext) {
		basename = common.ReplaceFileExt(basename, "."+imageOutputFormat)
	}
	outputName := filepath.Join(global.target.outputDir, basename)

	stat, err := os.Stat(outputName)
	if err != nil {
		// can't access local file, re-download it
		return outputName, basename
	}

	if timeErr == nil && stat.ModTime().Before(mTime) {
		// remote file has been updated
		return outputName, basename
	}

	sizeStr := r.Headers.Get("Content-Length")
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err == nil && size != stat.Size() {
		// file size does not match
		return outputName, basename
	}

	return "", basename
}

// checkNameEntryValid checks if a name map entry is pointing to a valid file
// on disk.
func checkNameEntryValid(entry *data_model.TaggedPostEntry, outputDir string, options *options) bool {
	if entry.MarkDeleted {
		return true
	}

	if entry.FileName == "" {
		return false
	}

	if options.ignoreFalied && entry.DlFailed {
		return true
	}

	filePath := filepath.Join(outputDir, entry.FileName)
	stat, err := os.Stat(filePath)

	return err == nil && stat.Mode().IsRegular()
}

type retryTask struct {
	contenteUrl string
	fileName    string
}

func retryAllFailedDownloadForTarget(target tagInfo) error {
	log.Infof("Retrying %s -> %s", target.tagName, target.outputDir)

	outputDir := target.outputDir
	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return err
	}

	taskChan := make(chan retryTask, 100)
	errChan := make(chan error, 1)
	go func() { errChan <- findAllFailedDownloads(target, taskChan) }()

	collector, ctxGlobal, err := makeCollector(&target)
	if err != nil {
		return err
	}

	bar := ctxGlobal.bar
	var failedCnt atomic.Int64
	db := target.db

	taskCnt := 0
	var enqueueErrs []error
	for task := range taskChan {
		taskCnt++
		changeProgressMax(ctxGlobal.bar, 1)

		outputName := filepath.Join(outputDir, task.fileName)
		onTaskFinished := func(ok bool) {
			bar.Add(1)
			if !ok {
				failedCnt.Add(1)
				return
			}

			// Clear the failure flag so that a successful retry is not
			// reported (and retried) again.
			db.Model(&data_model.TaggedPostEntry{}).
				Where("content_url = ? AND file_name = ?", task.contenteUrl, task.fileName).
				Update("dl_failed", false)
		}
		newCtx := makeImageDownloadContext(nil, ctxGlobal, 0, outputName, task.contenteUrl, onTaskFinished)

		if err := collector.Request("GET", task.contenteUrl, nil, newCtx, gelbooruImageHeader); err != nil {
			bar.Add(1)
			enqueueErrs = append(enqueueErrs, fmt.Errorf("failed to request %s: %w", task.contenteUrl, err))
		}
	}

	collector.Wait()

	dbErr := <-errChan
	if err := errors.Join(append(enqueueErrs, dbErr)...); err != nil {
		return err
	}
	if n := failedCnt.Load(); n > 0 {
		return fmt.Errorf("%d of %d failed download(s) still failed", n, taskCnt)
	}
	return nil
}

func findAllFailedDownloads(target tagInfo, taskChan chan retryTask) error {
	defer close(taskChan)

	db := target.db
	entry := &data_model.TaggedPostEntry{}

	rows, err := db.Model(entry).Where("tag = ? AND dl_failed == ?", target.tagName, 1).Rows()
	if err != nil {
		return fmt.Errorf("failed to query failed tasks for %s: %s", target.tagName, err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := db.ScanRows(rows, entry); err != nil {
			return fmt.Errorf("failed to scan failed task for %s: %s", target.tagName, err)
		}

		taskChan <- retryTask{
			contenteUrl: entry.ContentURL,
			fileName:    entry.FileName,
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate failed tasks for %s: %s", target.tagName, err)
	}

	return nil
}
