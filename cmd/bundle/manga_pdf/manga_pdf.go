package manga_pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/schollz/progressbar/v3"
	"github.com/signintech/gopdf"
	"github.com/urfave/cli/v3"
)

var imgExts = []string{".jpg", ".png", ".jpeg", ".gif"}

func Cmd() *cli.Command {
	var srcPath string
	var saveAs string

	cmd := &cli.Command{
		Name: "pdf",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "source-dir",
				UsageText:   "<source-dir>",
				Destination: &srcPath,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "output",
				UsageText:   "<output>",
				Destination: &saveAs,
				Max:         1,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			basename := filepath.Base(srcPath)
			defaultOutputName := basename + ".pdf"

			saveAs = common.GetStrOr(saveAs, defaultOutputName)
			if stat, err := os.Stat(saveAs); err == nil && stat.IsDir() {
				saveAs = filepath.Join(saveAs, defaultOutputName)
			}

			return cmdMain(srcPath, saveAs)
		},
	}

	return cmd
}

func cmdMain(srcPath, saveAs string) error {
	var err error

	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})

	trimBox := &gopdf.Box{
		Left:   0,
		Right:  0,
		Top:    0,
		Bottom: 0,
	}

	entries, err := os.ReadDir(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %s", err)
	}

	files := []string{}
	for _, entry := range entries {
		name := entry.Name()

		ext := filepath.Ext(name)
		ext = strings.ToLower(ext)
		if slices.Index(imgExts, ext) >= 0 {
			files = append(files, name)
		}
	}

	totalCnt := len(files)
	if totalCnt <= 0 {
		log.Warnf("%s is contains no images", srcPath)
		return nil
	}

	sort.Strings(files)

	bar := progressbar.Default(int64(totalCnt))
	for _, name := range files {
		imgName := filepath.Join(srcPath, name)
		imgObj := new(gopdf.ImageObj)
		err = imgObj.SetImagePath(imgName)
		if err != nil {
			log.Warnf("error loading image object %s: %s", imgName, err)
			continue
		}

		pdf.AddPageWithOption(gopdf.PageOption{
			TrimBox:  trimBox,
			PageSize: imgObj.GetRect(),
		})

		err = pdf.Image(imgName, 0, 0, imgObj.GetRect())
		if err != nil {
			log.Warnf("failed to add image %s: %s", imgName, err)
			continue
		}

		bar.Add(1)
	}

	err = pdf.WritePdf(saveAs)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %s", saveAs, err)
	}

	log.Infof("output save as: %s", saveAs)

	return nil
}
