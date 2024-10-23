package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirZenith/litnovel-dl/base"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	var dir string

	cmd := &cli.Command{
		Name:  "library",
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
			subCmdInit(),
			subCmdAddHeaderFile(),
			subCmdAddBook(),
		},
	}

	return cmd
}

// Reads library info form existing library.json
func readExistingInfo(filename string) (base.LibraryInfo, error) {
	info := base.LibraryInfo{}

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
func updateDefaultValue(info *base.LibraryInfo) {
	info.RootDir = base.GetStrOr(info.RootDir, "./")
	info.RawDirName = base.GetStrOr(info.RawDirName, "raw")
	info.TextDirName = base.GetStrOr(info.TextDirName, "text")
	info.ImgDirName = base.GetStrOr(info.ImgDirName, "image")
	info.EpubDirName = base.GetStrOr(info.EpubDirName, "epub")

	info.NameMapFile = base.GetStrOr(info.NameMapFile, "name_map.json")

	if info.HeaderFileList == nil {
		info.HeaderFileList = []base.HeaderFilePattern{}
	}

	if info.Books == nil {
		info.Books = []base.BookInfo{}
	}
}

// Save book info struct to file.
func saveInfoFile(info *base.LibraryInfo, filename string) error {
	data, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		return fmt.Errorf("JSON conversion failed: %s", err)
	}

	err = os.WriteFile(filename, data, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write info file: %s", err)
	}

	return nil
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

			return saveInfoFile(&info, outputName)
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
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "path of library.json file to be modified",
				Value:   "./library.json",
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
				UsageText:   "<path>",
				Destination: &path,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("file")
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read info file %s: %s", filePath, err)
			}

			info := &base.LibraryInfo{}
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

			info.HeaderFileList = append(info.HeaderFileList, base.HeaderFilePattern{
				Pattern: pattern,
				Path:    path,
			})

			return saveInfoFile(info, filePath)
		},
	}

	return cmd
}

func subCmdAddBook() *cli.Command {
	cmd := &cli.Command{
		Name:  "book",
		Usage: "add book entry to library.json",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "path of library.json file to be modified",
				Value:   "./library.json",
			},
			&cli.StringFlag{
				Name:     "title",
				Aliases:  []string{"t"},
				Usage:    "book title",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "author",
				Aliases:  []string{"a"},
				Usage:    "book author",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "toc",
				Usage:    "TOC URL of the book",
				Required: true,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("file")
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read info file %s: %s", filePath, err)
			}

			info := &base.LibraryInfo{}
			err = json.Unmarshal(data, info)
			if err != nil {
				return fmt.Errorf("failed to parse info file %s: %s", filePath, err)
			}

			toc := cmd.String("toc")
			if info.Books != nil {
				for i, book := range info.Books {
					if book.TocURL == toc {
						return fmt.Errorf("a book with the same TOC URL already exists a index %d", i)
					}
				}
			}

			info.Books = append(info.Books, base.BookInfo{
				Title:  cmd.String("title"),
				Author: cmd.String("author"),

				TocURL: toc,
			})

			return saveInfoFile(info, filePath)
		},
	}

	return cmd
}
