package main

import (
	"context"
	"os"

	"github.com/bilinovel/book_dl"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:    "bilinovel",
		Usage:   "helper program for downloading novels from www.bilinovel.com",
		Version: "0.1.0",
		Commands: []*cli.Command{
			book_dl.Cmd(),
		},
	}

	cmd.Run(context.Background(), os.Args)
}
