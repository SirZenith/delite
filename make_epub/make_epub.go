package make_epub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/SirZenith/litnovel-dl/base"
	"github.com/charmbracelet/log"
	"github.com/go-shiori/go-epub"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
)

const defaultOutputName = "out"

func Cmd() *cli.Command {
	var infoFile string

	cmd := &cli.Command{
		Name:  "epub",
		Usage: "bundle downloaded novel files into ePub book with infomation provided in info.json of the book",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output directory to save epub file to",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "info-file",
				UsageText:   "<JSON info>",
				Destination: &infoFile,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd, infoFile)
			if err != nil {
				return err
			}

			return cmdMain(options)
		},
	}

	return cmd
}

type options struct {
	textDir   string
	imageDir  string
	outputDir string
	bookTitle string
	author    string
}

type epubInfo struct {
	title      string
	author     string
	outputName string
	textDir    string
	imgDir     string
}

func getOptionsFromCmd(cmd *cli.Command, infoFile string) (options, error) {
	options := options{
		outputDir: cmd.String("output"),
	}

	if infoFile == "" {
		return options, errors.New("no info file is given")
	}

	bookInfo, err := base.ReadBookInfo(infoFile)
	if err != nil {
		return options, err
	}

	options.textDir = bookInfo.TextDir
	options.imageDir = bookInfo.ImgDir
	options.bookTitle = bookInfo.Title
	options.author = bookInfo.Author

	if options.outputDir == "" {
		if bookInfo.EpubDir != "" {
			options.outputDir = bookInfo.EpubDir
		} else {
			options.outputDir = filepath.Dir(infoFile)
		}
	}

	return options, nil
}

func cmdMain(options options) error {
	entryList, err := os.ReadDir(options.textDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %s", options.textDir, err)
	}

	err = os.MkdirAll(options.outputDir, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", options.outputDir, err)
	}

	for _, child := range entryList {
		volumeName := child.Name()

		title := fmt.Sprintf("%s %s", options.bookTitle, volumeName)

		outputName := fmt.Sprintf("%s %s.epub", options.bookTitle, volumeName)
		outputName = filepath.Join(options.outputDir, outputName)

		textDir := filepath.Join(options.textDir, volumeName)
		imgDir := options.imageDir
		if imgDir != "" {
			imgDir = filepath.Join(imgDir, volumeName)
		}

		err = makeEpub(epubInfo{
			title:      title,
			author:     options.author,
			outputName: outputName,
			textDir:    textDir,
			imgDir:     imgDir,
		})

		if err != nil {
			log.Infof("failed to make epub %s: %s", outputName, err)
		}
	}

	return nil
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

	err = addTexts(epub, info.textDir, imgNameMap)
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
func addTexts(epub *epub.Epub, textDir string, imgNameMap map[string]string) error {
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
		if err = addTextFile(epub, fullPath, imgNameMap); err != nil {
			fmt.Printf("failed to add %s: %s\n", fullPath, err)
		}
	}

	return nil
}

// Adds one text file to epub. Before adding it, src attribute value of all `img`
// gets replaced with internal image path.
// Any error happens during the process will be returned.
func addTextFile(epub *epub.Epub, fileName string, imgNameMap map[string]string) error {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}

	content := string(data)
	content, err = replaceImgSrc(content, imgNameMap)
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
func replaceImgSrc(content string, imgNameMap map[string]string) (string, error) {
	reader := strings.NewReader(content)
	tree, err := html.Parse(reader)
	if err != nil {
		return content, err
	}

	handleAllImgTags(tree, imgNameMap)

	writer := bytes.NewBufferString("")
	err = html.Render(writer, tree)
	if err != nil {
		return content, err
	}

	return writer.String(), err
}

// Recursion starting point of `img` tag processing.
func handleAllImgTags(node *html.Node, nameMap map[string]string) {
	if node == nil {
		return
	}

	if node.Type == html.ElementNode && node.Data == "img" {
		handleImgTag(node, nameMap)
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		handleAllImgTags(child, nameMap)
	}
}

// Replaces `src` attribute value of given tag with internal image path.
// This process is done by querying internal image path by base name of oroginal
// `src` value.
// If `data-src` attribute exists on given tag, value of `data-src` will be used
// for query instead of `src`'s.
func handleImgTag(node *html.Node, nameMap map[string]string) {
	attrList := node.Attr
	srcPath := ""
	for _, attr := range attrList {
		switch attr.Key {
		case "data-src":
			// higher priority, overrides `src` value
			srcPath = attr.Val
			break
		case "src":
			srcPath = attr.Val
		}
	}

	baseName := path.Base(srcPath)
	mapTo, ok := nameMap[baseName]
	if !ok {
		return
	}

	found := false
	for i := range attrList {
		if attrList[i].Key == "src" {
			attrList[i].Val = mapTo
			found = true
			break
		}
	}

	if !found {
		node.Attr = append(node.Attr, html.Attribute{
			Key: "src",
			Val: mapTo,
		})
	}
}
