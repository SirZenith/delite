package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	book_mgr "github.com/SirZenith/delite/book_management"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "config",
		Usage: "operation for manipulating library config",
		Commands: []*cli.Command{
			subCmdInit(),
		},
	}

	return cmd
}

func subCmdInit() *cli.Command {
	var dir string

	cmd := &cli.Command{
		Name: "init",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "directory",
				Destination: &dir,
				Value:       "./",
				Max:         1,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			outputName := filepath.Join(dir, "config.json")

			config := book_mgr.Config{}
			if _, err := os.Stat(outputName); err == nil {
				config, err = book_mgr.ReadConfigFile(outputName)
				if err != nil {
					log.Warnf("failed to read existing config file: %s, continue anyway", err)
				}
			}

			if config.JobCount <= 0 {
				config.JobCount = runtime.NumCPU()
			}
			if config.RetryCount <= 0 {
				config.RetryCount = 3
			}
			if config.TimeOut <= 0 {
				config.TimeOut = 30 * time.Second
			}

			config.OutputDir = "./"
			config.TargetList = "./dl.txt"
			config.HeaderFile = "./header.json"

			data, err := json.MarshalIndent(config, "", "    ")
			if err != nil {
				return fmt.Errorf("failed to convert data to JSON: %s", err)
			}

			err = os.WriteFile(outputName, data, 0o644)
			if err != nil {
				return fmt.Errorf("failed to write config output %s: %s", outputName, err)
			}

			return nil
		},
	}

	return cmd
}
