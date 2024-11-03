package latex

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	format_html "github.com/SirZenith/delite/format/html"
	"github.com/SirZenith/delite/format/latex"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const defaultOutputName = "out"

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
	template string
}

type makeBookTarget struct {
	TextDir   string
	ImageDir  string
	OutputDir string
	BookTitle string
	Author    string
}

type workload struct {
	options *options

	title          string
	author         string
	outputName     string
	textDir        string
	imgDir         string
	relativeImgDir string
}

func getOptionsFromCmd(cmd *cli.Command, libIndex int) (options, []makeBookTarget, error) {
	options := options{
		template: cmd.String("template"),
	}

	var targets []makeBookTarget

	templateFile := cmd.String("template-file")
	if options.template != "" {
		// pass
	} else if templateFile == "" {
		options.template = defaultLatexTemplte
	} else {
		data, err := os.ReadFile(templateFile)
		if err != nil {
			return options, targets, fmt.Errorf("failed to read template file %s: %s", templateFile, err)
		}

		options.template = string(data)
	}

	target, err := getTargetFromCmd(cmd)
	if err != nil {
		return options, targets, err
	} else if target.OutputDir != "" {
		targets = append(targets, target)
	}

	libraryInfoPath := cmd.String("library")
	if libraryInfoPath != "" {
		targetList, err := loadLibraryTargets(libraryInfoPath)
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

func getTargetFromCmd(cmd *cli.Command) (makeBookTarget, error) {
	target := makeBookTarget{
		OutputDir: cmd.String("output"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := book_mgr.ReadBookInfo(infoFile)
		if err != nil {
			return target, err
		}

		target.TextDir = bookInfo.TextDir
		target.ImageDir = bookInfo.ImgDir
		target.BookTitle = bookInfo.Title
		target.Author = bookInfo.Author

		if target.OutputDir == "" {
			if bookInfo.LatexDir != "" {
				target.OutputDir = bookInfo.LatexDir
			} else {
				target.OutputDir = filepath.Dir(infoFile)
			}
		}
	}

	return target, nil
}

// loadLibraryTargets reads book list from library info JSON and returns them
// as a list of MakeBookTarget.
func loadLibraryTargets(libInfoPath string) ([]makeBookTarget, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	targets := []makeBookTarget{}
	for _, book := range info.Books {
		targets = append(targets, makeBookTarget{
			TextDir:   book.TextDir,
			ImageDir:  book.ImgDir,
			OutputDir: book.LatexDir,
			BookTitle: book.Title,
			Author:    book.Author,
		})
	}

	return targets, nil
}

func cmdMain(options options, targets []makeBookTarget) error {
	for _, target := range targets {
		logWorkBeginBanner(target)

		entryList, err := os.ReadDir(target.TextDir)
		if err != nil {
			log.Errorf("failed to read directory %s: %s", target.TextDir, err)
			continue
		}

		err = os.MkdirAll(target.OutputDir, 0o755)
		if err != nil {
			log.Errorf("failed to create output directory %s: %s", target.OutputDir, err)
			continue
		}

		for _, child := range entryList {
			volumeName := child.Name()

			texDir := filepath.Join(target.OutputDir, volumeName)
			err = os.MkdirAll(texDir, 0o755)
			if err != nil {
				log.Errorf("failed to create output directory %s: %s", texDir, err)
				continue
			}

			title := fmt.Sprintf("%s %s", target.BookTitle, volumeName)

			outputName := filepath.Join(texDir, title+".tex")

			textDir := filepath.Join(target.TextDir, volumeName)

			imgDir := filepath.Join(target.ImageDir, volumeName)
			relativeImgDir, err := filepath.Rel(texDir, imgDir)
			if err != nil {
				log.Errorf("failed to get realtive path of image asset directory: %s", err)
				continue
			}

			err = bundleBook(workload{
				options: &options,

				title:          title,
				author:         target.Author,
				outputName:     outputName,
				textDir:        textDir,
				imgDir:         imgDir,
				relativeImgDir: relativeImgDir,
			})

			if err != nil {
				log.Warnf("failed to make epub %s: %s", outputName, err)
			} else {
				log.Infof("book save to: %s", outputName)
			}
		}

	}

	return nil
}

// logWorkBeginBanner prints a banner indicating a new download of book starts.
func logWorkBeginBanner(target makeBookTarget) {
	msgs := []string{
		fmt.Sprintf("%-12s: %s", "title", target.BookTitle),
		fmt.Sprintf("%-12s: %s", "author", target.Author),
		fmt.Sprintf("%-12s: %s", "text   dir", target.TextDir),
		fmt.Sprintf("%-12s: %s", "image  dir", target.ImageDir),
		fmt.Sprintf("%-12s: %s", "output dir", target.OutputDir),
	}

	common.LogBannerMsg(msgs, 5)
}

func bundleBook(info workload) error {
	nodes, err := readTextFiles(info.textDir)
	if err != nil {
		return err
	}

	sizeMap := map[string]*image.Point{}
	var errList []error
	for _, node := range nodes {
		errList = replaceImgSrc(info, node, sizeMap, errList)
	}
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	for _, node := range nodes {
		format_html.SetImageSizeMeta(node, sizeMap)
		format_html.SetImageTypeMeta(node)
	}

	saveOutput(info, nodes)

	return nil
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
func replaceImgSrc(info workload, node *html.Node, sizeMap map[string]*image.Point, errList []error) []error {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		errList = replaceImgSrc(info, child, sizeMap, errList)
	}

	if node.Type != html.ElementNode || node.DataAtom != atom.Img {
		return errList
	}

	srcAttr := html_util.GetNodeAttr(node, "src")
	dataSrcAttr := html_util.GetNodeAttr(node, "data-src")

	basename := ""
	if dataSrcAttr != nil {
		basename = path.Base(dataSrcAttr.Val)
	} else if srcAttr != nil {
		basename = path.Base(srcAttr.Val)
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

func saveOutput(info workload, nodes []*html.Node) error {
	container := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}

	converterMap := latex.GetLatexTategakiConverter()
	content, _ := latex.ConvertHTML2Latex(container, "", converterMap)

	// write output file
	outFile, err := os.Create(info.outputName)
	if err != nil {
		return fmt.Errorf("failed to write create file %s: %s", info.outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)
	fmt.Fprintln(outWriter, info.options.template)
	fmt.Fprintf(outWriter, "\\title{%s}\n", info.title)
	fmt.Fprintf(outWriter, "\\author{%s}\n", info.author)
	fmt.Fprintf(outWriter, "\\date{%s}\n", "")
	fmt.Fprint(outWriter, "\n")
	fmt.Fprintln(outWriter, "\\begin{document}")
	fmt.Fprintln(outWriter, "\\maketitle")
	fmt.Fprintln(outWriter, "\\tableofcontents")
	fmt.Fprintln(outWriter, "\\large")

	for _, segment := range content {
		outWriter.WriteString(segment)
	}

	fmt.Fprintln(outWriter, "\\end{document}")

	err = outWriter.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nil
}
