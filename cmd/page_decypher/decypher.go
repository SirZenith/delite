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

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
)

const (
	decypherTypeNone      = "none"
	decypherTypeLinovelib = "linovelib"
	decypherTypeBilinove  = "bilinovel"
)

// maximum allowed level of nested directory in decypher target
const maxDecypherDirDepth = 200

func Cmd() *cli.Command {
	var libFilePath string
	var libIndex int64

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
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "library-file",
				UsageText:   "<lib-file>",
				Destination: &libFilePath,
				Min:         1,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "library-index",
				UsageText:   " <index>",
				Destination: &libIndex,
				Value:       -1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd, libFilePath, int(libIndex))
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

type DecypherTarget struct {
	TranslateType string
	Target        string
	Output        string

	IsUnsupported bool
}

type options struct {
	jobCnt  int
	targets []DecypherTarget
}

type translateContext struct {
	runeRemap map[rune]rune
	fontReMap map[rune]rune
}

func getOptionsFromCmd(cmd *cli.Command, libFilePath string, libIndex int) (options, error) {
	options := options{
		jobCnt:  int(cmd.Int("job")),
		targets: []DecypherTarget{},
	}

	targetList, err := loadLibraryTargets(libFilePath)
	if err != nil {
		return options, err
	}

	if 0 <= libIndex && libIndex < len(targetList) {
		options.targets = append(options.targets, targetList[libIndex])
	} else {
		options.targets = append(options.targets, targetList...)
	}

	return options, nil
}

// loadLibraryTargets reads book list from library info JSON and returns them
// as a list of DecypherTarget.
func loadLibraryTargets(libInfoPath string) ([]DecypherTarget, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	targets := []DecypherTarget{}
	for _, book := range info.Books {
		targets = append(targets, DecypherTarget{
			Target:        book.RawDir,
			Output:        book.TextDir,
			TranslateType: getTranslateTypeByURL(book.TocURL),

			IsUnsupported: book.LocalInfo != nil,
		})
	}

	return targets, nil
}

// Guess translate type from a TOC URL. If translate can not be settle, this
// function will return an empty string.
func getTranslateTypeByURL(urlStr string) string {
	targetType := ""

	url, err := url.Parse(urlStr)
	if err != nil {
		return targetType
	}

	hostname := url.Hostname()
	suffixMap := map[string]string{
		"bilimanga.net": decypherTypeNone,
		"bilinovel.com": decypherTypeBilinove,
		"linovelib.com": decypherTypeLinovelib,
		"senmanga.com":  decypherTypeNone,
		"syosetu.com":   decypherTypeNone,
	}

	for suffix, value := range suffixMap {
		if strings.HasSuffix(hostname, suffix) {
			targetType = value
			break
		}
	}

	return targetType
}

// Returns translate rune map according to translate type, `nil` will be returned
// in case of invalid translate type.
func getTranslateMap(translateType string) translateContext {
	switch translateType {
	case decypherTypeLinovelib:
		return translateContext{
			runeRemap: desktopGetRuneRemapMap(),
			fontReMap: desktopGetFontRemapMap(),
		}
	case decypherTypeBilinove:
		return translateContext{
			runeRemap: mobileGetRuneRemapMap(),
			fontReMap: mobileGetFontRemapMap(),
		}
	default:
		return translateContext{}
	}
}

func cmdMain(options options) error {
	for _, target := range options.targets {
		logWorkBeginBanner(target)

		if target.IsUnsupported {
			log.Info("skip unsupported resource")
			continue
		}

		ctx := getTranslateMap(target.TranslateType)
		if target.TranslateType != decypherTypeNone && (ctx.runeRemap == nil || ctx.fontReMap == nil) {
			log.Warnf("no translate map for type: %q", target.TranslateType)
		}

		info, err := os.Stat(target.Target)
		if err != nil {
			log.Errorf("cannot access source path %q: %s", target.Target, err)
			continue
		}

		if info.IsDir() {
			err = decypherDirectory(ctx, &options, &target)
		} else {
			err = decypherSingleFile(ctx, target.Target, target.Output)
		}

		if err != nil {
			log.Errorf("%s", err)
		}
	}

	return nil
}

// logWorkBeginBanner prints a banner indicating a new download of book starts.
func logWorkBeginBanner(target DecypherTarget) {
	msgs := []string{
		fmt.Sprintf("%-10s: %s", "source", target.Target),
		fmt.Sprintf("%-10s: %s", "output", target.Output),
	}

	common.LogBannerMsg(msgs, 2)
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
func decypherDirectory(ctx translateContext, options *options, target *DecypherTarget) error {
	jobCnt := options.jobCnt
	task := make(chan string, jobCnt)
	result := make(chan Result, jobCnt)

	go func() {
		decypherBoss(task, target, "", 0)
		close(task)
	}()

	for i := 0; i < jobCnt; i++ {
		go decypherWorker(ctx, target, task, result)
	}

	endedCnt := 0
	for endedCnt < jobCnt {
		r := <-result
		if r.err != nil {
			log.Error(r.err)
		} else if r.childPath != "" {
			log.Debugf("ok: %s", r.childPath)
		} else {
			endedCnt++
		}
	}

	return nil
}

func decypherBoss(taskChan chan string, target *DecypherTarget, childPath string, nestedLevel int) error {
	fullPath := filepath.Join(target.Target, childPath)
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

	outputDir := filepath.Join(target.Output, childPath)
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
		if err = decypherBoss(taskChan, target, newChildPath, nestedLevel+1); err != nil {
			log.Error(err)
		}
	}

	return nil
}

func decypherWorker(ctx translateContext, target *DecypherTarget, taskChan chan string, resultChan chan Result) {
	for childPath := <-taskChan; childPath != ""; childPath = <-taskChan {
		srcFile := filepath.Join(target.Target, childPath)
		outputFile := filepath.Join(target.Output, childPath)

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
				if attr.Key == common.FontDecypherAttr {
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
