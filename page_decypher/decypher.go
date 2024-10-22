package page_decypher

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirZenith/bilinovel/base"
	"github.com/urfave/cli/v3"
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
func getTranslateMap(translateType string) map[rune]rune {
	switch translateType {
	case decypherTypeDesktop:
		return getDesktopRuneMap()
	case decypherTypeMobile:
		return getMobileRuneMap()
	default:
		return nil
	}
}

func cmdMain(options options) error {
	translate := getTranslateMap(options.translateType)
	if translate == nil {
		return fmt.Errorf("can not find translate map for type: %q", options.translateType)
	}

	info, err := os.Stat(options.target)
	if err != nil {
		return fmt.Errorf("cannot access source path %q: %s", options.target, err)
	}

	if info.IsDir() {
		return decypherDirectory(translate, &options)
	} else {
		return decypherSingleFile(translate, options.target, options.output)
	}
}

// Decyphers given source file, and write output file.
func decypherSingleFile(translate map[rune]rune, srcFile string, outputFile string) error {
	info, err := os.Stat(srcFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %s", srcFile, err)
	}

	data, err := os.ReadFile(srcFile)
	if err != nil {
		return fmt.Errorf("failed to read target file: %s", err)
	}

	content := string(data)
	buffer := bytes.NewBufferString("")
	for _, src := range content {
		if value, ok := translate[src]; ok {
			buffer.WriteRune(value)
		} else {
			buffer.WriteRune(src)
		}
	}

	err = os.WriteFile(outputFile, buffer.Bytes(), info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to write file %s: %s", outputFile, err)
	}

	return nil
}

// Recursively decypher all files under given directory.
func decypherDirectory(translate map[rune]rune, options *options) error {
	jobCnt := options.jobCnt
	task := make(chan string, jobCnt)
	result := make(chan Result, jobCnt)

	go func() {
		decypherBoss(task, options, "", 0)
		close(task)
	}()

	for i := 0; i < jobCnt; i++ {
		go decypherWorker(translate, options, task, result)
	}

	endedCnt := 0
	for endedCnt < jobCnt {
		r := <-result
		if r.err != nil {

			log.Println(r.err)
		} else if r.childPath != "" {

			log.Println("ok:", r.childPath)
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
			log.Println(err)
		}
	}

	return nil
}

func decypherWorker(translate map[rune]rune, options *options, taskChan chan string, resultChan chan Result) {
	for childPath := <-taskChan; childPath != ""; childPath = <-taskChan {
		srcFile := filepath.Join(options.target, childPath)
		outputFile := filepath.Join(options.output, childPath)

		resultChan <- Result{
			err:       decypherSingleFile(translate, srcFile, outputFile),
			childPath: childPath,
		}
	}

	// indicating job ended
	resultChan <- Result{}
}
