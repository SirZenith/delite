package manga_dl

import (
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:     "manga",
		Usage:    "download manga from website",
		Commands: []*cli.Command{},
	}

	return cmd
}
