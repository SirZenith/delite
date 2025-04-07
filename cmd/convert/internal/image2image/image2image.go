package image2image

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
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

	ctx := context.Background()
	ctx = context.WithValue(ctx, "options", &options)
	ctx = context.WithValue(ctx, "format", format)
	ctx = context.WithValue(ctx, "group", &group)
	ctx = context.WithValue(ctx, "taskChan", taskChan)

	for i := options.jobCnt; i > 0; i-- {
		go conversionWorker(ctx)
	}

	if fileMode.IsRegular() {
		group.Add(1)
		taskChan <- target
	} else {
		entryList, err := os.ReadDir(target)
		if err != nil {
			return fmt.Errorf("failed to read directory %s: %s", target, err)
		}

		for _, entry := range entryList {
			name := filepath.Join(target, entry.Name())
			group.Add(1)
			taskChan <- name
		}
	}

	group.Wait()

	return nil
}

func conversionWorker(ctx context.Context) {
	options := ctx.Value("options").(*options)
	format := ctx.Value("format").(string)
	group := ctx.Value("group").(*sync.WaitGroup)
	taskChan := ctx.Value("taskChan").(chan string)

	for filePath := range taskChan {
		err := convertImage(filePath, format)
		if err == nil {
			if options.removeSource {
				os.Remove(filePath)
			}
		} else {
			log.Error(err.Error())
		}

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

	dstFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to oppen output file %s: %s", outputName, err)
	}
	defer dstFile.Close()

	reader := bufio.NewReader(srcFile)
	writer := bufio.NewWriter(dstFile)
	defer writer.Flush()

	_, err = common.ConvertImageTo(reader, writer, format)
	if err != nil {
		return fmt.Errorf("conversion failed %s: %s", filePath, err)
	}

	log.Debugf("convert: %s -> %s", filePath, outputName)

	return nil
}
