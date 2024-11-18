package convert

import (
	"github.com/SirZenith/delite/cmd/convert/epub2html"
	"github.com/SirZenith/delite/cmd/convert/epub2latex"
	"github.com/SirZenith/delite/cmd/convert/image2image"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "convert",
		Usage: "convert bundled books into other formats",
		Commands: []*cli.Command{
			epub2html.Cmd(),
			epub2latex.Cmd(),
			image2image.Cmd(),
		},
	}

	return cmd
}
