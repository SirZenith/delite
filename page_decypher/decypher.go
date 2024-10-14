package page_decypher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"

	"github.com/bilinovel/base"
	"github.com/playwright-community/playwright-go"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
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
				Name:    "browser-type",
				Aliases: []string{"t"},
				Value:   "firefox",
				Usage:   "browser driver type to use, possible values are: firefox, chromium, webkit",
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
	jobCnt      int
	browserType string
	target      string
	output      string
}

func getDecypherOptionsFromCmd(cmd *cli.Command) (Options, error) {
	options := Options{
		jobCnt:      int(cmd.Int("job")),
		browserType: cmd.String("browser-type"),
		target:      cmd.String("intput"),
		output:      cmd.String("output"),
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
	}

	if options.output == "" {
		options.output = options.target
	}

	return options, nil
}

func pageDecypher(options Options) error {
	info, err := os.Stat(options.target)
	if err != nil {
		return err
	}

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %s", err)
	}
	defer func() {
		if err = pw.Stop(); err != nil {
			log.Println("could not stop Playwright:", err)
		}
	}()

	browser, err := getBrowser(pw, options.browserType)
	if err != nil {
		return fmt.Errorf("could not launch browser: %s", err)
	}
	defer func() {
		if err = browser.Close(); err != nil {
			log.Println("could not close browser:", err)
		}
	}()

	if info.IsDir() {
		return renderDirectory(browser, options.target, options.output, options.jobCnt)
	} else if page, err := browser.NewPage(); err == nil {
		return renderSingleFile(page, options.target, options.output)
	} else {
		return fmt.Errorf("could not create page: %v", err)
	}
}

func getBrowser(pw *playwright.Playwright, typ string) (playwright.Browser, error) {
	var browserType playwright.BrowserType
	switch typ {
	case "firefox":
		browserType = pw.Firefox
	case "chromium":
		browserType = pw.Chromium
	case "webkit":
		browserType = pw.WebKit
	default:
		browserType = pw.Firefox
	}

	return browserType.Launch()
}

func getTargetContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read target file %s: %s", path, err)
	}

	return TEMPLATE_HEADER + string(data) + TEMPLATE_FOOTER, nil
}

func renderSingleFile(page playwright.Page, srcFile string, outputFile string) error {
	info, err := os.Stat(srcFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %s", srcFile, err)
	}

	content, err := getTargetContent(srcFile)
	if err != nil {
		return fmt.Errorf("failed to read target file: %s", err)
	}

	err = page.SetContent(content)
	if err != nil {
		return fmt.Errorf("failed to set page content: %s", err)
	}

	container := page.Locator("#acontentz").First()
	rendered, err := container.InnerHTML()
	if err != nil {
		return fmt.Errorf("failed to get rendered contnet: %s", err)
	}

	err = os.WriteFile(outputFile, []byte(rendered), info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to write file %s: %s", outputFile, err)
	}

	return nil
}

func renderDirectory(browser playwright.Browser, srcDir, outputDir string, jobCnt int) error {
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
		go pageRenderWorker(browser, task, result)
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

func pageRenderWorker(browser playwright.Browser, taskChan chan Task, resultChan chan Result) {
	page, err := browser.NewPage()
	if err != nil {
		resultChan <- Result{
			err:  fmt.Errorf("could not create page: %s", err),
			task: nil,
		}
		return
	}

	for task := <-taskChan; task.srcFile != "" && task.outputFile != ""; task = <-taskChan {
		resultChan <- Result{
			err:  renderSingleFile(page, task.srcFile, task.outputFile),
			task: &task,
		}
	}
}
