package gelbooru

import (
	"context"
	"fmt"
	urlmod "net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
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

var targetExtensions = []string{".jpg", ".png", ".jpeg", ".gif"}

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

			dbPath := common.GetStrOr(cmd.String("db"), filepath.Join(outputDir, defaultDbName))
			db, err := database.Open(dbPath)
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
				jobCnt:   int(cmd.Int("job")),
				retryCnt: int(cmd.Int("retry")),
				timeout:  cmd.Duration("timeout"),
				delay:    cmd.Duration("delay"),

				ignoreFalied: isUpdate,
				doTagMigrant: cmd.Bool("tag-migrant"),
			}

			if options.jobCnt <= 0 {
				options.jobCnt = runtime.NumCPU()
			}

			libFilePath := cmd.String("library")
			info, err := book_mgr.ReadLibraryInfo(libFilePath)
			if err != nil {
				return err
			}

			dbPath := common.GetStrOr(info.DatabasePath, filepath.Join(info.RootDir, defaultDbName))
			db, err := database.Open(dbPath)
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

					outputDir: common.GetStrOr(tag.Title, common.InvalidPathCharReplace(tagName)),
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

			for _, target := range targets {
				if err := downloadPosts(target); err != nil {
					log.Warn(err.Error())
				}
			}

			return nil
		},
	}
}

func subCmdRetryFailed() *cli.Command {
	var rawKeyword string

	return &cli.Command{
		Name:  "retry-failed",
		Usage: "retry all failed download",
		Flags: []cli.Flag{
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
				jobCnt:   int(cmd.Int("job")),
				retryCnt: int(cmd.Int("retry")),
				timeout:  cmd.Duration("timeout"),
				delay:    cmd.Duration("delay"),

				ignoreFalied: isUpdate,
				doTagMigrant: cmd.Bool("tag-migrant"),
			}

			if options.jobCnt <= 0 {
				options.jobCnt = runtime.NumCPU()
			}

			libFilePath := cmd.String("library")
			info, err := book_mgr.ReadLibraryInfo(libFilePath)
			if err != nil {
				return err
			}

			dbPath := common.GetStrOr(info.DatabasePath, filepath.Join(info.RootDir, defaultDbName))
			db, err := database.Open(dbPath)
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

					outputDir: common.GetStrOr(tag.Title, common.InvalidPathCharReplace(tagName)),
					tagName:   tagName,
				}

				targets = append(targets, target)
			}

			for _, target := range targets {
				if err := retryAllFailedDownloadForTarget(target); err != nil {
					log.Warnf("error occured while retrying %s: %s\n", target.tagName, err)
				}
			}

			return nil
		},
	}
}

func downloadPosts(target tagInfo) error {
	log.Infof("%s: [%d, %d] -> %s", target.tagName, target.fromPage, target.toPage, target.outputDir)

	if err := os.MkdirAll(target.outputDir, 0o777); err != nil {
		return fmt.Errorf("failed to crate output directory %s: %s", target.outputDir, err)
	}

	collector, _ := makeCollector(&target)
	setupCollectorCallback(collector)

	err := visitPostPage(collector, target.tagName, target.fromPage)
	if err != nil {
		return fmt.Errorf("can't start collecting: %s", err)
	}

	collector.Wait()
	fmt.Fprint(os.Stderr, "\n")

	return nil
}

type ctxGlobal struct {
	collector *colly.Collector
	target    *tagInfo
	bar       *progressbar.ProgressBar

	unfinishedTaskCnt  int64
	lockUnfinishedTask sync.Mutex
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

func makeCollector(target *tagInfo) (*colly.Collector, *ctxGlobal) {
	c := colly.NewCollector(
		colly.Async(true),
	)
	c.SetRequestTimeout(target.options.timeout)

	domains := []string{
		"img.gelbooru.com",
		"img2.gelbooru.com",
		"img3.gelbooru.com",
		"img4.gelbooru.com",
	}

	for _, domain := range domains {
		c.Limit(&colly.LimitRule{
			DomainGlob:  domain,
			Delay:       target.options.delay,
			Parallelism: target.options.jobCnt,
		})
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
			bar.Describe(err.Error())
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
			bar.Describe(fmt.Sprintf("error requesting %s: %s", r.Request.URL, err))
		}
	})

	return c, global
}

// setupCollectorCallback registers callbacks for collecting web page elements.
func setupCollectorCallback(c *colly.Collector) {
	c.OnHTML("div.thumbnail-container", onPostPage)
}

// genPageRequest sends requests to each target page through channel. Channel gets
// closed after all requests are sent.
func visitPostPage(collector *colly.Collector, tagName string, pageNum int) error {
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
	collector.Request("GET", u.String(), nil, newCtx, nil)

	return nil
}

// onPostPage handles post page fetched by colly collector.
func onPostPage(e *colly.HTMLElement) {
	runtime.GC()

	e.ForEach("article.thumbnail-preview a img[src]", onThumbnailEntry)

	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)
	pageNum := ctx.GetAny("pageNum").(int)
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
		visitPostPage(global.collector, tagName, pageNum+1)
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
		global.bar.Describe(fmt.Sprintf("failed to generate target list for:\n\t%s\n\t%s", src, err))
	}

	db := global.target.db
	entry := &data_model.TaggedPostEntry{}
	db.Limit(1).Find(entry, "thumbnail_url = ?", src)

	if checkNameEntryValid(entry, global.target.outputDir, global.target.options) {
		if global.target.options.doTagMigrant {
			db.Model(entry).Update("tag", ctx.Get("tagName"))
		}

		bar := global.bar
		bar.Describe("")
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

	baseURLList := []string{
		// prefer pictures from img3.gelbooru.com/images, they are better on quality
		thumbnailURLToImageURL(url, segments),
		thumbnailURLToSampleURL(url, segments),
	}

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

// thumbnailURLToSampleURL converts a thumbnail URL to img3.gelbooru.com/samples
// URL, this function assume `segments` has more then 2 elements.
func thumbnailURLToSampleURL(url *urlmod.URL, segments []string) string {
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

	newURL := *url
	newURL.Path = path.Join(newSegments...)

	return newURL.String()
}

// thumbnailURLToSampleURL converts a thumbnail URL to img3.gelbooru.com/images
// URL, this function assume `segments` has more then 2 elements.
func thumbnailURLToImageURL(url *urlmod.URL, segments []string) string {
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

	newURL := *url
	newURL.Path = path.Join(newSegments...)

	return newURL.String()
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

		pageNum := ctx.GetAny("pageNum").(int)
		imgIndex := ctx.GetAny("imgIndex").(int)
		ctxGlobal.bar.Describe(fmt.Sprintf("failed to find available source for p%d-%d", pageNum, imgIndex))

		return
	}

	url := urlList[index]

	ctxGlobal.collector.Request("HEAD", url, nil, ctx, nil)
}

// onTargetImageHeadCheckFailed advances head check target index by one and resend
// head check request.
func onTargetImageHeadCheckFailed(checkResp *colly.Response, _ error) {
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

	thumbnailURL := ctx.Get("thumbnailURL")

	outputName, basename := getImageOutputName(r)
	if outputName == "" {
		changeUnfinishedTaskCnt(global, -1)
		return
	}

	contentUrl := r.Request.URL.String()

	db := global.target.db
	entry := &data_model.TaggedPostEntry{
		ThumbnailURL: thumbnailURL,
		ContentURL:   contentUrl,
		FileName:     basename,
		Tag:          ctx.Get("tagName"),
	}
	db.Save(entry)

	newCtx := makeImageDownloadContext(global, outputName, contentUrl, func(ok bool) {
		changeUnfinishedTaskCnt(global, -1)
		updateDlFailedMark(db, contentUrl, !ok)
	})

	global.collector.Request("GET", contentUrl, nil, newCtx, nil)
}

// makeImageDownloadContext makes new contenxt for initiate image download.
func makeImageDownloadContext(global *ctxGlobal, outputName string, contentUrl string, onFinished func(ok bool)) *colly.Context {
	bar := global.bar

	newCtx := colly.NewContext()
	newCtx.Put("global", global)
	newCtx.Put("leftRetryCnt", global.target.options.retryCnt)

	newCtx.Put("onResponse", colly.ResponseCallback(func(resp *colly.Response) {
		err := common.SaveImageAs(resp.Body, outputName, imageOutputFormat)
		if err == nil {
			bar.Describe("")
		} else {
			bar.Describe(fmt.Sprintf("failed to save image %s: %s\n", outputName, err))
		}

		onFinished(err == nil)
	}))

	newCtx.Put("onError", colly.ErrorCallback(func(resp *colly.Response, err error) {
		leftRetryCnt := resp.Ctx.GetAny("leftRetryCnt").(int)
		if leftRetryCnt <= 0 {
			bar.Describe(fmt.Sprintf("error requesting %s:\n\t%s", contentUrl, err))
			onFinished(false)
			return
		}

		resp.Ctx.Put("leftRetryCnt", leftRetryCnt-1)
		if err = resp.Request.Retry(); err != nil {
			bar.Describe(fmt.Sprintf("failed to retry %s:\n\t%s", contentUrl, err))
			onFinished(false)
		}
	}))

	return newCtx
}

// updateDlFailedMark updates dl_failed mark value of given target.
func updateDlFailedMark(db *gorm.DB, contentUrl string, isFailed bool) {
	db.Model(&data_model.TaggedPostEntry{}).Where("content_url = ?", contentUrl).Update("dl_failed", isFailed)
}

func tagMigrantDummyImageDownload(r *colly.Response) {
	ctx := r.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	thumbnailURL := ctx.Get("thumbnailURL")

	outputName, basename := getImageOutputName(r)
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
func getImageOutputName(r *colly.Response) (string, string) {
	ctx := r.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	basename := path.Base(r.Request.URL.Path)
	basename = common.ReplaceFileExt(basename, "."+imageOutputFormat)
	outputName := filepath.Join(global.target.outputDir, basename)

	// try to use modified time as file name
	mStr := r.Headers.Get("Last-Modified")
	mTime, timeErr := time.Parse(time.RFC1123, mStr)
	if timeErr == nil {
		dir := filepath.Dir(outputName)
		basename = strconv.FormatInt(mTime.Unix(), 10) + "." + imageOutputFormat
		outputName = filepath.Join(dir, basename)
	}

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
	go findAllFailedDownloads(target, taskChan)

	collector, ctxGlobal := makeCollector(&target)

	bar := ctxGlobal.bar
	onTaskFinished := func(_ok bool) {
		bar.Add(1)
	}

	for task := range taskChan {
		changeProgressMax(ctxGlobal.bar, 1)

		outputName := filepath.Join(outputDir, task.fileName)
		newCtx := makeImageDownloadContext(ctxGlobal, outputName, task.contenteUrl, onTaskFinished)

		collector.Request("GET", task.contenteUrl, nil, newCtx, nil)
	}

	collector.Wait()

	return nil
}

func findAllFailedDownloads(target tagInfo, taskChan chan retryTask) {
	defer close(taskChan)

	db := target.db
	entry := &data_model.TaggedPostEntry{}

	rows, err := db.Model(entry).Where("tag = ? AND dl_failed == ?", target.tagName, 1).Rows()
	if err != nil {
		log.Warnf("failed to query failed tasks for %s: %s\n", target.tagName, err)
		return
	}

	defer rows.Close()

	for rows.Next() {
		db.ScanRows(rows, &entry)

		taskChan <- retryTask{
			contenteUrl: entry.ContentURL,
			fileName:    entry.FileName,
		}
	}
}
