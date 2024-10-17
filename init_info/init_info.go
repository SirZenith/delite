package init_info

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirZenith/bilinovel/base"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	var dir string

	cmd := &cli.Command{
		Name: "init-info",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "directory",
				UsageText:   "<path>",
				Destination: &dir,
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
	info := base.BookInfo{
		Title:  "",
		Author: "",

		TocURL:        "",
		RawHTMLOutput: "./text_raw",
		HTMLOutput:    "./text",
		ImgOutput:     "./image",
		EpubOutput:    "./epub",
	}
	data, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		return fmt.Errorf("JSON conversion failed: %s", err)
	}

	outputName := filepath.Join(dir, "info.json")
	err = os.WriteFile(outputName, data, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write info file: %s", err)
	}

	return nil
}
