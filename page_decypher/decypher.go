package page_decypher

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/bilinovel/base"
	"github.com/urfave/cli/v3"
)

const DECYPHER_TYPE_DESKTOP = "desktop"
const DECYPHER_TYPE_MOBILE = "mobile"

func Cmd() *cli.Command {
	decypherTypes := []string{DECYPHER_TYPE_DESKTOP, DECYPHER_TYPE_MOBILE}

	cmd := &cli.Command{
		Name:    "decypher",
		Aliases: []string{"de"},
		Usage:   "decypher downloaded chapter pages",
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
		Arguments: []cli.Argument{},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getDecypherOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			return pageDecypher(options)
		},
	}

	return cmd
}

type Task struct {
	srcFile    string
	outputFile string
}

type Result struct {
	err  error
	task *Task
}

type Options struct {
	jobCnt        int
	translateType string
	target        string
	output        string
}

func getDecypherOptionsFromCmd(cmd *cli.Command) (Options, error) {
	options := Options{
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
		return DECYPHER_TYPE_MOBILE
	} else if strings.HasSuffix(hostname, "linovelib.com") {
		return DECYPHER_TYPE_DESKTOP
	}

	return ""
}

// Returns translate rune map according to translate type, `nil` will be returned
// in case of invalid translate type.
func getTranslateMap(translateType string) map[rune]rune {
	switch translateType {
	case DECYPHER_TYPE_DESKTOP:
		return getDesktopRuneMap()
	case DECYPHER_TYPE_MOBILE:
		return getMobileRuneMap()
	default:
		return nil
	}
}

func pageDecypher(options Options) error {
	translate := getTranslateMap(options.translateType)
	if translate == nil {
		return fmt.Errorf("can not find translate map for type: %q", options.translateType)
	}

	info, err := os.Stat(options.target)
	if err != nil {
		return fmt.Errorf("failed to read source file %q: %s", options.target, err)
	}

	if info.IsDir() {
		return decypherDirectory(translate, options.target, options.output, options.jobCnt)
	} else {
		return decypherSingleFile(translate, options.target, options.output)
	}
}

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

func decypherDirectory(translate map[rune]rune, srcDir, outputDir string, jobCnt int) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %s", err)
	}

	err = os.MkdirAll(outputDir, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to create output directory: %s", err)
	}

	targets, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	task := make(chan Task, jobCnt)
	result := make(chan Result, jobCnt)

	go func() {
		for _, entry := range targets {
			basename := entry.Name()
			task <- Task{
				srcFile:    path.Join(srcDir, basename),
				outputFile: path.Join(outputDir, basename),
			}
		}
		close(task)
	}()

	for i := 0; i < jobCnt; i++ {
		go decypherWorker(translate, task, result)
	}

	totalCnt := len(targets)
	for resultCnt := 0; resultCnt < totalCnt; resultCnt++ {
		r := <-result
		if r.err == nil {
			log.Println("ok:", r.task.outputFile)
		} else {
			log.Println(r.err)
		}
	}

	return nil
}

func decypherWorker(translate map[rune]rune, taskChan chan Task, resultChan chan Result) {
	for task := <-taskChan; task.srcFile != "" && task.outputFile != ""; task = <-taskChan {
		resultChan <- Result{
			err:  decypherSingleFile(translate, task.srcFile, task.outputFile),
			task: &task,
		}
	}
}
