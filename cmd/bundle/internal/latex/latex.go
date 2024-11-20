package latex

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

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/database"
	"github.com/SirZenith/delite/database/data_model"
	"github.com/SirZenith/delite/format/epub"
	format_html "github.com/SirZenith/delite/format/html"
	"github.com/SirZenith/delite/format/latex"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"gorm.io/gorm"
)

const outputAssetDirName = "assets"

const defaultLatexTemplte = `
\documentclass{ltjtbook}

\usepackage{
    afterpage,
    geometry,
    graphicx,
    hyperref,
    luatexja-fontspec,
    pdfpages,
    pxrubrica,
    url,
}

\setmainjfont{SourceHanSerif-Medium}

\rubysetup{g}

\geometry{
	paperwidth = 12cm,
	paperheight = 16cm,
    top = 1.5cm,
    bottom = 1.5cm,
    left = 1.2cm,
    right = 1.2cm,
}
`

const latexOutputBasename = "book"

func Cmd() *cli.Command {
	var rawKeyword string
	var volumeIndex int64

	return &cli.Command{
		Name:  "latex",
		Usage: "bundle downloaded novel files into LaTex file with infomation provided in info.json of the book",
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
				Name:    "template-file",
				Aliases: []string{"T"},
				Usage:   "path to file containing template string, ignored when `template` flag has non-empty value.",
			},
			&cli.StringFlag{
				Name:    "template",
				Aliases: []string{"t"},
				Usage: strings.Join([]string{
					"output template string, should be preamble content of output file, e.g. content before `\\began{document}` command",
				}, "\n"),
			},
			&cli.StringFlag{
				Name:  "preprocess",
				Usage: "path to preprocess Lua script",
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
				UsageText:   "<volume-index>",
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

	cliTemplate         string
	cliPreprocessScript string

	libTemplate string
}

type workerTask struct {
	ctx        context.Context
	volumeName string
	target     *bookInfo
}

type bookInfo struct {
	textDir       string
	imageDir      string
	epubDir       string
	outputDir     string
	isLocal       bool
	isUnsupported bool

	templateFile     string
	preprocessScript string

	bookTitle string
	author    string
	tocURL    *url.URL
	dbPath    string

	targetVolume int
}

type volumeInfo struct {
	book   string
	volume string
	title  string
	author string

	outputDir      string
	outputBaseName string
	textDir        string
	imgDir         string
	relativeImgDir string

	template         string
	preprocessScript string
}

type localVolumeInfo struct {
	book   string
	volume string
	title  string
	author string

	epubFile       string
	outputDir      string
	outputBaseName string
	assetDirName   string

	jobCnt           int
	template         string
	preprocessScript string
}

func getOptionsFromCmd(cmd *cli.Command, rawKeyword string, volumeIndex int) (options, []bookInfo, error) {
	options := options{
		jobCnt: int(cmd.Int("job")),

		cliTemplate:         cmd.String("template"),
		cliPreprocessScript: cmd.String("preprocess"),
	}

	templateFile := cmd.String("template-file")
	if options.cliTemplate != "" {
		// pass
	} else if templateFile != "" {
		data, err := os.ReadFile(templateFile)
		if err != nil {
			return options, nil, fmt.Errorf("failed to read template file %s: %s", templateFile, err)
		}

		options.cliTemplate = string(data)
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

	if info.LatexConfig.TemplateFile != "" {
		data, err := os.ReadFile(info.LatexConfig.TemplateFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read template file %s: %s", info.LatexConfig.TemplateFile, err)
		}

		options.libTemplate = string(data)
	}

	keyword := book_mgr.NewSearchKeyword(rawKeyword)
	targets := []bookInfo{}
	for index, book := range info.Books {
		if !keyword.MatchBook(index, book) {
			continue
		}

		tocURL, _ := url.Parse(book.TocURL)

		target := bookInfo{
			textDir:   book.TextDir,
			imageDir:  book.ImgDir,
			epubDir:   book.EpubDir,
			outputDir: book.LatexDir,

			bookTitle: book.Title,
			author:    book.Author,
			tocURL:    tocURL,
			dbPath:    info.DatabasePath,

			targetVolume: volumeIndex,
		}

		if book.LocalInfo != nil {
			ok := book.LocalInfo.Type == book_mgr.LocalBookTypeEpub
			target.isLocal = ok
			target.isUnsupported = !ok
		}

		if book.LatexInfo != nil {
			latexInfo := book.LatexInfo
			target.templateFile = latexInfo.TemplateFile
			target.preprocessScript = latexInfo.PreprocessScript
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

func getBookTemplate(cliTemplate string, libTemplate string, bookTemplateFile string) (string, error) {
	if cliTemplate != "" {
		return cliTemplate, nil
	}

	if bookTemplateFile == "" {
		if libTemplate != "" {
			return libTemplate, nil
		} else {
			return defaultLatexTemplte, nil
		}
	}

	data, err := os.ReadFile(bookTemplateFile)
	if err != nil {
		return "", fmt.Errorf("failed to read template file %s: %s", bookTemplateFile, err)
	}

	return string(data), nil
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
		fmt.Sprintf("%-12s: %s", "output dir", target.outputDir),
	}

	common.LogBannerMsg(msgs, 5)
}

// ----------------------------------------------------------------------------

func buildBoss(options *options, targets []bookInfo, taskChan chan workerTask, group *sync.WaitGroup) {
	for _, target := range targets {
		logWorkBeginBanner(target)

		if target.isUnsupported {
			log.Info("skip unsupported resource")
			continue
		}

		err := os.MkdirAll(target.outputDir, 0o777)
		if err != nil {
			log.Errorf("failed to create output directory %s: %s", target.outputDir, err)
			continue
		}

		if target.isLocal {
			err = buildLocalBoss(options, target, taskChan, group)
		} else {
			err = buildRemoteBoss(options, target, taskChan, group)
		}

		if err != nil {
			log.Errorf("%s", err)
		}
	}
}

func buildWorker(taskChan chan workerTask, group *sync.WaitGroup) {
	for task := range taskChan {
		if task.target.isLocal {
			buildLocalWorker(task)
		} else {
			buildRemoteWorker(task)
		}
		group.Done()
	}
}

// ----------------------------------------------------------------------------

func buildRemoteBoss(options *options, target bookInfo, taskChan chan workerTask, group *sync.WaitGroup) error {
	template, err := getBookTemplate(options.cliTemplate, options.libTemplate, target.templateFile)
	if err != nil {
		return err
	}

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

	preprocessScript := getBookPreprocessScript(options.cliPreprocessScript, target.preprocessScript)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "template", template)
	ctx = context.WithValue(ctx, "preprocessScript", preprocessScript)
	ctx = context.WithValue(ctx, "db", db)
	ctx = context.WithValue(ctx, "url", target.tocURL)

	for index, child := range entryList {
		if target.targetVolume >= 0 && index != target.targetVolume {
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

func buildRemoteWorker(task workerTask) {
	ctx := task.ctx
	target := task.target
	volumeName := task.volumeName

	template := ctx.Value("template").(string)
	preprocessScript := ctx.Value("preprocessScript").(string)

	outputDir := filepath.Join(target.outputDir, volumeName)
	err := os.MkdirAll(outputDir, 0o777)
	if err != nil {
		log.Errorf("failed to create output directory %s: %s", outputDir, err)
		return
	}

	title := fmt.Sprintf("%s %s", target.bookTitle, volumeName)
	outputBaseName := latexOutputBasename

	textDir := filepath.Join(target.textDir, volumeName)

	imgDir := filepath.Join(target.imageDir, volumeName)
	relativeImgDir, err := filepath.Rel(outputDir, imgDir)
	if err != nil {
		log.Errorf("failed to get realtive path of image asset directory: %s", err)
		return
	}

	err = bundleBook(ctx, volumeInfo{
		book:   target.bookTitle,
		volume: volumeName,
		title:  title,
		author: target.author,

		outputDir:      outputDir,
		outputBaseName: outputBaseName,
		textDir:        textDir,
		imgDir:         imgDir,
		relativeImgDir: relativeImgDir,

		template:         template,
		preprocessScript: preprocessScript,
	})

	if err != nil {
		log.Warnf("failed to make latex %s: %s", outputDir, err)
	} else {
		log.Infof("book save to: %s", outputDir)
	}
}

func bundleBook(ctx context.Context, info volumeInfo) error {
	nodes, err := readTextFiles(info.textDir)
	if err != nil {
		return err
	}

	sizeMap := map[string]*image.Point{}
	var (
		errList []error
		nodeOk  bool
	)
	for i := range nodes {
		nodeOk, errList = replaceImgSrc(ctx, info, nodes[i], sizeMap, errList)
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
	}

	// user script
	if info.preprocessScript != "" {
		meta := latex.PreprocessMeta{
			OutputDir:      info.outputDir,
			OutputBaseName: info.outputBaseName,
			SourceFileName: filepath.Base(info.textDir),
			Book:           info.book,
			Volume:         info.volume,
			Title:          info.title,
			Author:         info.author,
		}
		if processed, err := latex.RunPreprocessScript(nodes, info.preprocessScript, meta); err == nil {
			nodes = processed
		} else {
			return err
		}
	}

	return latex.FromEpubSaveOutput(nodes, info.outputBaseName, latex.FromEpubOptions{
		Template:  info.template,
		OutputDir: info.outputDir,

		Title:  info.title,
		Author: info.author,
	})
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
func replaceImgSrc(ctx context.Context, info volumeInfo, node *html.Node, sizeMap map[string]*image.Point, errList []error) (bool, []error) {
	var childOk bool

	child := node.FirstChild
	for child != nil {
		nextChild := child.NextSibling

		childOk, errList = replaceImgSrc(ctx, info, child, sizeMap, errList)
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

	mapTo := filepath.Join(info.relativeImgDir, basename)
	mapTo = filepath.ToSlash(mapTo)

	if srcAttr == nil {
		node.Attr = append(node.Attr, html.Attribute{
			Key: "src",
			Val: mapTo,
		})
	} else {
		srcAttr.Val = mapTo
	}

	imgPath := filepath.Join(info.imgDir, basename)
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

func buildLocalBoss(options *options, target bookInfo, taskChan chan workerTask, group *sync.WaitGroup) error {
	template, err := getBookTemplate(options.cliTemplate, options.libTemplate, target.templateFile)
	if err != nil {
		return err
	}

	entryList, err := os.ReadDir(target.epubDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %s", target.epubDir, err)
	}

	preprocessScript := getBookPreprocessScript(options.cliPreprocessScript, target.preprocessScript)
	epubNamePrefix := common.InvalidPathCharReplace(target.bookTitle) + " "

	ctx := context.Background()
	ctx = context.WithValue(ctx, "template", template)
	ctx = context.WithValue(ctx, "preprocessScript", preprocessScript)
	ctx = context.WithValue(ctx, "epubNamePrefix", epubNamePrefix)

	for index, child := range entryList {
		if target.targetVolume >= 0 && index != target.targetVolume {
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

func buildLocalWorker(task workerTask) {
	ctx := task.ctx
	target := task.target
	epubName := task.volumeName

	template := ctx.Value("template").(string)
	preprocessScript := ctx.Value("preprocessScript").(string)
	epubNamePrefix := ctx.Value("epubNamePrefix").(string)

	ext := filepath.Ext(epubName)
	volumeName := epubName[:len(epubName)-len(ext)]
	if strings.HasPrefix(volumeName, epubNamePrefix) {
		volumeName = volumeName[len(epubNamePrefix):]
	}

	outputDir := filepath.Join(target.outputDir, volumeName)
	err := os.MkdirAll(outputDir, 0o777)
	if err != nil {
		log.Errorf("failed to create output directory %s: %s", outputDir, err)
		return
	}

	title := fmt.Sprintf("%s %s", target.bookTitle, volumeName)
	outputBaseName := latexOutputBasename

	err = extractEpub(localVolumeInfo{
		book:   target.bookTitle,
		volume: volumeName,
		title:  title,
		author: target.author,

		epubFile:       filepath.Join(target.epubDir, epubName),
		outputDir:      outputDir,
		outputBaseName: outputBaseName,
		assetDirName:   outputAssetDirName,

		template:         template,
		preprocessScript: preprocessScript,
	})

	if err != nil {
		log.Warnf("failed to make latex %s: %s", outputDir, err)
	} else {
		log.Infof("book save to: %s", outputDir)
	}
}

func extractEpub(info localVolumeInfo) error {
	convertOptions := latex.FromEpubOptions{
		Template:  info.template,
		OutputDir: info.outputDir,

		Title:  info.outputBaseName,
		Author: info.author,
	}

	return epub.Merge(epub.EpubMergeOptions{
		EpubFile:     info.epubFile,
		OutputDir:    info.outputDir,
		AssetDirName: info.assetDirName,

		JobCnt: runtime.NumCPU(),

		PreprocessFunc: func(nodes []*html.Node) ([]*html.Node, error) {
			nodes = latex.FromEpubPreprocess(nodes, convertOptions)

			// user script
			if info.preprocessScript != "" {
				meta := latex.PreprocessMeta{
					OutputDir:      info.outputDir,
					OutputBaseName: info.outputBaseName,
					SourceFileName: filepath.Base(info.epubFile),
					Book:           info.book,
					Volume:         info.volume,
					Title:          info.title,
					Author:         info.author,
				}

				if processed, err := latex.RunPreprocessScript(nodes, info.preprocessScript, meta); err == nil {
					nodes = processed
				} else {
					return nil, err
				}
			}

			return nodes, nil
		},
		SaveOutputFunc: func(nodes []*html.Node, _ string, _ string) error {
			return latex.FromEpubSaveOutput(nodes, info.outputBaseName, convertOptions)
		},
	})
}
