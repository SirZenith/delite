package bundle

import (
	"github.com/SirZenith/delite/cmd/bundle/epub"
	"github.com/SirZenith/delite/cmd/bundle/latex"
	"github.com/SirZenith/delite/cmd/bundle/pdf"
	"github.com/SirZenith/delite/cmd/bundle/zip"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "bundle",
		Usage: "packing downloaded resources into a single file",
		Commands: []*cli.Command{
			epub.Cmd(),
			latex.Cmd(),
			pdf.Cmd(),
			zip.Cmd(),
		},
	}

	return cmd
}
