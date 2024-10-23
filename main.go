package main

import (
	"context"
	"os"
	"time"

	"github.com/SirZenith/litnovel-dl/book_dl"
	"github.com/SirZenith/litnovel-dl/font_descramble"
	"github.com/SirZenith/litnovel-dl/init_info"
	"github.com/SirZenith/litnovel-dl/library"
	"github.com/SirZenith/litnovel-dl/make_epub"
	"github.com/SirZenith/litnovel-dl/page_decypher"
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
		Name:                  "litnovel-dl",
		Usage:                 "helper program for downloading novels from www.bilinovel.com (mobile) or www.linovelib.com (desktop)",
		Version:               "0.3.0",
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			book_dl.Cmd(),
			font_descramble.Cmd(),
			init_info.Cmd(),
			library.Cmd(),
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
