package book

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/jeandeaual/go-locale"
	"github.com/urfave/cli/v3"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "book",
		Usage: "operation for manipulating book list in library",
		Commands: []*cli.Command{
			subCmdInit(),
			subCmdAdd(),
			subCmdAddEmpty(),
			subCmdList(),
			subCmdSort(),
		},
	}

	return cmd
}

// Read book info form existing info.json
func readExistingInfo(filename string) (book_mgr.BookInfo, error) {
	info := book_mgr.BookInfo{}

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

// Setup default value of book info.
func updateDefaultValue(info *book_mgr.BookInfo) {
	info.RootDir = common.GetStrOr(info.RootDir, "./")
	info.RawDir = common.GetStrOr(info.RawDir, "./text_raw")
	info.TextDir = common.GetStrOr(info.TextDir, "./text")
	info.ImgDir = common.GetStrOr(info.ImgDir, "./image")
	info.EpubDir = common.GetStrOr(info.EpubDir, "./epub")
	info.LatexDir = common.GetStrOr(info.EpubDir, "./latex")

	info.HeaderFile = common.GetStrOr(info.HeaderFile, "../header.json")
	info.NameMapFile = common.GetStrOr(info.NameMapFile, "./name_map.json")
}

func subCmdInit() *cli.Command {
	var dir string

	cmd := &cli.Command{
		Name:  "init",
		Usage: "initialize a book info.json file in given directory.",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "directory",
				UsageText:   "<path>",
				Destination: &dir,
				Max:         1,
				Value:       "./",
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			outputName := filepath.Join(dir, "info.json")

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

func subCmdAdd() *cli.Command {
	cmd := &cli.Command{
		Name:  "add",
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

			return info.SaveFile(filePath)
		},
	}

	return cmd
}

func subCmdAddEmpty() *cli.Command {
	cmd := &cli.Command{
		Name:  "empty",
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

			return info.SaveFile(filePath)
		},
	}

	return cmd
}

func subCmdList() *cli.Command {
	var libIndex int64

	cmd := &cli.Command{
		Name:  "list",
		Usage: "add an empty book entry to library.json",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "path of library.json file to be modified",
				Value:   "./library.json",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "print complete information of books",
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

			isVerbose := cmd.Bool("verbose")
			for index, book := range info.Books {
				if libIndex >= 0 && index != int(libIndex) {
					continue
				}

				fmt.Printf("%d. %s\n", index, common.GetStrOr(book.Title, "no-title"))

				if isVerbose {
					fmt.Println("  author:", book.Author)
					fmt.Println("  TOC   :", book.Author)
					fmt.Println("  root        :", book.RootDir)
					fmt.Println("  raw output  :", book.RawDir)
					fmt.Println("  text output :", book.TextDir)
					fmt.Println("  image output:", book.ImgDir)
					fmt.Println("  header  :", book.HeaderFile)
					fmt.Println("  name map:", book.NameMapFile)
				}
			}

			return nil
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

func subCmdSort() *cli.Command {
	cmd := &cli.Command{
		Name:  "sort",
		Usage: "apply localized sort to book list in library.json",
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

			return info.SaveFile(filePath)
		},
	}

	return cmd
}
