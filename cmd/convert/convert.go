package convert

import (
	"github.com/SirZenith/delite/cmd/convert/internal/epub2html"
	"github.com/SirZenith/delite/cmd/convert/internal/epub_by_script"
	"github.com/SirZenith/delite/cmd/convert/internal/html_by_script"
	"github.com/SirZenith/delite/cmd/convert/internal/image2image"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "convert",
		Usage: "convert bundled books into other formats",
		Commands: []*cli.Command{
			epub_by_script.Cmd(),
			epub2html.Cmd(),
			html_by_script.Cmd(),
			image2image.Cmd(),
		},
	}

	return cmd
}
