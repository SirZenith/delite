package illust

import (
	"github.com/SirZenith/delite/cmd/illust/download"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "illust",
		Usage: "delaing with illustration in novels",
		Commands: []*cli.Command{
			download.Cmd(),
		},
	}

	return cmd
}
