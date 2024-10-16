package make_epub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bilinovel/base"
	"github.com/go-shiori/go-epub"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
)

const defaultOutputName = "out"

func Cmd() *cli.Command {
	var infoFile string

	cmd := &cli.Command{
		Name: "epub",
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
}

type epubInfo struct {
	title      string
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

	options.bookTitle = bookInfo.Title
	options.textDir = bookInfo.HTMLOutput
	options.imageDir = bookInfo.ImgOutput

	if options.outputDir == "" {
		options.outputDir = filepath.Dir(infoFile)
	}

	return options, nil
}

func cmdMain(options options) error {
	entryList, err := os.ReadDir(options.textDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %s", options.textDir, err)
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
			outputName: outputName,
			textDir:    textDir,
			imgDir:     imgDir,
		})

		if err != nil {
			log.Printf("failed to write output %s: %s\n", outputName, err)
		}
	}

	return nil
}

func makeEpub(info epubInfo) error {
	epub, err := epub.NewEpub(info.title)
	if err != nil {
		return err
	}

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

func addImages(epub *epub.Epub, imgDir string) (map[string]string, error) {
	nameMap := map[string]string{}
	if imgDir == "" {
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
				log.Printf("failed to add %s: %s", fullPath, err)

			}
		}
	}

	return nameMap, nil
}

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
		fullPath := filepath.Join(textDir, name)
		if err = addTextFile(epub, fullPath, imgNameMap); err != nil {
			fmt.Printf("failed to add %s: %s\n", fullPath, err)
		}
	}

	return nil
}

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

	epub.AddSection(content, fileName, "", "")

	return nil
}

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
