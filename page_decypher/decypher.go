package page_decypher

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirZenith/bilinovel/base"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
)

const decypherTypeDesktop = "desktop"
const decypherTypeMobile = "mobile"

// maximum allowed level of nested directory in decypher target
const maxDecypherDirDepth = 200

func Cmd() *cli.Command {
	decypherTypes := []string{decypherTypeDesktop, decypherTypeMobile}

	cmd := &cli.Command{
		Name:    "decypher",
		Aliases: []string{"de"},
		Usage:   "decypher context text in downloaded chapter pages",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "job",
				Aliases: []string{"j"},
				Value:   int64(runtime.NumCPU()),
			},
			&cli.StringFlag{
				Name:    "translate",
				Aliases: []string{"t"},
				Usage: fmt.Sprintf(
					"type of translate map used by decypher process, possible values are: %s. If info file is used and no translate type is given, program will guess translate type from book's TOC URL",
					strings.Join(decypherTypes, ", "),
				),
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output file or directory depending on input type, default to the same value as input argument",
			},
			&cli.StringFlag{
				Name:    "input",
				Aliases: []string{"i"},
				Usage:   "path of source files or directory of source files.",
			},
			&cli.StringFlag{
				Name:  "info-file",
				Value: "",
				Usage: "path of book info JSON, if given command will try to download with option written in info file",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			return cmdMain(options)
		},
	}

	return cmd
}

type Task struct {
	srcFile    string
	outputFile string
}

type Result struct {
	err       error
	childPath string
}

type options struct {
	jobCnt        int
	translateType string
	target        string
	output        string
}

type translateContext struct {
	runeRemap map[rune]rune
	fontReMap map[rune]rune
}

func getOptionsFromCmd(cmd *cli.Command) (options, error) {
	options := options{
		jobCnt:        int(cmd.Int("job")),
		translateType: cmd.String("translate"),
		target:        cmd.String("input"),
		output:        cmd.String("output"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := base.ReadBookInfo(infoFile)
		if err != nil {
			return options, err
		}

		if options.target == "" {
			options.target = bookInfo.RawHTMLOutput
		}

		if options.output == "" {
			options.output = bookInfo.HTMLOutput
		}

		if options.translateType == "" {
			options.translateType = getTranslateTypeByURL(bookInfo.TocURL)
		}
	}

	if options.output == "" {
		options.output = options.target
	}

	return options, nil
}

// Guess translate type from a TOC URL. If translate can not be settle, this
// function will return an empty string.
func getTranslateTypeByURL(urlStr string) string {
	url, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	hostname := url.Hostname()

	if strings.HasSuffix(hostname, "bilinovel.com") {
		return decypherTypeMobile
	} else if strings.HasSuffix(hostname, "linovelib.com") {
		return decypherTypeDesktop
	}

	return ""
}

// Returns translate rune map according to translate type, `nil` will be returned
// in case of invalid translate type.
func getTranslateMap(translateType string) translateContext {
	switch translateType {
	case decypherTypeDesktop:
		return translateContext{
			runeRemap: desktopGetRuneRemapMap(),
			fontReMap: desktopGetFontRemapMap(),
		}
	case decypherTypeMobile:
		return translateContext{
			runeRemap: mobileGetRuneRemapMap(),
			fontReMap: mobileGetFontRemapMap(),
		}
	default:
		return translateContext{}
	}
}

func cmdMain(options options) error {
	ctx := getTranslateMap(options.translateType)
	if ctx.runeRemap == nil || ctx.fontReMap == nil {
		log.Warnf("no translate map for type: %q", options.translateType)
	}

	info, err := os.Stat(options.target)
	if err != nil {
		return fmt.Errorf("cannot access source path %q: %s", options.target, err)
	}

	if info.IsDir() {
		return decypherDirectory(ctx, &options)
	} else {
		return decypherSingleFile(ctx, options.target, options.output)
	}
}

// Decyphers given source file, and write output file.
func decypherSingleFile(ctx translateContext, srcFile string, outputFile string) error {
	info, err := os.Stat(srcFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %s", srcFile, err)
	}

	data, err := os.ReadFile(srcFile)
	if err != nil {
		return fmt.Errorf("failed to read target file: %s", err)
	}
	content := string(data)

	if ctx.runeRemap != nil {
		content = runeDecypher(content, ctx.runeRemap)
	}

	if ctx.fontReMap != nil {
		content, err = fontDecypher(content, ctx.fontReMap)
		if err != nil {
			return fmt.Errorf("font remap failed: %s:", err)
		}
	}

	err = os.WriteFile(outputFile, []byte(content), info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to write file %s: %s", outputFile, err)
	}

	return nil
}

// Recursively decypher all files under given directory.
func decypherDirectory(ctx translateContext, options *options) error {
	jobCnt := options.jobCnt
	task := make(chan string, jobCnt)
	result := make(chan Result, jobCnt)

	go func() {
		decypherBoss(task, options, "", 0)
		close(task)
	}()

	for i := 0; i < jobCnt; i++ {
		go decypherWorker(ctx, options, task, result)
	}

	endedCnt := 0
	for endedCnt < jobCnt {
		r := <-result
		if r.err != nil {
			log.Error(r.err)
		} else if r.childPath != "" {
			log.Infof("ok: %s", r.childPath)
		} else {
			endedCnt++
		}
	}

	return nil
}

func decypherBoss(taskChan chan string, options *options, childPath string, nestedLevel int) error {
	fullPath := filepath.Join(options.target, childPath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("failed to access %s: %s", fullPath, err)
	}

	if !info.IsDir() {
		taskChan <- childPath
		return nil
	}

	if nestedLevel == maxDecypherDirDepth {
		return fmt.Errorf("too many nested directory, skip: %s", fullPath)
	}

	outputDir := filepath.Join(options.output, childPath)
	err = os.MkdirAll(outputDir, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to create output directory: %s", err)
	}

	entryList, err := os.ReadDir(fullPath)
	if err != nil {
		return fmt.Errorf("unable to read directory %s: %s", fullPath, err)
	}

	for _, entry := range entryList {
		newChildPath := filepath.Join(childPath, entry.Name())
		if err = decypherBoss(taskChan, options, newChildPath, nestedLevel+1); err != nil {
			log.Error(err)
		}
	}

	return nil
}

func decypherWorker(ctx translateContext, options *options, taskChan chan string, resultChan chan Result) {
	for childPath := <-taskChan; childPath != ""; childPath = <-taskChan {
		srcFile := filepath.Join(options.target, childPath)
		outputFile := filepath.Join(options.output, childPath)

		resultChan <- Result{
			err:       decypherSingleFile(ctx, srcFile, outputFile),
			childPath: childPath,
		}
	}

	// indicating job ended
	resultChan <- Result{}
}

// ----------------------------------------------------------------------------

func runeDecypher(content string, translate map[rune]rune) string {
	buffer := bytes.NewBufferString("")

	for _, src := range content {
		if value, ok := translate[src]; ok {
			buffer.WriteRune(value)
		} else {
			buffer.WriteRune(src)
		}
	}

	return buffer.String()
}

// Parse HTMl content, finds all font decypher targets and translate text in them.
func fontDecypher(content string, translate map[rune]rune) (string, error) {
	reader := strings.NewReader(content)
	tree, err := html.Parse(reader)
	if err != nil {
		return content, err
	}

	handleFontDecypherAllTargets(tree, translate, false)

	writer := bytes.NewBufferString("")
	err = html.Render(writer, tree)
	if err != nil {
		return content, err
	}

	return writer.String(), err
}

// Recursively find and decypher all font decypher targets.
func handleFontDecypherAllTargets(node *html.Node, translate map[rune]rune, needTranslate bool) {
	if node == nil {
		return
	}

	switch node.Type {
	case html.TextNode:
		if needTranslate {
			handleFontDecypherTarget(node, translate)
		}
	case html.ElementNode:
		if !needTranslate {
			for _, attr := range node.Attr {
				if attr.Key == base.FontDecypherAttr {
					needTranslate = true
					break
				}
			}
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		handleFontDecypherAllTargets(child, translate, needTranslate)
	}
}

// Translate all text node in given node with font rune translate map.
// This function is dedicated for font descrambling, all runes with no corresponding
// in translate map will be ignored in final output.
func handleFontDecypherTarget(node *html.Node, translate map[rune]rune) {
	buffer := bytes.NewBufferString("")
	for _, val := range node.Data {
		if mapTo, ok := translate[val]; ok {
			buffer.WriteRune(mapTo)
		}
	}

	node.Data = buffer.String()
}
