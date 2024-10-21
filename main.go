package main

import (
	"context"
	"os"

	"github.com/SirZenith/bilinovel/book_dl"
	"github.com/SirZenith/bilinovel/font_descramble"
	"github.com/SirZenith/bilinovel/init_info"
	"github.com/SirZenith/bilinovel/make_epub"
	"github.com/SirZenith/bilinovel/page_decypher"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:                  "bilinovel",
		Usage:                 "helper program for downloading novels from www.bilinovel.com (mobile) or www.linovelib.com (desktop)",
		Version:               "0.3.0",
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			book_dl.Cmd(),
			font_descramble.Cmd(),
			init_info.Cmd(),
			make_epub.Cmd(),
			page_decypher.Cmd(),
		},
	}

	err := cmd.Run(context.Background(), os.Args)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
