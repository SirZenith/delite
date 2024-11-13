package database

import (
	"context"
	"fmt"

	"github.com/SirZenith/delite/database"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	return &cli.Command{
		Name:  "database",
		Usage: "database management utility",
		Commands: []*cli.Command{
			subCmdExport(),
			subCmdImport(),
			subCmdMigrate(),
		},
	}
}

func subCmdExport() *cli.Command {
	var dbPath string
	var tableName string
	var csvFilePath string

	return &cli.Command{
		Name:  "export",
		Usage: "export data as CSV",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "dbpath",
				UsageText:   "<db>",
				Destination: &dbPath,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "table-name",
				UsageText:   " <table>",
				Destination: &tableName,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "csv-file",
				UsageText:   " <csv>",
				Destination: &csvFilePath,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			model := database.GetModel(tableName)
			if model == nil {
				return fmt.Errorf("invald table name %q", tableName)
			}

			db, err := database.Open(dbPath)
			if err != nil {
				return err
			}

			err = database.ExportCSV(db, model, csvFilePath)
			if err != nil {
				return fmt.Errorf("failed to export table %s: %s", tableName, err)
			}

			return nil
		},
	}
}

func subCmdImport() *cli.Command {
	var dbPath string
	var csvFilePath string
	var tableName string

	return &cli.Command{
		Name:  "import",
		Usage: "import data from CSV",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "dbpath",
				UsageText:   "<db>",
				Destination: &dbPath,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "csv-file",
				UsageText:   " <csv>",
				Destination: &csvFilePath,
				Min:         1,
				Max:         1,
			},
			&cli.StringArg{
				Name:        "table-name",
				UsageText:   " <table-name>",
				Destination: &tableName,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			model := database.GetModel(tableName)
			if model == nil {
				return fmt.Errorf("invald table name %q", tableName)
			}

			db, err := database.Open(dbPath)
			if err != nil {
				return err
			}

			err = database.ImportCSV(db, model, csvFilePath)
			if err != nil {
				return fmt.Errorf("failed to export table %s: %s", tableName, err)
			}

			return nil
		},
	}
}

func subCmdMigrate() *cli.Command {
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
