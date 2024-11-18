package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/cmd/library/book"
	"github.com/SirZenith/delite/cmd/library/config"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "library",
		Usage: "create library.json file under given directory",
		Commands: []*cli.Command{
			subCmdInit(),
			subCmdAddHeaderFile(),
			subCmdAddLimitRule(),

			book.Cmd(),
			config.Cmd(),
		},
	}

	return cmd
}

// Reads library info form existing library.json
func readExistingInfo(filename string) (book_mgr.LibraryInfo, error) {
	info := book_mgr.LibraryInfo{}

	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return info, nil
		}
		return info, err
	}

	err = json.Unmarshal(data, &info)
	if err != nil {
		return info, err
	}

	return info, nil
}

// Setup default value of library info.
func updateDefaultValue(info *book_mgr.LibraryInfo) {
	info.RootDir = common.GetStrOr(info.RootDir, "./")
	info.RawDirName = common.GetStrOr(info.RawDirName, "raw")
	info.TextDirName = common.GetStrOr(info.TextDirName, "text")
	info.ImgDirName = common.GetStrOr(info.ImgDirName, "image")
	info.EpubDirName = common.GetStrOr(info.EpubDirName, "epub")
	info.LatexDirName = common.GetStrOr(info.LatexDirName, "latex")
	info.ZipDirName = common.GetStrOr(info.ZipDirName, "zip")

	info.DatabasePath = common.GetStrOr(info.DatabasePath, "./library.db")

	if info.HeaderFileList == nil {
		info.HeaderFileList = []book_mgr.HeaderFilePattern{}
	}

	if info.Books == nil {
		info.Books = []book_mgr.BookInfo{}
	}
}

func subCmdInit() *cli.Command {
	var dir string

	cmd := &cli.Command{
		Name:  "init",
		Usage: "create library.json file under given directory",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "path",
				UsageText:   "<directory>",
				Value:       "./",
				Destination: &dir,
				Max:         1,
			},
		},
		Commands: []*cli.Command{
			subCmdAddHeaderFile(),
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			outputName := filepath.Join(dir, "library.json")

			info, err := readExistingInfo(outputName)
			if err != nil {
				log.Infof("failed to read existing info file: %s, go on processing any way", err)
			}

			updateDefaultValue(&info)

			return info.SaveFile(outputName)
		},
	}

	return cmd
}

func subCmdAddHeaderFile() *cli.Command {
	var pattern string
	var path string

	cmd := &cli.Command{
		Name:  "header",
		Usage: "add header file to library.json",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "pattern",
				UsageText:   "<pattern>",
				Destination: &pattern,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "path",
				UsageText:   " <path>",
				Destination: &path,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("library")
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read info file %s: %s", filePath, err)
			}

			info := &book_mgr.LibraryInfo{}
			err = json.Unmarshal(data, info)
			if err != nil {
				return fmt.Errorf("failed to parse info file %s: %s", filePath, err)
			}

			if info.HeaderFileList != nil {
				for i, entry := range info.HeaderFileList {
					if entry.Pattern == pattern {
						return fmt.Errorf("an entry with the same patter already exists at index %d", i)
					}
				}
			}

			info.HeaderFileList = append(info.HeaderFileList, book_mgr.HeaderFilePattern{
				Pattern: pattern,
				Path:    path,
			})

			return info.SaveFile(filePath)
		},
	}

	return cmd
}

func subCmdAddLimitRule() *cli.Command {
	cmd := &cli.Command{
		Name:  "limit",
		Usage: "add limit rule to library.json",
		Flags: []cli.Flag{
			&cli.DurationFlag{
				Name:    "delay",
				Aliases: []string{"d"},
				Usage:   "request delay",
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
			&cli.StringFlag{
				Name:    "glob",
				Aliases: []string{"g"},
				Usage:   "domain glob pattern",
			},
			&cli.IntFlag{
				Name:    "parallelism",
				Aliases: []string{"p"},
				Usage:   "maxium paralle request job count",
			},
			&cli.StringFlag{
				Name:    "regex",
				Aliases: []string{"r"},
				Usage:   "domain regex pattern",
			},
			&cli.DurationFlag{
				Name:    "random-delay",
				Aliases: []string{"D"},
				Usage:   "extra random delay besides `delay`",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("library")
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read info file %s: %s", filePath, err)
			}

			info := &book_mgr.LibraryInfo{}
			err = json.Unmarshal(data, info)
			if err != nil {
				return fmt.Errorf("failed to parse info file %s: %s", filePath, err)
			}

			info.LimitRules = append(info.LimitRules, book_mgr.LimitRule{
				DomainRegexp: cmd.String("regex"),
				DomainGlob:   cmd.String("glob"),
				Delay:        cmd.Duration("delay"),
				RandomDelay:  cmd.Duration("random-delay"),
				Parallelism:  int(cmd.Int("parallelism")),
			})

			return info.SaveFile(filePath)
		},
	}

	return cmd
}
