package init_library

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
		Name:  "init-library",
		Usage: "create library.json file under given directory",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "path",
				UsageText:   "<directory>",
				Value:       "./",
				Destination: &dir,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return cmdMain(dir)
		},
	}

	return cmd
}

func cmdMain(dir string) error {
	outputName := filepath.Join(dir, "library.json")

	info, err := readExistingInfo(outputName)
	if err != nil {
		log.Infof("failed to read existing info file: %s, go on processing any way", err)
	}

	updateDefaultValue(&info)

	return saveInfoFile(&info, outputName)
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
