package database

import (
	"context"

	"github.com/SirZenith/delite/database"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	return &cli.Command{
		Name:  "database",
		Usage: "database management utility",
		Commands: []*cli.Command{
			subcmdMigrate(),
		},
	}
}

func subcmdMigrate() *cli.Command {
	var dbPath string

	return &cli.Command{
		Name:  "migrate",
		Usage: "auto migrate database schema",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "dbpath",
				UsageText:   "<path>",
				Destination: &dbPath,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			db, err := database.Open(dbPath)
			if err != nil {
				return err
			}

			return database.Migrate(db)
		},
	}
}
