package init_info

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/SirZenith/bilinovel/base"
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
		log.Printf("failed to read existing info file: %s, go on processing any way\n", err)
	}

	updateDefaultValue(&info)

	return saveInfoFile(&info, outputName)
}

// Read book info form existing info.json
func readExistingInfo(filename string) (base.BookInfo, error) {
	info := base.BookInfo{}

	data, err := os.ReadFile(filename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
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
	info.RawHTMLOutput = getStrOr(info.RawHTMLOutput, "./text_raw")
	info.HTMLOutput = getStrOr(info.HTMLOutput, "./text")
	info.ImgOutput = getStrOr(info.ImgOutput, "./image")
	info.EpubOutput = getStrOr(info.EpubOutput, "./epub")

	info.HeaderFile = getStrOr(info.HeaderFile, "../header.json")
	info.NameMapFile = getStrOr(info.NameMapFile, "./name_map.json")
}

// If given `value` is not empty, returns it. Else `defaultValue` will be returned.
func getStrOr(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	} else {
		return value
	}
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
