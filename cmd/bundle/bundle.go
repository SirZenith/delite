package bundle

import (
	"github.com/SirZenith/delite/cmd/bundle/manga_pdf"
	"github.com/SirZenith/delite/cmd/bundle/novel_epub"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "bundle",
		Usage: "packing downloaded resources into a single file",
		Commands: []*cli.Command{
			manga_pdf.Cmd(),
			novel_epub.Cmd(),
		},
	}

	return cmd
}
