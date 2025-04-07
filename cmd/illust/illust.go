package illust

import (
	"github.com/SirZenith/delite/cmd/illust/internal/download"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "illust",
		Usage: "dealing with illustration in novels",
		Commands: []*cli.Command{
			download.Cmd(),
		},
	}

	return cmd
}
