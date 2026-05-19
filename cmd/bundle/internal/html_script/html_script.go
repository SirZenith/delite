package html_script

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"

	book_mgr "github.com/SirZenith/delite/book_management"
	bundle_common "github.com/SirZenith/delite/cmd/bundle/internal/common"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/database"
	"github.com/SirZenith/delite/database/data_model"
	"github.com/SirZenith/delite/format/epub"
	format_html "github.com/SirZenith/delite/format/html"
	"github.com/SirZenith/delite/format/latex"
	luamodule "github.com/SirZenith/delite/lua_module"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"gorm.io/gorm"
)

const outputAssetDirName = "assets"

func Cmd() *cli.Command {
	var (
		rawKeyword  string
		volumeIndex int64
	)

	return &cli.Command{
		Name:  "html-script",
		Usage: "bundle novel files with Lua script. Return value of given script will be used as conversion definition used in HTML conversion.",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "job",
				Aliases: []string{"j"},
				Value:   int64(runtime.NumCPU()),
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path to library info JSON file",
				Value: "./library.json",
			},
			&cli.StringFlag{
				Name:  "preprocess",
				Usage: "path to preprocess Lua script",
			},
			&cli.StringFlag{
				Name:     "converter",
				Usage:    "path to converter Lua script",
				Required: true,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "book-keyword",
				UsageText:   "<book>",
				Destination: &rawKeyword,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "volume-index",
				UsageText:   " <volume-index>",
				Destination: &volumeIndex,
				Value:       -1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, targets, err := getOptionsFromCmd(cmd, rawKeyword, int(volumeIndex))
			if err != nil {
				return err
			}

			return cmdMain(options, targets)
		},
	}
}

type options struct {
	jobCnt int

	cliPreprocessScript string
	converterScript     string
}

type workerTask struct {
	ctx        context.Context
	volumeName string
	target     *bookInfo
}

type bookInfo struct {
	rootDir       string
	textDir       string
	imageDir      string
	epubDir       string
	isEpubSrc     bool
	isUnsupported bool

	bookTitle string
	author    string
	tocURL    *url.URL
	dbPath    string

	targetVolume int
}

type volumeInfo struct {
	book      string
	volume    string
	fullTitle string
	author    string

	rootDir string

	// HTML book
	textDir string
	imgDir  string

	// ePub book
	epubFile string

	preprocessScript string
	converterScript  string
}

func getOptionsFromCmd(cmd *cli.Command, rawKeyword string, volumeIndex int) (options, []bookInfo, error) {
	options := options{
		jobCnt: int(cmd.Int("job")),

		cliPreprocessScript: cmd.String("preprocess"),
		converterScript:     cmd.String("converter"),
	}

	libFilePath := cmd.String("library")
	targets, err := loadLibraryTargets(&options, libFilePath, rawKeyword, volumeIndex)
	if err != nil {
		return options, targets, err
	}

	return options, targets, nil
}

// loadLibraryTargets reads book list from library info JSON and returns them
// as a list of MakeBookTarget.
func loadLibraryTargets(options *options, libInfoPath string, rawKeyword string, volumeIndex int) ([]bookInfo, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	keyword := book_mgr.NewSearchKeyword(rawKeyword)
	targets := []bookInfo{}
	for index, book := range info.Books {
		if !keyword.MatchBook(index, book) {
			continue
		}

		tocURL, _ := url.Parse(book.TocURL)

		target := bookInfo{
			rootDir:  book.RootDir,
			textDir:  book.TextDir,
			imageDir: book.ImgDir,
			epubDir:  book.EpubDir,

			bookTitle: book.Title,
			author:    book.Author,
			tocURL:    tocURL,
			dbPath:    info.DatabasePath,

			targetVolume: volumeIndex,
		}

		if book.LocalInfo != nil {
			switch book.LocalInfo.Type {
			case book_mgr.LocalBookTypeEpub:
				target.isEpubSrc = true
				target.isUnsupported = false
			case book_mgr.LocalBookTypeHTML:
				target.isEpubSrc = false
				target.isUnsupported = false
			default:
				target.isEpubSrc = false
				target.isUnsupported = true
			}
		}

		targets = append(targets, target)
	}

	return targets, nil
}

func cmdMain(options options, targets []bookInfo) error {
	var group sync.WaitGroup
	taskChan := make(chan workerTask, options.jobCnt)

	for i := options.jobCnt; i > 0; i-- {
		go buildWorker(taskChan, &group)
	}

	buildBoss(&options, targets, taskChan, &group)

	group.Wait()

	return nil
}

func getBookPreprocessScript(cliScript string, bookScript string) string {
	if cliScript != "" {
		return cliScript
	}

	if bookScript != "" {
		return bookScript
	}

	return ""
}

// logWorkBeginBanner prints a banner indicating a new download of book starts.
func logWorkBeginBanner(target bookInfo) {
	msgs := []string{
		fmt.Sprintf("%-12s: %s", "title", target.bookTitle),
		fmt.Sprintf("%-12s: %s", "author", target.author),
		fmt.Sprintf("%-12s: %s", "text   dir", target.textDir),
		fmt.Sprintf("%-12s: %s", "image  dir", target.imageDir),
	}

	common.LogBannerMsg(msgs, 5)
}

// ----------------------------------------------------------------------------

func buildBoss(options *options, targets []bookInfo, taskChan chan workerTask, group *sync.WaitGroup) {
	var err error

	for _, target := range targets {
		logWorkBeginBanner(target)

		if target.isUnsupported {
			log.Info("skip unsupported resource")
			continue
		}

		if target.isEpubSrc {
			err = buildFromEpubBoss(options, target, taskChan, group)
		} else {
			err = buildFromHTMLBoss(options, target, taskChan, group)
		}

		if err != nil {
			log.Errorf("%s", err)
		}
	}
}

func buildWorker(taskChan chan workerTask, group *sync.WaitGroup) {
	for task := range taskChan {
		if task.target.isEpubSrc {
			buildFromEpubWorker(task)
		} else {
			buildFromHTMLWorker(task)
		}
		group.Done()
	}
}

// ----------------------------------------------------------------------------

func buildFromHTMLBoss(options *options, target bookInfo, taskChan chan workerTask, group *sync.WaitGroup) error {
	entryList, err := os.ReadDir(target.textDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %s", target.textDir, err)
	}

	var db *gorm.DB
	if target.dbPath != "" {
		db, err = database.Open(target.dbPath)
		if err != nil {
			return fmt.Errorf("failed to open book db: %s", err)
		}
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "preprocessScript", options.cliPreprocessScript)
	ctx = context.WithValue(ctx, "converterScript", options.converterScript)
	ctx = context.WithValue(ctx, "db", db)
	ctx = context.WithValue(ctx, "url", target.tocURL)

	for index, child := range entryList {
		if target.targetVolume > 0 && index+1 != target.targetVolume {
			continue
		}

		group.Add(1)

		taskChan <- workerTask{
			ctx:        ctx,
			target:     &target,
			volumeName: child.Name(),
		}
	}

	return nil
}

func buildFromHTMLWorker(task workerTask) {
	ctx := task.ctx
	target := task.target
	volumeName := task.volumeName

	preprocessScript := ctx.Value("preprocessScript").(string)
	converterScript := ctx.Value("converterScript").(string)

	var title string
	if volumeName == bundle_common.SingleVolumeName {
		title = target.bookTitle
	} else {
		title = fmt.Sprintf("%s %s", target.bookTitle, volumeName)
	}

	textDir := filepath.Join(target.textDir, volumeName)
	imgDir := filepath.Join(target.imageDir, volumeName)

	err := bundleBook(ctx, volumeInfo{
		book:      target.bookTitle,
		volume:    volumeName,
		fullTitle: title,
		author:    target.author,

		rootDir: target.rootDir,
		textDir: textDir,
		imgDir:  imgDir,

		preprocessScript: preprocessScript,
		converterScript:  converterScript,
	})

	if err != nil {
		log.Warnf("conversion failed %s: %s", title, err)
	} else {
		log.Infof("conversion done: %s", title)
	}
}

func bundleBook(ctx context.Context, info volumeInfo) error {
	ls, stateInfo, err := luamodule.MakeConverterLuaState(info.converterScript, luamodule.ConversionArgs{
		ScriptDir:  filepath.Dir(info.converterScript),
		ScriptPath: info.converterScript,

		BookRoot:       info.rootDir,
		SourceFileName: filepath.Base(info.textDir),
		Book:           info.book,
		Volume:         info.volume,
		FullTitle:      info.fullTitle,
		Author:         info.author,
	})
	if ls != nil {
		defer ls.Close()
	}

	if err != nil {
		return fmt.Errorf("failed to prepare Lua state for converter script: %s", err)
	}

	relativeImgDir, err := filepath.Rel(stateInfo.Meta.OutputDir, info.imgDir)
	if err != nil {
		return fmt.Errorf("failed to get realtive path of image asset directory: %s", err)
	}

	nodes, err := readTextFiles(info.textDir)
	if err != nil {
		return err
	}

	ctx = context.WithValue(ctx, "imgDir", info.imgDir)
	ctx = context.WithValue(ctx, "relativeImgDir", relativeImgDir)

	sizeMap := map[string]*image.Point{}
	var (
		errList []error
		nodeOk  bool
	)
	for i := range nodes {
		nodeOk, errList = replaceImgSrc(ctx, nodes[i], sizeMap, errList)
		if !nodeOk {
			nodes[i].Type = html.CommentNode
		}
	}
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	for _, node := range nodes {
		format_html.SetImageSizeMeta(node, sizeMap)
		format_html.SetImageTypeMeta(node)
		format_html.UnescapleAllTextNode(node)
		format_html.SetListLevelMeta(node, 0, false)
	}

	if info.preprocessScript != "" {
		meta := luamodule.PreprocessMeta{
			OutputDir:      stateInfo.Meta.OutputDir,
			SourceFileName: filepath.Base(info.textDir),
			Book:           info.book,
			Volume:         info.volume,
			Title:          info.fullTitle,
			Author:         info.author,
		}
		if processed, err := luamodule.RunPreprocessScript(nodes, info.preprocessScript, meta); err == nil {
			nodes = processed
		} else {
			return err
		}
	}

	err = os.MkdirAll(stateInfo.Meta.OutputDir, 0o777)
	if err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", stateInfo.Meta.OutputDir, err)
	}

	return runConverterScript(ls, stateInfo, nodes)
}

func readTextFiles(textDir string) ([]*html.Node, error) {
	entryList, err := os.ReadDir(textDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read text directory %s: %s", textDir, err)
	}

	names := []string{}
	for _, child := range entryList {
		if !child.IsDir() {
			name := child.Name()
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var nodes []*html.Node
	for _, name := range names {
		ext := filepath.Ext(name)
		if ext != ".html" && ext != ".xhtml" {
			continue
		}

		fullPath := filepath.Join(textDir, name)
		if node, err := readTextFile(fullPath); err == nil {
			nodes = append(nodes, node)
		} else {
			fmt.Printf("failed to add %s: %s\n", fullPath, err)
		}
	}

	return nodes, nil
}

// Adds one text file to epub. Before adding it, src attribute value of all `img`
// gets replaced with internal image path.
// Any error happens during the process will be returned.
func readTextFile(fileName string) (*html.Node, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	tree, err := html.Parse(reader)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// Parses given text as HTML, and replace all `img` tags' `src` attribute value
// with internal image path used by epub.
func replaceImgSrc(ctx context.Context, node *html.Node, sizeMap map[string]*image.Point, errList []error) (bool, []error) {
	var childOk bool

	imgDir := ctx.Value("imgDir").(string)
	relativeImgDir := ctx.Value("relativeImgDir").(string)

	child := node.FirstChild
	for child != nil {
		nextChild := child.NextSibling

		childOk, errList = replaceImgSrc(ctx, child, sizeMap, errList)
		if !childOk {
			node.RemoveChild(child)
		}

		child = nextChild
	}

	if node.Type != html.ElementNode || node.DataAtom != atom.Img {
		return true, errList
	}

	srcAttr := html_util.GetNodeAttr(node, "src")
	dataSrcAttr := html_util.GetNodeAttr(node, "data-src")

	basename := ""
	var src string
	if dataSrcAttr != nil {
		basename = getMappedImageName(ctx, dataSrcAttr.Val)
		src = dataSrcAttr.Val
	} else if srcAttr != nil {
		basename = getMappedImageName(ctx, srcAttr.Val)
		src = srcAttr.Val
	}

	if basename == "" {
		errList = append(errList, fmt.Errorf("image reference can't not be found in database, src: %q", src))
		return false, errList
	}

	mapTo := filepath.Join(relativeImgDir, basename)
	mapTo = filepath.ToSlash(mapTo)

	if srcAttr == nil {
		node.Attr = append(node.Attr, html.Attribute{
			Key: "src",
			Val: mapTo,
		})
	} else {
		srcAttr.Val = mapTo
	}

	imgPath := filepath.Join(imgDir, basename)
	size, err := getImageSize(imgPath)
	if err != nil {
		errList = append(errList, err)
		return false, errList
	}

	sizeMap[mapTo] = size

	return true, errList
}

func getMappedImageName(ctx context.Context, src string) string {
	db := ctx.Value("db").(*gorm.DB)
	toclURL := ctx.Value("url").(*url.URL)

	var basename string
	var parsedSrc *url.URL
	var err error

	if toclURL != nil {
		parsedSrc, err = common.ConvertBookSrcURLToAbs(toclURL, src)
	}

	if err != nil || !parsedSrc.IsAbs() {
		basename = path.Base(src)
	} else if db == nil {
		basename = common.ReplaceFileExt(path.Base(parsedSrc.Path), ".png")
	} else {
		entry := data_model.FileEntry{}
		db.Limit(1).Find(&entry, "url = ?", parsedSrc.String())
		basename = entry.FileName
	}

	return basename
}

func getImageSize(filePath string) (*image.Point, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %s", filePath, err)
	}
	defer file.Close()

	fileReader := bufio.NewReader(file)
	img, _, err := image.Decode(fileReader)
	if err != nil {
		return nil, fmt.Errorf("can't decode image %s: %s", filePath, err)
	}

	size := img.Bounds().Size()

	return &size, nil
}

// ----------------------------------------------------------------------------

func buildFromEpubBoss(options *options, target bookInfo, taskChan chan workerTask, group *sync.WaitGroup) error {
	entryList, err := os.ReadDir(target.epubDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %s", target.epubDir, err)
	}

	epubNamePrefix := common.InvalidPathCharReplace(target.bookTitle)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "preprocessScript", options.cliPreprocessScript)
	ctx = context.WithValue(ctx, "converterScript", options.converterScript)
	ctx = context.WithValue(ctx, "epubNamePrefix", epubNamePrefix)

	for index, child := range entryList {
		if target.targetVolume > 0 && index+1 != target.targetVolume {
			continue
		}

		group.Add(1)

		taskChan <- workerTask{
			ctx:        ctx,
			target:     &target,
			volumeName: child.Name(),
		}
	}

	return nil
}

func buildFromEpubWorker(task workerTask) {
	ctx := task.ctx
	target := task.target
	epubName := task.volumeName

	preprocessScript := ctx.Value("preprocessScript").(string)
	converterScript := ctx.Value("converterScript").(string)
	epubNamePrefix := ctx.Value("epubNamePrefix").(string)

	ext := filepath.Ext(epubName)
	volumeName := epubName[:len(epubName)-len(ext)]
	if strings.HasPrefix(volumeName, epubNamePrefix) {
		volumeName = strings.TrimLeftFunc(volumeName[len(epubNamePrefix):], unicode.IsSpace)
		if volumeName == "" {
			volumeName = bundle_common.SingleVolumeName
		}
	}

	var title string
	if volumeName == bundle_common.SingleVolumeName {
		title = target.bookTitle
	} else {
		title = fmt.Sprintf("%s %s", target.bookTitle, volumeName)
	}

	err := extractEpub(volumeInfo{
		book:      target.bookTitle,
		volume:    volumeName,
		fullTitle: title,
		author:    target.author,

		rootDir:  target.rootDir,
		epubFile: filepath.Join(target.epubDir, epubName),

		preprocessScript: preprocessScript,
		converterScript:  converterScript,
	})

	if err != nil {
		log.Warnf("conversion failed %s: %s", title, err)
	} else {
		log.Infof("conversion done: %s", title)
	}
}

func extractEpub(info volumeInfo) error {
	ls, stateInfo, err := luamodule.MakeConverterLuaState(info.converterScript, luamodule.ConversionArgs{
		ScriptDir:  filepath.Dir(info.converterScript),
		ScriptPath: info.converterScript,

		BookRoot:       info.rootDir,
		SourceFileName: filepath.Base(info.epubFile),
		Book:           info.book,
		Volume:         info.volume,
		FullTitle:      info.fullTitle,
		Author:         info.author,
	})

	if ls != nil {
		defer ls.Close()
	}

	if err != nil {
		return fmt.Errorf("failed to prepare Lua state for converter script: %s", err)
	}

	err = os.MkdirAll(stateInfo.Meta.OutputDir, 0o777)
	if err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", stateInfo.Meta.OutputDir, err)
	}

	return epub.Merge(epub.EpubMergeOptions{
		EpubFile:     info.epubFile,
		OutputDir:    stateInfo.Meta.OutputDir,
		AssetDirName: stateInfo.Meta.AssetDirBasename,

		JobCnt: runtime.NumCPU(),

		PreprocessFunc: func(nodes []*html.Node) ([]*html.Node, error) {
			// TODO: this preprocess is not specific to LaTeX format, extract it
			// as a common function.
			nodes = latex.FromEpubPreprocess(nodes, latex.FromEpubOptions{
				OutputDir: stateInfo.Meta.OutputDir,

				Title:  info.fullTitle,
				Author: info.author,
			})

			if info.preprocessScript != "" {
				meta := luamodule.PreprocessMeta{
					OutputDir:      stateInfo.Meta.OutputDir,
					SourceFileName: filepath.Base(info.epubFile),
					Book:           info.book,
					Volume:         info.volume,
					Title:          info.fullTitle,
					Author:         info.author,
				}

				if processed, err := luamodule.RunPreprocessScript(nodes, info.preprocessScript, meta); err == nil {
					nodes = processed
				} else {
					return nil, err
				}
			}

			return nodes, nil
		},
		SaveOutputFunc: func(nodes []*html.Node, _ string, _ string) error {
			return runConverterScript(ls, stateInfo, nodes)
		},
	})
}

// ----------------------------------------------------------------------------

func runConverterScript(ls *lua.LState, stateInfo *luamodule.ConverterStateInfo, nodes []*html.Node) error {
	result := luamodule.RunConverterScript(ls, *stateInfo, nodes)

	// write output file
	outputName := filepath.Join(stateInfo.Meta.OutputDir, stateInfo.Meta.OutputFileBasename)
	outFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)

	for ele := result.Front(); ele != nil; ele = ele.Next() {
		switch v := ele.Value.(type) {
		case lua.LValue:
			outWriter.WriteString(v.String())
		case string:
			outWriter.WriteString(v)
		default:
		}
	}

	err = outWriter.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nil
}
