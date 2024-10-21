package init_info

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
func readExistingInfo(filename string) (base.BookInfo, error) {
	info := base.BookInfo{}

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
func updateDefaultValue(info *base.BookInfo) {
	info.RawHTMLOutput = base.GetStrOr(info.RawHTMLOutput, "./text_raw")
	info.HTMLOutput = base.GetStrOr(info.HTMLOutput, "./text")
	info.ImgOutput = base.GetStrOr(info.ImgOutput, "./image")
	info.EpubOutput = base.GetStrOr(info.EpubOutput, "./epub")

	info.HeaderFile = base.GetStrOr(info.HeaderFile, "../header.json")
	info.NameMapFile = base.GetStrOr(info.NameMapFile, "./name_map.json")
}

// Save book info struct to file.
func saveInfoFile(info *base.BookInfo, filename string) error {
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
