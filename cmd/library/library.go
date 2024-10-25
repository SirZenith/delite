package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	book_mgr "github.com/SirZenith/litnovel-dl/book_management"
	"github.com/SirZenith/litnovel-dl/common"
	"github.com/charmbracelet/log"
	"github.com/jeandeaual/go-locale"
	"github.com/urfave/cli/v3"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "library",
		Usage: "create library.json file under given directory",
		Commands: []*cli.Command{
			subCmdInit(),

			subCmdAddHeaderFile(),

			subCmdAddBook(),
			subCmdAddEmptyBook(),
			subCmdSortBook(),
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

	info.NameMapFile = common.GetStrOr(info.NameMapFile, "name_map.json")

	if info.HeaderFileList == nil {
		info.HeaderFileList = []book_mgr.HeaderFilePattern{}
	}

	if info.Books == nil {
		info.Books = []book_mgr.BookInfo{}
	}
}

// Save book info struct to file.
func saveInfoFile(info *book_mgr.LibraryInfo, filename string) error {
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

			info := &book_mgr.LibraryInfo{}
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

			info.Books = append(info.Books, book_mgr.BookInfo{
				Title:  cmd.String("title"),
				Author: cmd.String("author"),

				TocURL: toc,
			})

			return saveInfoFile(info, filePath)
		},
	}

	return cmd
}

func subCmdAddEmptyBook() *cli.Command {
	cmd := &cli.Command{
		Name:  "book-empty",
		Usage: "add an empty book entry to library.json",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "path of library.json file to be modified",
				Value:   "./library.json",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("file")
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read info file %s: %s", filePath, err)
			}

			info := &book_mgr.LibraryInfo{}
			err = json.Unmarshal(data, info)
			if err != nil {
				return fmt.Errorf("failed to parse info file %s: %s", filePath, err)
			}

			info.Books = append(info.Books, book_mgr.BookInfo{})

			return saveInfoFile(info, filePath)
		},
	}

	return cmd
}

type BookList []book_mgr.BookInfo

func (b BookList) Len() int {
	return len(b)
}

func (b BookList) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b BookList) Bytes(i int) []byte {
	title := b[i].Title
	return []byte(title)
}

func subCmdSortBook() *cli.Command {
	cmd := &cli.Command{
		Name:  "sort-books",
		Usage: "add an empty book entry to library.json",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "path of library.json file to be modified",
				Value:   "./library.json",
			},
			&cli.StringFlag{
				Name:    "locale",
				Aliases: []string{"l"},
				Usage:   "IETF BCP 47 language tag to be used as sorting language",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("file")
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read info file %s: %s", filePath, err)
			}

			info := &book_mgr.LibraryInfo{}
			err = json.Unmarshal(data, info)
			if err != nil {
				return fmt.Errorf("failed to parse info file %s: %s", filePath, err)
			}

			if info.Books == nil {
				return nil
			}

			langTag := language.AmericanEnglish

			lang := cmd.String("locale")
			if lang != "" {
				if parsedTag, err := language.Parse(lang); err == nil {
					langTag = parsedTag
				} else {
					log.Warnf("invalid locale, fallback to %s: %s", langTag, err)
				}
			} else if lang, err := locale.GetLocale(); err == nil {
				if parsedTag, err := language.Parse(lang); err == nil {
					langTag = parsedTag
					log.Infof("detected sort locale: %s", langTag)
				}
			}

			list := collate.New(langTag)
			list.Sort(BookList(info.Books))

			return saveInfoFile(info, filePath)
		},
	}

	return cmd
}
