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

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/cmd/convert/common/epub_merge"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/database"
	"github.com/SirZenith/delite/database/data_model"
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

func Cmd() *cli.Command {
	var libIndex int64

	cmd := &cli.Command{
		Name:  "latex",
		Usage: "bundle downloaded novel files into LaTex file with infomation provided in info.json of the book",
		Flags: []cli.Flag{
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
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output directory to save .tex file to",
			},
			&cli.StringFlag{
				Name:  "preprocess",
				Usage: "path to preprocess Lua script",
			},
			&cli.StringFlag{
				Name:  "info-file",
				Usage: "path to info json file",
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path to library info JSON.",
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
	cliTemplate         string
	cliPreprocessScript string

	libTemplate string
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
}

type volumeInfo struct {
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

func getOptionsFromCmd(cmd *cli.Command, libIndex int) (options, []bookInfo, error) {
	options := options{
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

	var targets []bookInfo

	target, err := getTargetFromCmd(cmd)
	if err != nil {
		return options, targets, err
	} else if target.outputDir != "" {
		targets = append(targets, target)
	}

	libraryInfoPath := cmd.String("library")
	if libraryInfoPath != "" {
		targetList, err := loadLibraryTargets(libraryInfoPath, &options)
		if err != nil {
			return options, targets, err
		}

		if 0 <= libIndex && libIndex < len(targetList) {
			targets = append(targets, targetList[libIndex])
		} else {
			targets = append(targets, targetList...)
		}
	}

	return options, targets, nil
}

func getTargetFromCmd(cmd *cli.Command) (bookInfo, error) {
	target := bookInfo{
		outputDir: cmd.String("output"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		book, err := book_mgr.ReadBookInfo(infoFile)
		if err != nil {
			return target, err
		}

		target.textDir = book.TextDir
		target.imageDir = book.ImgDir
		target.epubDir = book.EpubDir
		target.bookTitle = book.Title
		target.author = book.Author
		target.tocURL, _ = url.Parse(book.TocURL)
		target.dbPath = book.DatabasePath

		if target.outputDir == "" {
			if book.LatexDir != "" {
				target.outputDir = book.LatexDir
			} else {
				target.outputDir = filepath.Dir(infoFile)
			}
		}

		target.isLocal = book.LocalInfo != nil && book.LocalInfo.Type == book_mgr.LocalBookTypeEpub

		if book.LatexInfo != nil {
			latexInfo := book.LatexInfo
			target.templateFile = latexInfo.TemplateFile
			target.preprocessScript = latexInfo.PreprocessScript
		}
	}

	return target, nil
}

// loadLibraryTargets reads book list from library info JSON and returns them
// as a list of MakeBookTarget.
func loadLibraryTargets(libInfoPath string, options *options) ([]bookInfo, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	targets := []bookInfo{}
	for _, book := range info.Books {
		tocURL, _ := url.Parse(book.TocURL)

		target := bookInfo{
			textDir:   book.TextDir,
			imageDir:  book.ImgDir,
			epubDir:   book.EpubDir,
			outputDir: book.LatexDir,

			bookTitle: book.Title,
			author:    book.Author,
			tocURL:    tocURL,
			dbPath:    book.DatabasePath,
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

	if info.LatexConfig.TemplateFile != "" {
		data, err := os.ReadFile(info.LatexConfig.TemplateFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read template file %s: %s", info.LatexConfig.TemplateFile, err)
		}

		options.libTemplate = string(data)
	}

	return targets, nil
}

func cmdMain(options options, targets []bookInfo) error {
	for _, target := range targets {
		logWorkBeginBanner(target)

		if target.isUnsupported {
			log.Info("skip unsupported resource")
			continue
		}

		err := os.MkdirAll(target.outputDir, 0o755)
		if err != nil {
			log.Errorf("failed to create output directory %s: %s", target.outputDir, err)
			continue
		}

		if target.isLocal {
			err = bundlingLocalTarget(options, target)
		} else {
			err = bundlingRemoteTarget(options, target)
		}

		if err != nil {
			log.Errorf("%s", err)
		}
	}

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

func bundlingRemoteTarget(options options, target bookInfo) error {
	template, err := getBookTemplate(options.cliTemplate, options.libTemplate, target.templateFile)
	if err != nil {
		return err
	}

	preprocessScript := getBookPreprocessScript(options.cliPreprocessScript, target.preprocessScript)

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

	ctx := context.WithValue(context.Background(), "db", db)
	ctx = context.WithValue(ctx, "url", target.tocURL)

	for _, child := range entryList {
		volumeName := child.Name()

		outputDir := filepath.Join(target.outputDir, volumeName)
		err = os.MkdirAll(outputDir, 0o755)
		if err != nil {
			log.Errorf("failed to create output directory %s: %s", outputDir, err)
			continue
		}

		title := fmt.Sprintf("%s %s", target.bookTitle, volumeName)
		outputBaseName := common.InvalidPathCharReplace(title)

		textDir := filepath.Join(target.textDir, volumeName)

		imgDir := filepath.Join(target.imageDir, volumeName)
		relativeImgDir, err := filepath.Rel(outputDir, imgDir)
		if err != nil {
			log.Errorf("failed to get realtive path of image asset directory: %s", err)
			continue
		}

		err = bundleBook(ctx, volumeInfo{
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

	return nil
}

func bundleBook(ctx context.Context, info volumeInfo) error {
	nodes, err := readTextFiles(info.textDir)
	if err != nil {
		return err
	}

	sizeMap := map[string]*image.Point{}
	var errList []error
	for _, node := range nodes {
		errList = replaceImgSrc(ctx, info, node, sizeMap, errList)
	}
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	for _, node := range nodes {
		format_html.SetImageSizeMeta(node, sizeMap)
		format_html.SetImageTypeMeta(node)
	}

	// user script
	if info.preprocessScript != "" {
		meta := latex.PreprocessMeta{
			OutputBaseName: info.outputBaseName,
			SourceFileName: filepath.Base(info.textDir),
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
func replaceImgSrc(ctx context.Context, info volumeInfo, node *html.Node, sizeMap map[string]*image.Point, errList []error) []error {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		errList = replaceImgSrc(ctx, info, child, sizeMap, errList)
	}

	if node.Type != html.ElementNode || node.DataAtom != atom.Img {
		return errList
	}

	srcAttr := html_util.GetNodeAttr(node, "src")
	dataSrcAttr := html_util.GetNodeAttr(node, "data-src")

	basename := ""
	if dataSrcAttr != nil {
		basename = getMappedImageName(ctx, dataSrcAttr.Val)
	} else if srcAttr != nil {
		basename = getMappedImageName(ctx, srcAttr.Val)
	}
	if basename == "" {
		return errList
	}

	mapTo := filepath.Join(info.relativeImgDir, basename)

	if srcAttr == nil {
		node.Attr = append(node.Attr, html.Attribute{
			Key: "src",
			Val: mapTo,
		})
	} else {
		srcAttr.Val = mapTo
	}

	imgPath := filepath.Join(info.imgDir, basename)
	if size, err := getImageSize(imgPath); err == nil {
		sizeMap[mapTo] = size
	} else {
		errList = append(errList, err)
	}

	return errList
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

func bundlingLocalTarget(options options, target bookInfo) error {
	template, err := getBookTemplate(options.cliTemplate, options.libTemplate, target.templateFile)
	if err != nil {
		return err
	}

	preprocessScript := getBookPreprocessScript(options.cliPreprocessScript, target.preprocessScript)

	entryList, err := os.ReadDir(target.epubDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %s", target.epubDir, err)
	}

	for _, child := range entryList {
		epubName := child.Name()
		ext := filepath.Ext(epubName)
		volumeName := epubName[:len(epubName)-len(ext)]

		outputDir := filepath.Join(target.outputDir, volumeName)
		err = os.MkdirAll(outputDir, 0o755)
		if err != nil {
			log.Errorf("failed to create output directory %s: %s", outputDir, err)
			continue
		}

		title := fmt.Sprintf("%s %s", target.bookTitle, volumeName)
		outputBaseName := common.InvalidPathCharReplace(title)

		err = extractEpub(localVolumeInfo{
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

	return nil
}

func extractEpub(info localVolumeInfo) error {
	convertOptions := latex.FromEpubOptions{
		Template:  info.template,
		OutputDir: info.outputDir,

		Title:  info.outputBaseName,
		Author: info.author,
	}

	return epub_merge.Merge(epub_merge.EpubMergeOptions{
		EpubFile:     info.epubFile,
		OutputDir:    info.outputDir,
		AssetDirName: info.assetDirName,

		JobCnt: runtime.NumCPU(),

		PreprocessFunc: func(nodes []*html.Node) ([]*html.Node, error) {
			nodes = latex.FromEpubPreprocess(nodes, convertOptions)

			// user script
			if info.preprocessScript != "" {
				meta := latex.PreprocessMeta{
					OutputBaseName: info.outputBaseName,
					SourceFileName: filepath.Base(info.epubFile),
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
