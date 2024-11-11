package database

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"os"

	"github.com/SirZenith/delite/database"
	"github.com/urfave/cli/v3"
)

func Cmd() *cli.Command {
	return &cli.Command{
		Name:  "database",
		Usage: "database management utility",
		Commands: []*cli.Command{
			subcmdExport(),
			// subcmdImport(),
			subcmdMigrate(),
		},
	}
}

func subcmdExport() *cli.Command {
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

			rows, err := db.Model(model).Rows()
			if err != nil {
				return fmt.Errorf("failed to make query to table %s: %s", tableName, err)
			}

			return database.SaveAsCSV(rows, csvFilePath)
		},
	}
}

func subcmdImport() *cli.Command {
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
			file, err := os.Open(csvFilePath)
			if err != nil {
				return fmt.Errorf("failed to open CSV file %s: %s", csvFilePath, err)
			}
			defer file.Close()

			bufReader := bufio.NewReader(file)

			csvReader := csv.NewReader(bufReader)
			csvReader.Read()

			/* db, err := database.Open(dbPath)
			if err != nil {
				return err
			} */

			return nil
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
