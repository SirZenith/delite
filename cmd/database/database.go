package database

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/SirZenith/delite/database"
	"github.com/urfave/cli/v3"
	"gorm.io/gorm/clause"
)

func Cmd() *cli.Command {
	return &cli.Command{
		Name:  "database",
		Usage: "database management utility",
		Commands: []*cli.Command{
			subcmdExport(),
			subcmdImport(),
			subcmdMigrate(),
		},
	}
}

func getTypeHeader(rType reflect.Type, fieldName string, header []string) []string {
	switch rType.Kind() {
	case reflect.Ptr:
		header = getTypeHeader(rType.Elem(), fieldName, header)
	case reflect.Struct:
		fieldCnt := rType.NumField()

		for i := 0; i < fieldCnt; i++ {
			field := rType.Field(i)
			if !field.IsExported() {
				continue
			}

			var childFieldName string
			if fieldName == "" {
				childFieldName = field.Name
			} else {
				childFieldName = fieldName + "." + field.Name
			}
			header = getTypeHeader(field.Type, childFieldName, header)
		}
	default:
		header = append(header, fieldName)
	}

	return header
}

func getModelCsvLine(rValue reflect.Value, line []string) ([]string, error) {
	switch rValue.Kind() {
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64:
		line = append(line, strconv.FormatInt(rValue.Int(), 10))
	case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		line = append(line, strconv.FormatUint(rValue.Uint(), 10))
	case reflect.Float32:
		line = append(line, strconv.FormatFloat(rValue.Float(), 'f', 6, 32))
	case reflect.Float64:
		line = append(line, strconv.FormatFloat(rValue.Float(), 'f', 16, 64))
	case reflect.Bool:
		if rValue.Bool() {
			line = append(line, "1")
		} else {
			line = append(line, "0")
		}
	case reflect.String:
		line = append(line, rValue.String())
	case reflect.Func:
		// pass
	case reflect.Ptr:
		return getModelCsvLine(rValue.Elem(), line)
	case reflect.Struct:
		rType := rValue.Type()
		fieldCnt := rType.NumField()

		for i := 0; i < fieldCnt; i++ {
			field := rType.Field(i)

			if !field.IsExported() {
				continue
			}

			var err error
			line, err = getModelCsvLine(rValue.Field(i), line)
			if err != nil {
				return line, err
			}
		}
	case reflect.Interface:
		return getModelCsvLine(reflect.ValueOf(rValue.Interface()), line)
	default:
		return line, fmt.Errorf("unhandled type: %s", rValue.Kind())
	}

	return line, nil
}

func getFlattenedFieldList(rValue reflect.Value, fields []reflect.Value) []reflect.Value {
	switch rValue.Kind() {
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.Bool,
		reflect.String:

		fields = append(fields, rValue)
	case reflect.Func:
		// pass
	case reflect.Ptr:
		return getFlattenedFieldList(rValue.Elem(), fields)
	case reflect.Struct:
		rType := rValue.Type()
		fieldCnt := rType.NumField()

		for i := 0; i < fieldCnt; i++ {
			field := rType.Field(i)

			if !field.IsExported() {
				continue
			}

			fields = getFlattenedFieldList(rValue.Field(i), fields)
		}
	case reflect.Interface:
		return getFlattenedFieldList(reflect.ValueOf(rValue.Interface()), fields)
	default:
		return fields
	}

	return fields
}

func consumeCsvLine(rValue reflect.Value, line []string) ([]string, error) {
	if len(line) <= 0 {
		return line, nil
	}

	switch rValue.Kind() {
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(line[0], 10, 64)
		if err != nil {
			return line, err
		}
		rValue.SetInt(i)
		line = line[1:]
	case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(line[0], 10, 64)
		if err != nil {
			return line, err
		}
		rValue.SetUint(u)
		line = line[1:]
	case reflect.Float32:
		f, err := strconv.ParseFloat(line[0], 32)
		if err != nil {
			return line, err
		}
		line = line[1:]
		rValue.SetFloat(f)
	case reflect.Float64:
		f, err := strconv.ParseFloat(line[0], 64)
		if err != nil {
			return line, err
		}
		line = line[1:]
		rValue.SetFloat(f)
	case reflect.Bool:
		if line[0] == "0" {
			rValue.SetBool(false)
		} else {
			rValue.SetBool(true)
		}
		line = line[1:]
	case reflect.String:
		rValue.SetString(line[0])
		line = line[1:]
	case reflect.Func:
		// pass
	case reflect.Ptr:
		return consumeCsvLine(rValue.Elem(), line)
	case reflect.Struct:
		rType := rValue.Type()
		fieldCnt := rType.NumField()

		for i := 0; i < fieldCnt; i++ {
			field := rType.Field(i)

			if !field.IsExported() {
				continue
			}

			var err error
			line, err = consumeCsvLine(rValue.Field(i), line)
			if err != nil {
				return line, err
			}
		}
	case reflect.Interface:
		return consumeCsvLine(reflect.ValueOf(rValue.Interface()), line)
	default:
		return line, fmt.Errorf("unhandled type: %s", rValue.Kind())
	}

	return line, nil
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

			file, err := os.Create(csvFilePath)
			if err != nil {
				return fmt.Errorf("failed to open CSV file %s: %s", csvFilePath, err)
			}
			defer file.Close()

			csvWriter := csv.NewWriter(file)
			defer csvWriter.Flush()

			header := getTypeHeader(reflect.TypeOf(model), "", nil)
			err = csvWriter.Write(header)
			if err != nil {
				return fmt.Errorf("failed to write header %s: %s", csvFilePath, err)
			}

			db, err := database.Open(dbPath)
			if err != nil {
				return err
			}

			rows, err := db.Model(model).Rows()
			if err != nil {
				return fmt.Errorf("failed to make query to table %s: %s", tableName, err)
			}

			for rows.Next() {
				db.ScanRows(rows, &model)

				line, err := getModelCsvLine(reflect.ValueOf(&model), nil)
				if err != nil {
					return fmt.Errorf("failed to marshar database data: %s", err)
				}

				csvWriter.Write(line)
			}

			return nil
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
			model := database.GetModel(tableName)
			if model == nil {
				return fmt.Errorf("invald table name %q", tableName)
			}

			file, err := os.Open(csvFilePath)
			if err != nil {
				return fmt.Errorf("failed to open CSV file %s: %s", csvFilePath, err)
			}
			defer file.Close()

			csvReader := csv.NewReader(file)

			header, err := csvReader.Read()
			if err != nil {
				return fmt.Errorf("failed to read CSV header: %s", err)
			}

			typeHeader := getTypeHeader(reflect.TypeOf(model), "", nil)
			for i := 0; i < len(typeHeader); i++ {
				if typeHeader[i] != header[i] {
					return fmt.Errorf("field mismatch at header index #%d: want %q, get %q", i, typeHeader[i], header[i])
				}
			}

			db, err := database.Open(dbPath)
			if err != nil {
				return err
			}

			index := 2
			line, err := csvReader.Read()
			fields := getFlattenedFieldList(reflect.ValueOf(model), nil)
			for err == nil {
				for fieldIndex, field := range fields {
					line, err = consumeCsvLine(field, line)
					if err != nil {
						return fmt.Errorf("failed to unmarshal line %d field %d: %s", index, fieldIndex, err)
					}
				}

				db.Clauses(clause.OnConflict{DoNothing: true}).Create(model)

				line, err = csvReader.Read()
				index++
			}

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
