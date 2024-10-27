package gelbooru

import (
	"context"
	"fmt"
	urlmod "net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/network"
	collect "github.com/SirZenith/delite/page_collect"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v3"
)

const imgCntPerPage = 42
const gelbooruBaseURL = "https://gelbooru.com/index.php"

var targetExtensions = []string{".jpg", ".png", ".jpeg", ".gif", ".mp4"}

func Cmd() *cli.Command {
	var tagName string
	var fromPage int64
	var toPage int64

	cmd := &cli.Command{
		Name:  "gelbooru",
		Usage: "download images from gelbooru.com, download page range can be specified by starting and ending page number or ending page number alone.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "proxy",
				Usage: "proxy url, e.g. http://127.0.0.1:1080",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "path to output directory",
			},
			&cli.IntFlag{
				Name:  "job",
				Usage: "concurrent download job count",
			},
			&cli.IntFlag{
				Name:  "retry",
				Usage: "retry count for each download",
				Value: 3,
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "request timeout given in seconds",
				Value: 30,
			},
			&cli.DurationFlag{
				Name:  "delay",
				Usage: "request delay in miliseconds",
				Value: 20,
			},
			&cli.StringFlag{
				Name:  "name-map",
				Usage: "name of name map JSON file",
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
				Name:        "pageSt",
				UsageText:   "<page num>",
				Destination: &fromPage,
				Min:         1,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "pageEd",
				UsageText:   "<page num>",
				Destination: &toPage,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			return cmdMain(options, tagName, fromPage, toPage)
		},
	}

	return cmd
}

type options struct {
	proxyURL  string
	outputDir string
	jobCnt    int64
	retryCnt  int64
	timeout   time.Duration
	delay     time.Duration

	fromPage int64
	toPage   int64

	nameMapPath string
}

func getOptionsFromCmd(cmd *cli.Command) (options, error) {
	options := options{
		proxyURL:  cmd.String("proxy"),
		outputDir: cmd.String("output"),
		jobCnt:    cmd.Int("job"),
		retryCnt:  cmd.Int("retry"),
		timeout:   cmd.Duration("timeout"),
		delay:     cmd.Duration("delay"),
	}

	if options.jobCnt <= 0 {
		options.jobCnt = int64(runtime.NumCPU())
	}

	return options, nil
}

func cmdMain(options options, tagName string, fromPage, toPage int64) error {
	if fromPage <= 0 {
		fromPage = 1
	}
	if toPage <= 0 {
		toPage = 1
	}
	if fromPage > toPage {
		fromPage, toPage = toPage, fromPage
	}

	options.fromPage = fromPage
	options.toPage = toPage
	options.outputDir = common.GetStrOr(options.outputDir, tagName)
	options.outputDir = common.InvalidPathCharReplace(options.outputDir)
	options.nameMapPath = common.GetStrOr(options.nameMapPath, filepath.Join(options.outputDir, "name_map.json"))

	if err := os.MkdirAll(options.outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to crate output directory %s: %s", options.outputDir, err)
	}

	collector, global := makeCollector(&options)
	setupCollectorCallback(collector)

	err := visitPostPage(collector, tagName, fromPage)
	if err != nil {
		return fmt.Errorf("can't start collecting: %s", err)
	}

	collector.Wait()
	fmt.Fprint(os.Stderr, "\n")

	global.nameMap.SaveNameMap(options.nameMapPath)

	return nil
}

type ctxGlobal struct {
	collector *colly.Collector
	options   *options
	bar       *progressbar.ProgressBar
	nameMap   *collect.GardedNameMap
}

func makeCollector(options *options) (*colly.Collector, *ctxGlobal) {
	c := colly.NewCollector(
		colly.Async(true),
	)
	c.SetRequestTimeout(options.timeout * time.Second)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "img3.gelbooru.com",
		Delay:       options.delay * time.Millisecond,
		Parallelism: int(options.jobCnt),
	})

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

	// load name map
	nameMap := &collect.GardedNameMap{NameMap: make(map[string]collect.NameMapEntry)}
	if _, err := os.Stat(options.nameMapPath); err == nil {
		nameMap.ReadNameMap(options.nameMapPath)
	}

	global := &ctxGlobal{
		collector: c,
		options:   options,
		bar:       bar,
		nameMap:   nameMap,
	}

	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("global", global)
	})
	c.OnResponse(func(r *colly.Response) {
		if data, err := network.DecompressResponseBody(r); err == nil {
			r.Body = data
		} else {
			log.Errorf("%s", err)
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

	return c, global
}

// downloadFile saves response body to file. Output file name will be read form
// request context.
func downloadFile(r *colly.Response) {
	ctx := r.Ctx

	outputName, ok := ctx.GetAny("outputName").(string)
	if !ok || outputName == "" {
		log.Warnf("can't find output name from download request context:\n\t%s", r.Request.URL)
		return
	}

	if err := os.WriteFile(outputName, r.Body, 0o644); err == nil {
		if global, ok := ctx.GetAny("global").(*ctxGlobal); ok && global != nil {
			global.bar.Add(1)
		}
	} else {
		log.Warnf("failed to save file %s: %s", outputName, err)
	}
}

// setupCollectorCallback registers callbacks for collecting web page elements.
func setupCollectorCallback(c *colly.Collector) {
	c.OnHTML("div.thumbnail-container", onPostPage)
}

// genPageRequest sends requests to each target page through channel. Channel gets
// closed after all requests are sent.
func visitPostPage(collector *colly.Collector, tagName string, pageNum int64) error {
	u, err := urlmod.Parse(gelbooruBaseURL)
	if err != nil {
		return fmt.Errorf("failed to parse base url: %s", err)
	}

	query := u.Query()
	query.Add("page", "post")
	query.Add("s", "list")
	query.Set("tags", tagName)

	pid := (pageNum - 1) * imgCntPerPage
	query.Set("pid", strconv.FormatInt(pid, 10))

	u.RawQuery = query.Encode()

	newCtx := colly.NewContext()
	newCtx.Put("tagName", tagName)
	newCtx.Put("pageNum", pageNum)
	collector.Request("GET", u.String(), nil, newCtx, nil)

	return nil
}

// onPostPage handles post page fetched by colly collector.
func onPostPage(e *colly.HTMLElement) {
	e.ForEach("article.thumbnail-preview a img[src]", onThumbnailEntry)

	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)
	pageNum := ctx.GetAny("pageNum").(int64)
	if pageNum < global.options.toPage {
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

	urlList, err := genTargetListWithThumbnailURL(src)
	if err != nil {
		log.Warnf("failed to generate target list for:\n\t%s\n\t%s", src, err)
	}

	ctx := e.Request.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	entry := global.nameMap.GetEntry(src)
	if checkNameEntryValid(entry, global.options.outputDir) {
		bar := global.bar
		oldMax := bar.GetMax64()
		bar.ChangeMax64(oldMax + 1)

		curProgress := bar.State().CurrentNum
		if curProgress == oldMax {
			bar.Reset()
		}
		bar.Set64(curProgress + 1)

		return
	}

	newCtx := colly.NewContext()
	newCtx.Put("global", ctx.GetAny("global"))
	newCtx.Put("pageNum", ctx.GetAny("pageNum"))
	newCtx.Put("thumbnailURL", src)
	newCtx.Put("imgIndex", imgIndex)
	newCtx.Put("urlList", urlList)
	newCtx.Put("curIndex", int(0))
	newCtx.Put("onResponse", colly.ResponseCallback(sendImageDownloadRequest))

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
		pageNum := ctx.GetAny("pageNum").(int64)
		imgIndex := ctx.GetAny("imgIndex").(int)
		log.Warnf("failed to find available source for p%d-%d", pageNum, imgIndex)
		return
	}

	url := urlList[index]

	ctx.Put("onError", colly.ErrorCallback(func(checkResp *colly.Response, _ error) {
		checkCtx := checkResp.Ctx
		oldIndex := checkCtx.GetAny("curIndex").(int)
		checkCtx.Put("curIndex", oldIndex+1)
		targetImageHeadCheck(checkCtx)
	}))

	ctxGlobal.collector.Request("HEAD", url, nil, ctx, nil)
}

// sendImageDownloadRequest makes a new request again to the same URL of current
// request, and body of new request will be save to file. Output file name is
// will be determined by request's URL path or Last-Modified value in response
// header if available.
func sendImageDownloadRequest(r *colly.Response) {
	ctx := r.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	thumbnailURL := ctx.Get("thumbnailURL")
	global.nameMap.SetMapToTitle(thumbnailURL, r.Request.URL.String())

	bar := global.bar
	oldMax := bar.GetMax64()
	bar.ChangeMax64(oldMax + 1)

	curProgress := bar.State().CurrentNum
	if curProgress == oldMax {
		bar.Reset()
		bar.Set64(curProgress)
	}

	outputName := getImageOutputName(r, thumbnailURL, global.nameMap)
	if (curProgress+1)%imgCntPerPage == 0 {
		go global.nameMap.SaveNameMap(global.options.nameMapPath)
	}

	if outputName == "" {
		bar.Add(1)
		return
	}

	newCtx := colly.NewContext()
	newCtx.Put("global", global)
	newCtx.Put("outputName", outputName)
	newCtx.Put("leftRetryCnt", global.options.retryCnt)
	newCtx.Put("onResponse", colly.ResponseCallback(downloadFile))
	newCtx.Put("onError", colly.ErrorCallback(func(resp *colly.Response, err error) {
		leftRetryCnt := resp.Ctx.GetAny("leftRetryCnt").(int64)
		if leftRetryCnt <= 0 {
			log.Errorf("error requesting %s:\n\t%s", r.Request.URL, err)
			bar.Add(1)
			return
		}

		resp.Ctx.Put("leftRetryCnt", leftRetryCnt-1)
		if err = resp.Request.Retry(); err != nil {
			log.Errorf("failed to retry %s:\n\t%s", r.Request.URL, err)
		}
	}))

	url := r.Request.URL.String()
	global.collector.Request("GET", url, nil, newCtx, nil)
}

// getImageOutputName checks if the image given response should be downloaded.
// When the answer is yes, this function returns output path to be used for
// downloading, else empty string will be returned.
func getImageOutputName(r *colly.Response, thumbnailURL string, nameMap *collect.GardedNameMap) string {
	ctx := r.Ctx
	global := ctx.GetAny("global").(*ctxGlobal)

	basename := path.Base(r.Request.URL.Path)
	outputName := filepath.Join(global.options.outputDir, basename)

	// try to use modified time as file name
	mStr := r.Headers.Get("Last-Modified")
	mTime, timeErr := time.Parse(time.RFC1123, mStr)
	if timeErr == nil {
		dir := filepath.Dir(outputName)
		ext := filepath.Ext(outputName)
		basename = strconv.FormatInt(mTime.Unix(), 10) + ext
		outputName = filepath.Join(dir, basename)
	}

	nameMap.SetMapToFile(thumbnailURL, basename)

	stat, err := os.Stat(outputName)
	if err != nil {
		// can't access local file, re-download it
		return outputName
	}

	if timeErr == nil && stat.ModTime().Before(mTime) {
		// remote file has been updated
		return outputName
	}

	sizeStr := r.Headers.Get("Content-Length")
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err == nil && size != stat.Size() {
		// file size does not match
		return outputName
	}

	return ""
}

// checkNameEntryValid checks if a name map entry is pointing to a valid file
// on disk.
func checkNameEntryValid(entry collect.NameMapEntry, outputDir string) bool {
	filePath := filepath.Join(outputDir, entry.File)
	stat, err := os.Stat(filePath)
	return err == nil && !stat.IsDir()
}
