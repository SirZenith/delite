package book

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

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
			subCmdAdd(),
			subCmdAddEmpty(),
			subCmdList(),
			subCmdListVolume(),
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
	info.LatexDir = common.GetStrOr(info.LatexDir, "./latex")
	info.ZipDir = common.GetStrOr(info.EpubDir, "./zip")

	info.HeaderFile = common.GetStrOr(info.HeaderFile, "../header.json")
}

func subCmdAdd() *cli.Command {
	cmd := &cli.Command{
		Name:  "add",
		Usage: "add book entry to library.json",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "author",
				Aliases:  []string{"a"},
				Usage:    "book author",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
			&cli.StringFlag{
				Name:  "local",
				Usage: "mark book as local book, available types are: " + strings.Join(book_mgr.AllLocalBookType, ", "),
			},
			&cli.StringFlag{
				Name:     "title",
				Aliases:  []string{"t"},
				Usage:    "book title",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "toc",
				Usage:    "TOC URL of the book",
				Required: true,
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

			toc := cmd.String("toc")
			if info.Books != nil {
				for i, book := range info.Books {
					if book.TocURL == toc {
						return fmt.Errorf("a book with the same TOC URL already exists a index %d", i)
					}
				}
			}

			book := book_mgr.BookInfo{
				Title:  cmd.String("title"),
				Author: cmd.String("author"),

				TocURL: toc,
			}

			localType := cmd.String("local")
			if localType != "" {
				book.LocalInfo = &book_mgr.LocalInfo{
					Type: localType,
				}
			}

			info.Books = append(info.Books, book)

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
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
			&cli.StringFlag{
				Name:  "local",
				Usage: "mark book as local book, available types are: " + strings.Join(book_mgr.AllLocalBookType, ", "),
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

			book := book_mgr.BookInfo{}

			localType := cmd.String("local")
			if localType != "" {
				book.LocalInfo = &book_mgr.LocalInfo{
					Type: localType,
				}
			}

			info.Books = append(info.Books, book)

			return info.SaveFile(filePath)
		},
	}

	return cmd
}

func subCmdList() *cli.Command {
	var libIndex int64

	cmd := &cli.Command{
		Name:  "list",
		Usage: "print books in library",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "print complete information of books",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "print information in JSON format",
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
			filePath := cmd.String("library")
			info, err := book_mgr.ReadLibraryInfo(filePath)
			if err != nil {
				return err
			}

			books := info.Books
			if libIndex >= 0 {
				books = books[libIndex : libIndex+1]
			}

			switch {
			case cmd.Bool("json"):
				printBooksJSON(books)
			case cmd.Bool("verbose"):
				printBooksVerbose(books)
			default:
				printBooksSimple(books)
			}

			return nil
		},
	}

	return cmd
}

func printBooksSimple(books []book_mgr.BookInfo) {
	for index, book := range books {
		fmt.Printf("%d. %s\n", index, common.GetStrOr(book.Title, "no-title"))
	}
}

func printBooksVerbose(books []book_mgr.BookInfo) {
	for index, book := range books {
		fmt.Printf("%d. %s\n", index, common.GetStrOr(book.Title, "no-title"))
		fmt.Println("  author:", book.Author)
		fmt.Println("  TOC   :", book.Author)
		fmt.Println("  root        :", book.RootDir)
		fmt.Println("  raw output  :", book.RawDir)
		fmt.Println("  text output :", book.TextDir)
		fmt.Println("  image output:", book.ImgDir)
		fmt.Println("  header      :", book.HeaderFile)
	}
}

func printBooksJSON(books []book_mgr.BookInfo) {
	data, _ := json.MarshalIndent(books, "", "    ")
	fmt.Println(string(data))
}

func subCmdListVolume() *cli.Command {
	var bookIndex int64

	cmd := &cli.Command{
		Name:  "list-volume",
		Usage: "list volumes of a book",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
		},
		Arguments: []cli.Argument{
			&cli.IntArg{
				Name:        "book-index",
				UsageText:   "<index>",
				Destination: &bookIndex,
				Value:       -1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("library")
			info, err := book_mgr.ReadLibraryInfo(filePath)
			if err != nil {
				return err
			}

			if bookIndex < 0 || int(bookIndex) >= len(info.Books) {
				return fmt.Errorf("index out of range")
			}

			book := info.Books[bookIndex]

			fmt.Println("title:", book.Title)

			var entryList []fs.DirEntry

			if book.LocalInfo == nil {
				entryList, err = os.ReadDir(book.RawDir)
			} else {
				switch book.LocalInfo.Type {
				case book_mgr.LocalBookTypeEpub:
					entryList, err = os.ReadDir(book.EpubDir)
				case book_mgr.LocalBookTypeImage:
					entryList, err = os.ReadDir(book.ImgDir)
				case book_mgr.LocalBookTypeLatex:
					entryList, err = os.ReadDir(book.LatexDir)
				case book_mgr.LocalBookTypeHTML:
					entryList, err = os.ReadDir(book.TextDir)
				case book_mgr.LocalBookTypeZip:
					entryList, err = os.ReadDir(book.ZipDir)
				default:
					return fmt.Errorf("unknown local book type %q", book.LocalInfo.Type)
				}
			}

			if err != nil {
				return fmt.Errorf("failed to read volume directory of book %s: %s", book.Title, err)
			}

			for index, entry := range entryList {
				fmt.Printf("%d. %s\n", index, entry.Name())
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
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
			&cli.StringFlag{
				Name:    "locale",
				Aliases: []string{"l"},
				Usage:   "IETF BCP 47 language tag to be used as sorting language",
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
