package init_info

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
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	var dir string

	cmd := &cli.Command{
		Name:  "init-info",
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
			return cmdMain(dir)
		},
	}

	return cmd
}

func cmdMain(dir string) error {
	outputName := filepath.Join(dir, "info.json")

	info, err := readExistingInfo(outputName)
	if err != nil {
		log.Infof("failed to read existing info file: %s, go on processing any way", err)
	}

	updateDefaultValue(&info)

	return saveInfoFile(&info, outputName)
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

	info.HeaderFile = common.GetStrOr(info.HeaderFile, "../header.json")
	info.NameMapFile = common.GetStrOr(info.NameMapFile, "./name_map.json")
}

// Save book info struct to file.
func saveInfoFile(info *book_mgr.BookInfo, filename string) error {
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
