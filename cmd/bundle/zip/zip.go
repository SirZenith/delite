package zip

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
)

const defaultOutputName = "out"

func Cmd() *cli.Command {
	var bookIndex int64
	var volumeIndex int64

	cmd := &cli.Command{
		Name:  "zip",
		Usage: "bundle downloaded manga into ZIP archive with infomation provided in info.json of the book",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Usage:   "image format used in output archive file. Available formats are: " + strings.Join(common.AllImageFormats, ", "),
			},
			&cli.IntFlag{
				Name:    "job",
				Aliases: []string{"j"},
				Usage:   "job count for image decode/encoding",
				Value:   int64(runtime.NumCPU()),
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path to library info JSON file",
				Value: "./library.json",
			},
		},
		Arguments: []cli.Argument{
			&cli.IntArg{
				Name:        "book-index",
				UsageText:   "<book-index>",
				Destination: &bookIndex,
				Value:       -1,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "volume-index",
				UsageText:   "<volume-index>",
				Destination: &volumeIndex,
				Value:       -1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, targets, err := getOptionsFromCmd(cmd, int(bookIndex), int(volumeIndex))
			if err != nil {
				return err
			}

			return cmdMain(options, targets)
		},
	}

	return cmd
}

type bookInfo struct {
	textDir   string
	imageDir  string
	outputDir string
	bookTitle string
	author    string

	targetVolume  int
	isUnsupported bool
}

type options struct {
	jobCnt int
	format string
}

type workload struct {
	options *options

	title      string
	author     string
	outputName string
	imgDir     string
}

func getOptionsFromCmd(cmd *cli.Command, bookIndex, volumeIndex int) (options, []bookInfo, error) {
	options := options{
		jobCnt: int(cmd.Int("job")),
		format: cmd.String("format"),
	}

	if slices.Index(common.AllImageFormats, options.format) < 0 {
		return options, nil, fmt.Errorf("unsupported output image format: %q", options.format)
	}

	libFilePath := cmd.String("library")
	targets, err := loadLibraryTargets(libFilePath, bookIndex, volumeIndex)
	if err != nil {
		return options, targets, err
	}

	return options, targets, nil
}

// loadLibraryTargets reads book list from library info JSON and returns them
// as a list of MakeBookTarget.
func loadLibraryTargets(libInfoPath string, bookIndex, volumeIndex int) ([]bookInfo, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	targets := []bookInfo{}
	for index, book := range info.Books {
		if bookIndex >= 0 && index != bookIndex {
			continue
		}

		targets = append(targets, bookInfo{
			textDir:   book.TextDir,
			imageDir:  book.ImgDir,
			outputDir: book.ZipDir,
			bookTitle: book.Title,
			author:    book.Author,

			targetVolume:  volumeIndex,
			isUnsupported: book.LocalInfo != nil && book.LocalInfo.Type != book_mgr.LocalBookTypeImage,
		})
	}

	return targets, nil
}

func cmdMain(options options, targets []bookInfo) error {
	for _, target := range targets {
		logWorkBeginBanner(target)

		if target.isUnsupported {
			log.Info("skip unsupported resource")
			continue
		}

		entryList, err := os.ReadDir(target.imageDir)
		if err != nil {
			log.Errorf("failed to read directory %s: %s", target.textDir, err)
			continue
		}

		err = os.MkdirAll(target.outputDir, 0o777)
		if err != nil {
			log.Errorf("failed to create output directory %s: %s", target.outputDir, err)
			continue
		}

		for index, child := range entryList {
			if target.targetVolume >= 0 && index != target.targetVolume {
				continue
			}

			volumeName := child.Name()

			title := fmt.Sprintf("%s %s", target.bookTitle, volumeName)

			outputName := filepath.Join(target.outputDir, title+".zip")

			imgDir := filepath.Join(target.imageDir, volumeName)

			err = makeBook(workload{
				options: &options,

				title:      title,
				author:     target.author,
				outputName: outputName,
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
func logWorkBeginBanner(target bookInfo) {
	msgs := []string{
		fmt.Sprintf("%-12s: %s", "title", target.bookTitle),
		fmt.Sprintf("%-12s: %s", "author", target.author),
		fmt.Sprintf("%-12s: %s", "text   dir", target.textDir),
		fmt.Sprintf("%-12s: %s", "image  dir", target.imageDir),
		fmt.Sprintf("%-12s: %s", "output dir", target.outputDir),
	}

	common.LogBannerMsg(msgs, 5)
}

type archiveResult struct {
	outputName string
	data       []byte
	err        error
}

func makeBook(info workload) error {
	entryList, err := os.ReadDir(info.imgDir)
	if err != nil {
		return fmt.Errorf("failed to access image asset directory %s: %s", info.imgDir, err)
	}

	file, err := os.Create(info.outputName)
	if err != nil {
		return fmt.Errorf("failed to create output file: %s", err)
	}
	defer file.Close()

	bufWriter := bufio.NewWriter(file)
	defer bufWriter.Flush()

	zipWriter := zip.NewWriter(bufWriter)
	defer zipWriter.Close()

	taskChan := make(chan string, info.options.jobCnt)
	resultChan := make(chan archiveResult, info.options.jobCnt)

	go func() {
		for _, entry := range entryList {
			taskChan <- entry.Name()
		}
		close(taskChan)
	}()

	for i := 0; i < int(info.options.jobCnt); i++ {
		go func() {
			for name := range taskChan {
				imgPath := filepath.Join(info.imgDir, name)
				data, outputName, err := writerImageToArchive(imgPath, info.options.format)
				resultChan <- archiveResult{
					outputName: outputName,
					data:       data,
					err:        err,
				}
			}
		}()
	}

	totalCnt := len(entryList)
	finishedCnt := 0
	for result := range resultChan {
		if result.err == nil {
			if writer, err := zipWriter.Create(result.outputName); err == nil {
				writer.Write(result.data)
				log.Infof("done: %s", result.outputName)
			} else {
				log.Warnf("failed to create archive entry with name %s: %s", result.outputName, err)
			}
		} else {
			log.Warnf("%s", result.err)
		}

		finishedCnt++
		if finishedCnt >= totalCnt {
			break
		}
	}

	return nil
}

func writerImageToArchive(imgPath string, outputFormat string) ([]byte, string, error) {
	imgFile, err := os.Open(imgPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open image file %s: %s", imgPath, err)
	}
	defer imgFile.Close()

	writer := bytes.NewBuffer([]byte{})
	outputExt, err := common.ConvertImageTo(imgFile, writer, outputFormat)
	if err != nil {
		return nil, "", err
	}

	basename := filepath.Base(imgPath)
	ext := filepath.Ext(basename)
	outputName := basename[:len(basename)-len(ext)] + "." + outputExt

	return writer.Bytes(), outputName, nil
}
