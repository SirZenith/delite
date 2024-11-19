package nhentai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	book_mgr "github.com/SirZenith/delite/book_management"
	nhentai "github.com/SirZenith/delite/cmd/nhentai/internal"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
)

const defaultRetryCnt = 3

func Cmd() *cli.Command {
	return &cli.Command{
		Name:  "nhentai",
		Usage: "download manga fron nhentai with book ID",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "output",
				Usage: "path to output directory",
				Value: "./",
			},
			&cli.StringFlag{
				Name:  "proxy",
				Usage: "proxy URL, e.g. http://127.0.0.1:1080, this proxy will be used by both HTTP request and HTTPS request",
			},
			&cli.IntFlag{
				Name:  "retry",
				Usage: "max retry count for each page",
			},
			&cli.IntFlag{
				Name:  "job",
				Usage: "concurrent downlad job count",
			},
			&cli.IntFlag{
				Name:  "id",
				Usage: "target book id",
			},
			&cli.IntFlag{
				Name:  "start",
				Usage: "starting page number for downloading, 1-base index, effective for ID provided by command line flag",
				Value: 1,
			},
			&cli.IntFlag{
				Name:  "list-file",
				Usage: "file with a list of ID to be downloaded, each line contains one ID",
			},
			&cli.StringFlag{
				Name:  "header",
				Usage: "header JSON file, in form of Array<{ name: string, value: string }>",
			},
			&cli.BoolFlag{
				Name:  "no-dump-info",
				Usage: "save book info to JSON after download",
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to JSON config file",
			},
		},
		Commands: []*cli.Command{
			subCmdParseTitle(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			return cmdMain(options)
		},
	}
}

type DlTask struct {
	ID     int
	StPage int // starting page number for downloading, 1-base index, inclusive
}

type options struct {
	httpProxy  string // proxy used by http request during downloading
	httpsProxy string // proxy used by https request during downloading
	jobCount   int64  // goroutine amount used by downloader
	retryCount int64  // retry count for each manga page if any error is encountered

	headerFile string // path to header json file
	headers    map[string]string

	outputDir string // directory to download manga pages to
	task      *DlTask
	listFile  string

	dumpInfo bool
}

func getOptionsFromCmd(cmd *cli.Command) (options, error) {
	options := options{
		httpProxy:  cmd.String("proxy"),
		httpsProxy: cmd.String("proxy"),
		jobCount:   cmd.Int("job"),
		retryCount: cmd.Int("retry"),

		headerFile: cmd.String("header"),
		headers:    map[string]string{},

		outputDir: cmd.String("output"),
		listFile:  cmd.String("list-file"),

		dumpInfo: !cmd.Bool("no-dump-info"),
	}

	configPath := cmd.String("config")
	if configPath != "" {
		err := loadOptionsFromConfig(&options, configPath)
		if err != nil {
			return options, err
		}
	}

	if options.headerFile != "" {
		err := readHeaderFile(options.headerFile, options.headers)
		if err != nil {
			log.Warnf("failed to read header file: %s", err)
		}
	}

	bookID := cmd.Int("id")
	if bookID > 0 {
		options.task = &DlTask{
			ID:     int(bookID),
			StPage: int(cmd.Int("start")),
		}
	}

	setupDefualtOptioinValue(&options)

	return options, nil
}

func loadOptionsFromConfig(options *options, configPath string) error {
	config, err := book_mgr.ReadConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %s", configPath, err)
	}

	options.httpProxy = common.GetStrOr(options.httpProxy, config.HttpProxy)
	options.httpsProxy = common.GetStrOr(options.httpsProxy, config.HttpsProxy)

	if options.jobCount <= 0 {
		options.jobCount = int64(config.JobCount)
	}

	if options.retryCount <= 0 {
		options.retryCount = int64(config.RetryCount)
	}

	options.outputDir = common.GetStrOr(options.outputDir, config.OutputDir)
	options.listFile = common.GetStrOr(options.listFile, config.TargetList)

	options.headerFile = common.GetStrOr(options.headerFile, config.HeaderFile)

	return nil
}

func setupDefualtOptioinValue(options *options) {
	if options.jobCount <= 0 {
		options.jobCount = int64(runtime.NumCPU())
	}

	if options.retryCount <= 0 {
		options.retryCount = defaultRetryCnt
	}
}

func cmdMain(options options) error {
	downloader := nhentai.NewDownloader(int(options.jobCount), int(options.retryCount))
	downloader.InitClient(options.headers, options.httpProxy, options.httpsProxy)

	if options.task != nil {
		if err := dlBook(downloader, options, *options.task); err != nil {
			log.Errorf("failed to download %d: %s", options.task.ID, err)
		}
	}

	if options.listFile != "" {
		dlFromList(downloader, options, options.listFile)
	}

	return nil
}

type headerValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Reads header value from file and stores them into the map passed as argument.
// Header file should a JSON containing array of header objects. Each header
// objects should be object with tow string field `name` and `value`.
func readHeaderFile(path string, result map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read header file %s: %s", path, err)
	}

	list := []headerValue{}
	err = json.Unmarshal(data, &list)
	if err != nil {
		return fmt.Errorf("failed to parse header %s: %s", path, err)
	}

	for _, entry := range list {
		key := strings.ToLower(entry.Name)
		result[key] = entry.Value
	}

	return nil
}

func dlBook(downloader *nhentai.Downloader, options options, task DlTask) error {
	err := downloader.GetBook(task.ID)
	if err != nil {
		return err
	}

	log.Infof("Book ID: %d", downloader.CurBookId)
	log.Infof("Title: %s", downloader.Title)
	log.Infof("Page: %d", downloader.Book.NumPages)

	title := downloader.Title
	outputDir, err := common.FindAvailableFileName(options.outputDir, title, "", 100)
	if err != nil {
		return fmt.Errorf("failed to find available output directory name for %s: %s", title, err)
	}

	if err := os.MkdirAll(outputDir, 0o777); err != nil {
		return fmt.Errorf("failed to crate download output directory %s: %s", outputDir, err)
	}

	if options.dumpInfo {
		infoPath := filepath.Join(outputDir, "info.json")
		if err = downloader.DumpBookInfo(infoPath); err != nil {
			log.Warnf("failed to save book info for %s: %s", downloader.CurBookId, err)
		}
	}

	err = downloader.StartDownload(outputDir, task.StPage)
	if err != nil {
		return err
	}

	return nil
}

// dlFromList tries download all target listed in target file. Each line should
// contain book ID and an optional starting page number, separated by space.
func dlFromList(downloader *nhentai.Downloader, options options, listName string) {
	records := map[int]bool{}
	task := DlTask{}

	for {
		err := readDlList(listName, &task)
		if err != nil {
			log.Fatalf("failed to read download list: %s", err)
		}

		if records[task.ID] {
			log.Warnf("found repeated book ID: %d, quit downloading", task.ID)
			break
		}

		if task.ID <= 0 {
			log.Info("read empty book ID, quit downloading")
			break
		}

		err = dlBook(downloader, options, task)
		if err != nil {
			log.Errorf("failed to download book %d: %s", task.ID, err)
		}

		records[task.ID] = true

		appendBookId := 0
		if err != nil {
			appendBookId = task.ID
		}

		writeDlList(listName, task.ID, appendBookId)
	}
}

// readDlList reads one line from target list file, and update task struct according
// to content of the line.
func readDlList(listName string, task *DlTask) error {
	file, err := os.Open(listName)
	if err != nil {
		return fmt.Errorf("can't optn file %s: %s", listName, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	scanner.Scan()

	line := scanner.Text()

	err = parseDlLine(line, task)
	if err != nil {
		return fmt.Errorf("failed to parse target line:\n\t%s\n\t%s", line, err)
	}

	return nil
}

// writeDlList reads all lines in list file, remove the line corresponds to
// `removeBookId` from its content, and add `appendBookId` to end of the list if
// its value is greater than zero.
func writeDlList(listName string, removeBookId int, appendBookId int) error {
	file, err := os.OpenFile(listName, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return fmt.Errorf("failed to open target list file %s: %s", listName, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	lines := []string{}

	task := DlTask{}
	for scanner.Scan() {
		line := scanner.Text()
		err := parseDlLine(line, &task)
		if err != nil || task.ID != removeBookId {
			lines = append(lines, line)
		}
	}

	if appendBookId > 0 {
		lines = append(lines, strconv.Itoa(appendBookId))
	}

	file.Seek(0, 0)
	file.Truncate(0)
	writer := bufio.NewWriter(file)
	for i, line := range lines {
		if i != 0 {
			writer.WriteRune('\n')
		}

		_, err := writer.WriteString(line)
		if err != nil {
			log.Warnf("failed to write target line:\n\t%s\n\t%s", line, err)
		}
	}

	err = writer.Flush()
	if err != nil {
		log.Warnf("failed to flush target list content: %s", err)
	}

	return nil
}

// parseDlLine updates a DlTask struct according to content of target line.
func parseDlLine(line string, task *DlTask) error {
	parts := strings.Split(strings.TrimSpace(line), " ")
	cnt := len(parts)
	if cnt > 2 {
		return fmt.Errorf("too many arguments")
	}

	idStr := strings.TrimLeft(parts[0], "#")
	if idStr == "" {
		task.ID = 0
		return nil
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return fmt.Errorf("invalid book id %s: %s", idStr, err)
	}
	task.ID = id

	if len(parts) == 1 {
		task.StPage = 1
	} else if len(parts) == 2 {
		numStr := parts[1]

		if numStr == "" {
			task.StPage = 1
		} else {
			st, err := strconv.Atoi(numStr)
			if err != nil {
				return fmt.Errorf("invalid page number %s: %s", parts[1], err)
			}
			task.StPage = st
		}
	}

	return nil
}

func subCmdParseTitle() *cli.Command {
	var title string

	cmd := &cli.Command{
		Name:  "parse-title",
		Usage: "parse nhentai manga title, prints its normal form",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "title",
				UsageText:   "<title>",
				Destination: &title,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			normalized := nhentai.GetMangaTitle(title)
			fmt.Println(normalized)
			return nil
		},
	}

	return cmd
}
