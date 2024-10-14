package main

import (
	"context"
	"fmt"
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

	err := cmd.Run(context.Background(), os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
