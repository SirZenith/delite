package tag

import (
	"context"
	"encoding/json"
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
		Name:  "tag",
		Usage: "operation for manipulating book list in library",
		Commands: []*cli.Command{
			subCmdAdd(),
			subCmdAddEmpty(),
			subCmdList(),
			subCmdSort(),
		},
	}
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
				for i, info := range info.TaggedPosts {
					if info.Tag == tag {
						return fmt.Errorf("a info with the tag already exists at index %d", i)
					}
				}
			}

			tag := book_mgr.TaggedPostInfo{
				Title:   title,
				Tag:     tag,
				PageCnt: int(page),
			}

			info.TaggedPosts = append(info.TaggedPosts, tag)

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

			tag := book_mgr.TaggedPostInfo{}

			info.TaggedPosts = append(info.TaggedPosts, tag)

			return info.SaveFile(filePath)
		},
	}
}

func subCmdList() *cli.Command {
	var libIndex int64

	return &cli.Command{
		Name:  "list",
		Usage: "print tag post info in library",
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

			tags := info.TaggedPosts
			if libIndex >= 0 {
				tags = tags[libIndex : libIndex+1]
			}

			switch {
			case cmd.Bool("json"):
				printBooksJSON(tags)
			case cmd.Bool("verbose"):
				printBooksVerbose(tags)
			default:
				printBooksSimple(tags)
			}

			return nil
		},
	}
}

func printBooksSimple(tags []book_mgr.TaggedPostInfo) {
	for index, tag := range tags {
		fmt.Printf("%d. %s\n", index, common.GetStrOr(tag.Title, "no-title"))
	}
}

func printBooksVerbose(tags []book_mgr.TaggedPostInfo) {
	for index, tag := range tags {
		fmt.Printf("%d. %s\n", index, common.GetStrOr(tag.Title, "no-title"))
		fmt.Println("  tag   :", tag.Tag)
		fmt.Println("  page  :", tag.PageCnt)
	}
}

func printBooksJSON(tags []book_mgr.TaggedPostInfo) {
	data, _ := json.MarshalIndent(tags, "", "    ")
	fmt.Println(string(data))
}

type TagList []book_mgr.TaggedPostInfo

func (b TagList) Len() int {
	return len(b)
}

func (b TagList) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b TagList) Bytes(i int) []byte {
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
			list.Sort(TagList(info.TaggedPosts))

			return info.SaveFile(filePath)
		},
	}

	return cmd
}
