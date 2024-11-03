package zip

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	_ "github.com/gen2brain/avif"
	"github.com/urfave/cli/v3"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/ccitt"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const defaultOutputName = "out"

func Cmd() *cli.Command {
	var libIndex int64

	cmd := &cli.Command{
		Name:  "zip",
		Usage: "bundle downloaded manga into ZIP archive with infomation provided in info.json of the book",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output directory to save zip file to",
			},
			&cli.IntFlag{
				Name:    "job",
				Aliases: []string{"j"},
				Usage:   "job count for image decode/encoding",
				Value:   int64(runtime.NumCPU()),
			},
			&cli.StringFlag{
				Name:  "info-file",
				Usage: "path to info json file",
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path to library info JSON.",
			},
		},
		Arguments: []cli.Argument{
			&cli.IntArg{
				Name:        "library-index",
				UsageText:   "<index>",
				Destination: &libIndex,
				Value:       -1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, targets, err := getOptionsFromCmd(cmd, int(libIndex))
			if err != nil {
				return err
			}

			return cmdMain(options, targets)
		},
	}

	return cmd
}

type MakeBookTarget struct {
	TextDir   string
	ImageDir  string
	OutputDir string
	BookTitle string
	Author    string
}

type options struct {
	jobCnt int
}

type workload struct {
	options *options

	title      string
	author     string
	outputName string
	imgDir     string
}

func getOptionsFromCmd(cmd *cli.Command, libIndex int) (options, []MakeBookTarget, error) {
	options := options{
		jobCnt: int(cmd.Int("job")),
	}

	targets := []MakeBookTarget{}

	target, err := getTargetFromCmd(cmd)
	if err != nil {
		return options, targets, err
	} else if target.OutputDir != "" {
		targets = append(targets, target)
	}

	libraryInfoPath := cmd.String("library")
	if libraryInfoPath != "" {
		targetList, err := loadLibraryTargets(libraryInfoPath)
		if err != nil {
			return options, targets, err
		}

		if 0 <= libIndex && libIndex < len(targetList) {
			targets = append(targets, targetList[libIndex])
		} else {
			targets = append(targets, targetList...)
		}
	}

	return options, targets, nil
}

func getTargetFromCmd(cmd *cli.Command) (MakeBookTarget, error) {
	target := MakeBookTarget{
		OutputDir: cmd.String("output"),
	}

	infoFile := cmd.String("info-file")
	if infoFile != "" {
		bookInfo, err := book_mgr.ReadBookInfo(infoFile)
		if err != nil {
			return target, err
		}

		target.TextDir = bookInfo.TextDir
		target.ImageDir = bookInfo.ImgDir
		target.BookTitle = bookInfo.Title
		target.Author = bookInfo.Author

		if target.OutputDir == "" {
			if bookInfo.ZipDir != "" {
				target.OutputDir = bookInfo.ZipDir
			} else {
				target.OutputDir = filepath.Dir(infoFile)
			}
		}
	}

	return target, nil
}

// loadLibraryTargets reads book list from library info JSON and returns them
// as a list of MakeBookTarget.
func loadLibraryTargets(libInfoPath string) ([]MakeBookTarget, error) {
	info, err := book_mgr.ReadLibraryInfo(libInfoPath)
	if err != nil {
		return nil, err
	}

	targets := []MakeBookTarget{}
	for _, book := range info.Books {
		targets = append(targets, MakeBookTarget{
			TextDir:   book.TextDir,
			ImageDir:  book.ImgDir,
			OutputDir: book.ZipDir,
			BookTitle: book.Title,
			Author:    book.Author,
		})
	}

	return targets, nil
}

func cmdMain(options options, targets []MakeBookTarget) error {
	for _, target := range targets {
		logWorkBeginBanner(target)

		entryList, err := os.ReadDir(target.ImageDir)
		if err != nil {
			log.Errorf("failed to read directory %s: %s", target.TextDir, err)
			continue
		}

		err = os.MkdirAll(target.OutputDir, 0o755)
		if err != nil {
			log.Errorf("failed to create output directory %s: %s", target.OutputDir, err)
			continue
		}

		for _, child := range entryList {
			volumeName := child.Name()

			title := fmt.Sprintf("%s %s", target.BookTitle, volumeName)

			outputName := filepath.Join(target.OutputDir, title+".zip")

			imgDir := filepath.Join(target.ImageDir, volumeName)

			err = makeBook(workload{
				options: &options,

				title:      title,
				author:     target.Author,
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
func logWorkBeginBanner(target MakeBookTarget) {
	msgs := []string{
		fmt.Sprintf("%-12s: %s", "title", target.BookTitle),
		fmt.Sprintf("%-12s: %s", "author", target.Author),
		fmt.Sprintf("%-12s: %s", "text   dir", target.TextDir),
		fmt.Sprintf("%-12s: %s", "image  dir", target.ImageDir),
		fmt.Sprintf("%-12s: %s", "output dir", target.OutputDir),
	}

	common.LogBannerMsg(msgs, 5)
}

type archiveResult struct {
	imgPath string
	data    []byte
	err     error
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
				data, err := writerImageToArchive(name, imgPath)
				resultChan <- archiveResult{
					imgPath: imgPath,
					data:    data,
					err:     err,
				}
			}
		}()
	}

	totalCnt := len(entryList)
	finishedCnt := 0
	for result := range resultChan {
		if result.err == nil {
			basename := filepath.Base(result.imgPath)
			ext := filepath.Ext(basename)
			outputName := basename[:len(basename)-len(ext)] + ".png"

			if writer, err := zipWriter.Create(outputName); err == nil {
				writer.Write(result.data)
				log.Infof("done: %s", result.imgPath)
			} else {
				log.Warnf("failed to create archive entry with name %s: %s", outputName, err)
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

func writerImageToArchive(name string, imgPath string) ([]byte, error) {
	imgFile, err := os.Open(imgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image file %s: %s", imgPath, err)
	}
	defer imgFile.Close()

	img, _, err := image.Decode(imgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to deocde image %s: %s", imgPath, err)
	}

	writer := bytes.NewBuffer([]byte{})
	err = png.Encode(writer, img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode image %s as png: %s", imgPath, err)
	}

	return writer.Bytes(), nil
}
