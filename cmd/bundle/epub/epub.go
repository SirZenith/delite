package epub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/database"
	"github.com/SirZenith/delite/database/data_model"
	"github.com/charmbracelet/log"
	"github.com/go-shiori/go-epub"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
	"gorm.io/gorm"
)

const defaultOutputName = "out"

func Cmd() *cli.Command {
	var libIndex int64

	cmd := &cli.Command{
		Name:  "epub",
		Usage: "bundle downloaded novel files into ePub book with infomation provided in info.json of the book",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output directory to save epub file to",
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
			options, err := getOptionsFromCmd(cmd, int(libIndex))
			if err != nil {
				return err
			}

			return cmdMain(options)
		},
	}

	return cmd
}

type makeBookTarget struct {
	textDir   string
	imageDir  string
	outputDir string
	bookTitle string
	author    string
	tocURL    *url.URL
	dbPath    string

	isUnsupported bool
}

type options struct {
	targets []makeBookTarget
}

type epubInfo struct {
	title  string
	author string
	tocURL *url.URL
	db     *gorm.DB

	outputName string
	textDir    string
	imgDir     string
}

func getOptionsFromCmd(cmd *cli.Command, libIndex int) (options, error) {
	options := options{
		targets: []makeBookTarget{},
	}

	target, err := getTargetFromCmd(cmd)
	if err != nil {
		return options, err
	} else if target.outputDir != "" {
		options.targets = append(options.targets, target)
	}

	libraryInfoPath := cmd.String("library")
	if libraryInfoPath != "" {
		targetList, err := loadLibraryTargets(libraryInfoPath)
		if err != nil {
			return options, err
		}

		if 0 <= libIndex && libIndex < len(targetList) {
			options.targets = append(options.targets, targetList[libIndex])
		} else {
			options.targets = append(options.targets, targetList...)
		}
	}

	return options, nil
}

func getTargetFromCmd(cmd *cli.Command) (makeBookTarget, error) {
	target := makeBookTarget{
		outputDir: cmd.String("output"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := book_mgr.ReadBookInfo(infoFile)
		if err != nil {
			return target, err
		}

		target.textDir = bookInfo.TextDir
		target.imageDir = bookInfo.ImgDir
		target.bookTitle = bookInfo.Title
		target.author = bookInfo.Author
		target.tocURL, _ = url.Parse(bookInfo.TocURL)
		target.dbPath = bookInfo.DatabasePath

		if target.outputDir == "" {
			if bookInfo.EpubDir != "" {
				target.outputDir = bookInfo.EpubDir
			} else {
				target.outputDir = filepath.Dir(infoFile)
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
		url, _ := url.Parse(book.TocURL)

		targets = append(targets, makeBookTarget{
			textDir:       book.TextDir,
			imageDir:      book.ImgDir,
			outputDir:     book.EpubDir,
			bookTitle:     book.Title,
			author:        book.Author,
			tocURL:        url,
			dbPath:        book.DatabasePath,
			isUnsupported: book.LocalInfo != nil && book.LocalInfo.Type != book_mgr.LocalBookTypeHTML,
		})
	}

	return targets, nil
}

func cmdMain(options options) error {
	for _, target := range options.targets {
		logWorkBeginBanner(target)

		if target.isUnsupported {
			log.Info("skip unsupported resource")
			continue
		}

		entryList, err := os.ReadDir(target.textDir)
		if err != nil {
			log.Errorf("failed to read directory %s: %s", target.textDir, err)
			continue
		}

		err = os.MkdirAll(target.outputDir, 0o755)
		if err != nil {
			log.Errorf("failed to create output directory %s: %s", target.outputDir, err)
			continue
		}

		var db *gorm.DB
		if target.dbPath != "" {
			var err error
			db, err = database.Open(target.dbPath)
			if err != nil {
				log.Errorf("failed to open book database %s: %s", target.dbPath, err)
				continue
			}
		}

		for _, child := range entryList {
			volumeName := child.Name()

			title := fmt.Sprintf("%s %s", target.bookTitle, volumeName)

			outputName := fmt.Sprintf("%s %s.epub", target.bookTitle, volumeName)
			outputName = filepath.Join(target.outputDir, outputName)

			textDir := filepath.Join(target.textDir, volumeName)
			imgDir := target.imageDir
			if imgDir != "" {
				imgDir = filepath.Join(imgDir, volumeName)
			}

			err = makeEpub(epubInfo{
				title:  title,
				author: target.author,
				tocURL: target.tocURL,
				db:     db,

				outputName: outputName,
				textDir:    textDir,
				imgDir:     imgDir,
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
		fmt.Sprintf("%-12s: %s", "title", target.bookTitle),
		fmt.Sprintf("%-12s: %s", "author", target.author),
		fmt.Sprintf("%-12s: %s", "text   dir", target.textDir),
		fmt.Sprintf("%-12s: %s", "image  dir", target.imageDir),
		fmt.Sprintf("%-12s: %s", "output dir", target.outputDir),
	}

	common.LogBannerMsg(msgs, 5)
}

func makeEpub(info epubInfo) error {
	epub, err := epub.NewEpub(info.title)
	if err != nil {
		return err
	}

	epub.SetAuthor(info.author)

	imgNameMap, err := addImages(epub, info.imgDir)
	if err != nil {
		return err
	}

	ctx := context.WithValue(context.Background(), "imgNameMap", imgNameMap)
	ctx = context.WithValue(ctx, "db", info.db)
	ctx = context.WithValue(ctx, "url", info.tocURL)

	err = addTexts(epub, info.textDir, ctx)
	if err != nil {
		return err
	}

	epub.Write(info.outputName)

	return nil
}

// Adds all files in given directory as image into epub. Returns a map with base
// name of original file as key, internal path in epub of that file as value.
// If this function cannot read given directory, it will return error. But any
// Errors happend during add image will be logged instead of returned.
func addImages(epub *epub.Epub, imgDir string) (map[string]string, error) {
	nameMap := map[string]string{}
	if imgDir == "" {
		return nameMap, nil
	}

	if _, err := os.Stat(imgDir); errors.Is(err, os.ErrNotExist) {
		log.Warnf("image directory not found, skip: %s", imgDir)
		return nameMap, nil
	}

	entryList, err := os.ReadDir(imgDir)
	if err != nil {
		return nameMap, fmt.Errorf("cannot read image directory %s: %s", imgDir, err)
	}

	for _, child := range entryList {
		if !child.IsDir() {
			name := child.Name()
			fullPath := filepath.Join(imgDir, name)

			if internalPath, err := epub.AddImage(fullPath, name); err == nil {
				nameMap[name] = internalPath
			} else {
				log.Infof("failed to add %s: %s", fullPath, err)

			}
		}
	}

	return nameMap, nil
}

// Adds all files in given directory as text into epub. If given directory cannot
// be read, this function will return an error. Any error happens during adding
// text will only be logged but not returned.
// All `img` tags in input text content will be updated to use internal image
// path before gets added to book.
func addTexts(epub *epub.Epub, textDir string, ctx context.Context) error {
	entryList, err := os.ReadDir(textDir)
	if err != nil {
		return fmt.Errorf("failed to read text directory %s: %s", textDir, err)
	}

	names := []string{}
	for _, child := range entryList {
		if !child.IsDir() {
			name := child.Name()
			names = append(names, name)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		ext := filepath.Ext(name)
		if ext != ".html" && ext != ".xhtml" {
			continue
		}

		fullPath := filepath.Join(textDir, name)
		if err = addTextFile(epub, fullPath, ctx); err != nil {
			fmt.Printf("failed to add %s: %s\n", fullPath, err)
		}
	}

	return nil
}

// Adds one text file to epub. Before adding it, src attribute value of all `img`
// gets replaced with internal image path.
// Any error happens during the process will be returned.
func addTextFile(epub *epub.Epub, fileName string, ctx context.Context) error {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}

	content := string(data)
	content, err = replaceImgSrc(content, ctx)
	if err != nil {
		return err
	}

	sectionName := getSectionNameFromFileName(fileName)
	_, err = epub.AddSection(content, sectionName, "", "")
	if err != nil {
		return err
	}

	return nil
}

// Returns a table of contents entry name for given text file name.
func getSectionNameFromFileName(fileName string) string {
	basename := filepath.Base(fileName)
	ext := filepath.Ext(basename)
	sectionName := basename[:len(basename)-len(ext)]

	// remove prefix automatically prepended during downloading.
	patt := regexp.MustCompile(`^\d{4} \- `)
	sectionName = patt.ReplaceAllString(sectionName, "")

	return sectionName
}

// Parses given text as HTML, and replace all `img` tags' `src` attribute value
// with internal image path used by epub.
func replaceImgSrc(content string, ctx context.Context) (string, error) {
	reader := strings.NewReader(content)
	tree, err := html.Parse(reader)
	if err != nil {
		return content, err
	}

	handleAllImgTags(tree, ctx)

	writer := bytes.NewBufferString("")
	err = html.Render(writer, tree)
	if err != nil {
		return content, err
	}

	return writer.String(), err
}

// Recursion starting point of `img` tag processing.
func handleAllImgTags(node *html.Node, ctx context.Context) {
	if node == nil {
		return
	}

	if node.Type == html.ElementNode && node.Data == "img" {
		handleImgTag(node, ctx)
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		handleAllImgTags(child, ctx)
	}
}

// Replaces `src` attribute value of given tag with internal image path.
// This process is done by querying internal image path by base name of oroginal
// `src` value.
// If `data-src` attribute exists on given tag, value of `data-src` will be used
// for query instead of `src`'s.
func handleImgTag(node *html.Node, ctx context.Context) {
	var mapTo string

	srcAttr := html_util.GetNodeAttr(node, "src")
	dataSrcAttr := html_util.GetNodeAttr(node, "data-src")

	if srcAttr != nil {
		if value := getMappedImagePath(ctx, srcAttr.Val); value != "" {
			mapTo = value
		}
	}
	if dataSrcAttr != nil {
		if value := getMappedImagePath(ctx, dataSrcAttr.Val); value != "" {
			mapTo = value
		}
	}

	if mapTo == "" {
		return
	}

	if srcAttr != nil {
		srcAttr.Val = mapTo
	} else {
		node.Attr = append(node.Attr, html.Attribute{
			Key: "src",
			Val: mapTo,
		})
	}
}

func getMappedImagePath(ctx context.Context, src string) string {
	nameMap := ctx.Value("imgNameMap").(map[string]string)
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

	return nameMap[basename]
}
