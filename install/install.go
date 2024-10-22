package install

import (
	"context"

	"github.com/playwright-community/playwright-go"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "install",
		Usage: "install playwright drivers.",
		Action: func(_ context.Context, _ *cli.Command) error {
			return playwright.Install()
		},
	}

	return cmd
}
