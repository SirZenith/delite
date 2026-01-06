package book

import (
	"context"
	"encoding/json"
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
			subCmdLatexPreprocess(),
			subCmdList(),
			subCmdListVolume(),
			subCmdMarkRead(),
			subCmdRate(),
			subCmdSort(),
		},
	}

	return cmd
}

func subCmdAdd() *cli.Command {
	var title string
	var author string
	var tocURL string

	return &cli.Command{
		Name:  "add",
		Usage: "add book entry to library.json",
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
			&cli.FloatFlag{
				Name:  "rating",
				Value: -1,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "title",
				UsageText:   "<title>",
				Destination: &title,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "author",
				UsageText:   " <author>",
				Destination: &author,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "toc-url",
				UsageText:   " <toc-url>",
				Destination: &tocURL,
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

			if info.Books != nil && tocURL != "" {
				for i, book := range info.Books {
					if book.TocURL == tocURL {
						return fmt.Errorf("a book with the same TOC URL already exists at index %d", i)
					}
				}
			}

			book := book_mgr.BookInfo{
				Title:  title,
				Author: author,

				TocURL: tocURL,

				Meta: &book_mgr.BookMeta{
					Rating: cmd.Float("rating"),
				},
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
}

func subCmdAddEmpty() *cli.Command {
	return &cli.Command{
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
}

func subCmdLatexPreprocess() *cli.Command {
	var rawKeyword string

	return &cli.Command{
		Name:  "latex-preprocess",
		Usage: "add preprocess script info to book",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "book-keyword",
				UsageText:   "<book>",
				Destination: &rawKeyword,
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

			keyword := book_mgr.NewSearchKeyword(rawKeyword)

			for i := range info.Books {
				book := &info.Books[i]

				if !keyword.MatchBook(i, *book) {
					continue
				}

				meta := book.LatexInfo
				if meta == nil {
					meta = new(book_mgr.LatexBookInfo)
					book.LatexInfo = meta
				}
			}

			return info.SaveFile(filePath)
		},
	}
}

func subCmdList() *cli.Command {
	var rawKeyword string

	return &cli.Command{
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
			&cli.StringArg{
				Name:        "book-keyword",
				UsageText:   "<book>",
				Destination: &rawKeyword,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("library")
			info, err := book_mgr.ReadLibraryInfo(filePath)
			if err != nil {
				return err
			}

			keyword := book_mgr.NewSearchKeyword(rawKeyword)
			books := info.Books

			switch {
			case cmd.Bool("json"):
				printBooksJSON(books, keyword)
			case cmd.Bool("verbose"):
				printBooksVerbose(books, keyword)
			default:
				printBooksSimple(books, keyword)
			}

			return nil
		},
	}
}

func printBooksSimple(books []book_mgr.BookInfo, keyword *book_mgr.SearchKeyword) {
	for index, book := range books {
		if !keyword.MatchBook(index, book) {
			continue
		}

		fmt.Printf("%d. %s\n", index, common.GetStrOr(book.Title, "no-title"))
	}
}

func printBooksVerbose(books []book_mgr.BookInfo, keyword *book_mgr.SearchKeyword) {
	for index, book := range books {
		if !keyword.MatchBook(index, book) {
			continue
		}

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

func printBooksJSON(books []book_mgr.BookInfo, keyword *book_mgr.SearchKeyword) {
	filtered := make([]book_mgr.BookInfo, 0, len(books))
	for i, book := range books {
		if keyword.MatchBook(i, book) {
			filtered = append(filtered, book)
		}
	}

	data, _ := json.MarshalIndent(filtered, "", "    ")
	fmt.Println(string(data))
}

func subCmdListVolume() *cli.Command {
	var rawKeyword string

	return &cli.Command{
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
			&cli.StringArg{
				Name:        "book-keyword",
				UsageText:   "<book>",
				Destination: &rawKeyword,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath := cmd.String("library")
			info, err := book_mgr.ReadLibraryInfo(filePath)
			if err != nil {
				return err
			}

			keyword := book_mgr.NewSearchKeyword(rawKeyword)

			for i, book := range info.Books {
				if !keyword.MatchBook(i, book) {
					continue
				}

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
					case book_mgr.LocalBookTypePdf:
						entryList, err = os.ReadDir(book.PdfDir)
					case book_mgr.LocalBookTypeHTML:
						entryList, err = os.ReadDir(book.TextDir)
					case book_mgr.LocalBookTypeZip:
						entryList, err = os.ReadDir(book.ZipDir)
					default:
						err = fmt.Errorf("unknown local book type %q", book.LocalInfo.Type)
					}
				}

				if err == nil {
					for index, entry := range entryList {
						fmt.Printf("%d. %s\n", index, entry.Name())
					}
				} else {
					log.Errorf("failed to read volume directory of book %s: %s", book.Title, err)
				}
			}

			return nil
		},
	}
}

func subCmdMarkRead() *cli.Command {
	var rawKeyword string
	var isRead int64

	return &cli.Command{
		Name:  "mark-read",
		Usage: "mark a book as completely read",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "book-keyword",
				UsageText:   "<book>",
				Destination: &rawKeyword,
				Min:         1,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "is-read",
				UsageText:   " <is-read>",
				Destination: &isRead,
				Max:         1,
				Value:       1,
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

			keyword := book_mgr.NewSearchKeyword(rawKeyword)

			for i := range info.Books {
				book := &info.Books[i]

				if !keyword.MatchBook(i, *book) {
					continue
				}

				book.IsRead = isRead != 0

				var mark string
				if book.IsRead {
					mark = "■"
				} else {
					mark = "▢"
				}
				fmt.Printf("%s %d. %s\n", mark, i, info.Books[i].Title)
			}

			err = info.SaveFile(filePath)
			if err != nil {
				return err
			}

			return nil
		},
	}
}

func subCmdRate() *cli.Command {
	var rawKeyword string
	var rating float64

	return &cli.Command{
		Name:  "rate",
		Usage: "set rating for a book",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "book-keyword",
				UsageText:   "<book>",
				Destination: &rawKeyword,
				Min:         1,
				Max:         1,
			},
			&cli.FloatArg{
				Name:        "rating",
				UsageText:   " <rating>",
				Destination: &rating,
				Max:         1,
				Value:       1,
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

			keyword := book_mgr.NewSearchKeyword(rawKeyword)

			for i := range info.Books {
				book := &info.Books[i]

				if !keyword.MatchBook(i, *book) {
					continue
				}

				if book.Meta == nil {
					book.Meta = new(book_mgr.BookMeta)
				}
				book.Meta.Rating = rating
				fmt.Printf("%d %s: %.2f\n", i, info.Books[i].Title, rating)
			}

			err = info.SaveFile(filePath)
			if err != nil {
				return err
			}

			return nil
		},
	}
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
	return &cli.Command{
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
}
