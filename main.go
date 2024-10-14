package main

import (
	"context"
	"os"

	cli "github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "bilinovel",
		Usage: "helper program for downloading novels from www.bilinovel.com",
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	cmd.Run(context.Background(), os.Args)
}
