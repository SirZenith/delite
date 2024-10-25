package manga_dl

import (
	"github.com/SirZenith/delite/cmd/manga_dl/internal/gelbooru"
	"github.com/SirZenith/delite/cmd/manga_dl/internal/nhentai"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	cmd := &cli.Command{
		Name:  "manga",
		Usage: "download manga from website",
		Commands: []*cli.Command{
			gelbooru.Cmd(),
			nhentai.Cmd(),
		},
	}

	return cmd
}
