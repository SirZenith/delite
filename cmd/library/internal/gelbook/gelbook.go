package gelbook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/jeandeaual/go-locale"
	"github.com/urfave/cli/v3"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func Cmd() *cli.Command {
	return &cli.Command{
		Name:  "gelbook",
		Usage: "operation for manipulating book list in library",
		Commands: []*cli.Command{
			subCmdAdd(),
			subCmdAddEmpty(),
			subCmdSort(),
		},
	}
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
	var title string
	var tag string
	var page int64

	return &cli.Command{
		Name:  "add",
		Usage: "add gelbooru book entry to library.json",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "library",
				Usage: "path of library.json file to be modified",
				Value: "./library.json",
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
				Name:        "tag",
				UsageText:   " <tag>",
				Destination: &tag,
				Min:         1,
				Max:         1,
			},
			&cli.IntArg{
				Name:        "page",
				UsageText:   " <page-cnt>",
				Destination: &page,
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

			if info.TaggedPosts != nil {
				for i, book := range info.TaggedPosts {
					if book.Tag == tag {
						return fmt.Errorf("a book with the same TOC URL already exists a index %d", i)
					}
				}
			}

			book := book_mgr.GelbooruBookInfo{
				Title:   title,
				Tag:     tag,
				PageCnt: int(page),
			}

			info.TaggedPosts = append(info.TaggedPosts, book)

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

			book := book_mgr.GelbooruBookInfo{}

			info.TaggedPosts = append(info.TaggedPosts, book)

			return info.SaveFile(filePath)
		},
	}
}

type BookList []book_mgr.GelbooruBookInfo

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
			list.Sort(BookList(info.TaggedPosts))

			return info.SaveFile(filePath)
		},
	}

	return cmd
}
