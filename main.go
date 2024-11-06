package main

import (
	"context"
	"os"
	"time"

	"github.com/SirZenith/delite/cmd/book_dl"
	"github.com/SirZenith/delite/cmd/bundle"
	"github.com/SirZenith/delite/cmd/convert"
	"github.com/SirZenith/delite/cmd/font_descramble"
	"github.com/SirZenith/delite/cmd/gelbooru"
	"github.com/SirZenith/delite/cmd/illust"
	"github.com/SirZenith/delite/cmd/library"
	"github.com/SirZenith/delite/cmd/nhentai"
	"github.com/SirZenith/delite/cmd/page_decypher"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
)

func main() {
	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.TimeOnly,
	})
	log.SetDefault(logger)

	cmd := &cli.Command{
		Name:                  "delite",
		Usage:                 "scraper program for downloading books and images from various website",
		Version:               "0.3.1",
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			book_dl.Cmd(),
			bundle.Cmd(),
			convert.Cmd(),
			gelbooru.Cmd(),
			font_descramble.Cmd(),
			illust.Cmd(),
			library.Cmd(),
			nhentai.Cmd(),
			page_decypher.Cmd(),
		},
	}

	err := cmd.Run(context.Background(), os.Args)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
