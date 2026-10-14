package image2image

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	var target string
	var format string

	return &cli.Command{
		Name:  "image2image",
		Usage: "convert images to another format",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "job",
				Aliases: []string{"j"},
				Usage:   "conversion job count",
				Value:   int64(runtime.NumCPU()),
			},
			&cli.BoolFlag{
				Name:  "remove-source",
				Usage: "remove source file after conversion",
				Value: false,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "target",
				UsageText:   "<path>",
				Destination: &target,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "format",
				UsageText:   " <format>",
				Destination: &format,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd)
			if err != nil {
				return err
			}

			return cmdMain(options, target, format)
		},
	}
}

type options struct {
	jobCnt       int
	removeSource bool
}

func getOptionsFromCmd(cmd *cli.Command) (options, error) {
	options := options{
		jobCnt:       int(cmd.Int("job")),
		removeSource: cmd.Bool("remove-source"),
	}

	return options, nil
}

func cmdMain(options options, target, format string) error {
	stat, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("failed to access %s: %s", target, err)
	}

	fileMode := stat.Mode()
	if !fileMode.IsDir() && !fileMode.IsRegular() {
		return fmt.Errorf("target path does not point to a directory or file: %s", target)
	}

	var group sync.WaitGroup
	taskChan := make(chan string, options.jobCnt)

	bar := progressbar.NewOptions(
		0,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(5),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "options", &options)
	ctx = context.WithValue(ctx, "format", format)
	ctx = context.WithValue(ctx, "group", &group)
	ctx = context.WithValue(ctx, "taskChan", taskChan)
	ctx = context.WithValue(ctx, "bar", bar)

	for i := options.jobCnt; i > 0; i-- {
		go conversionWorker(ctx)
	}

	if fileMode.IsRegular() {
		bar.ChangeMax(1)
		bar.Reset()

		group.Add(1)
		taskChan <- target
	} else {
		entryList, err := os.ReadDir(target)
		if err != nil {
			return fmt.Errorf("failed to read directory %s: %s", target, err)
		}

		bar.ChangeMax(len(entryList))
		bar.Reset()

		for _, entry := range entryList {
			name := filepath.Join(target, entry.Name())
			group.Add(1)
			taskChan <- name
		}
	}

	group.Wait()

	fmt.Println()

	return nil
}

func conversionWorker(ctx context.Context) {
	options := ctx.Value("options").(*options)
	format := ctx.Value("format").(string)
	group := ctx.Value("group").(*sync.WaitGroup)
	taskChan := ctx.Value("taskChan").(chan string)
	bar := ctx.Value("bar").(*progressbar.ProgressBar)

	for filePath := range taskChan {
		err := convertImage(filePath, format)
		if err == nil {
			if options.removeSource {
				os.Remove(filePath)
			}
		} else {
			log.Error(err.Error())
		}

		bar.Describe(filepath.Base(filePath))
		bar.Add(1)
		group.Done()
	}
}

func convertImage(filePath, format string) error {
	outputName := common.ReplaceFileExt(filePath, "."+format)
	if filePath == outputName {
		return fmt.Errorf("output file cannot be the same file as source file")
	}

	srcFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %s", filePath, err)
	}
	defer srcFile.Close()

	stat, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file %s: %s", filePath, err)
	}
	if stat.Size() == 0 {
		return fmt.Errorf("source file is empty: %s", filePath)
	}

	// Write to a temporary file in the same directory first, and only rename it to
	// the final name after the conversion succeeded, so that a failed or
	// interrupted conversion never leaves a broken (e.g. zero byte) output file
	// behind, which would be mistaken for a valid image on a later run.
	dstFile, err := os.CreateTemp(filepath.Dir(outputName), "."+filepath.Base(outputName)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp output file for %s: %s", outputName, err)
	}
	tempName := dstFile.Name()

	reader := bufio.NewReader(srcFile)
	writer := bufio.NewWriter(dstFile)

	_, err = common.ConvertImageTo(reader, writer, format)
	if err == nil {
		err = writer.Flush()
	}

	// The temp file cannot be renamed while it is still open on some platforms.
	if closeErr := dstFile.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("conversion failed %s: %s", filePath, err)
	}

	if err = os.Rename(tempName, outputName); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("failed to write output file %s: %s", outputName, err)
	}

	log.Debugf("convert: %s -> %s", filePath, outputName)

	return nil
}
