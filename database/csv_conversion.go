package database

import (
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// getModelFlattenedFieldList converts all nested fields of a value into a list of
// reflect.Value. Fields are listed in field index order.
func getModelFlattenedFieldList(rValue reflect.Value, fields []reflect.Value) []reflect.Value {
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
		return getModelFlattenedFieldList(rValue.Elem(), fields)
	case reflect.Struct:
		rType := rValue.Type()
		fieldCnt := rType.NumField()

		for i := 0; i < fieldCnt; i++ {
			field := rType.Field(i)

			if !field.IsExported() {
				continue
			}

			fields = getModelFlattenedFieldList(rValue.Field(i), fields)
		}
	case reflect.Interface:
		return getModelFlattenedFieldList(reflect.ValueOf(rValue.Interface()), fields)
	default:
		return fields
	}

	return fields
}

func getModelCSVHeader(rType reflect.Type, fieldName string, header []string) []string {
	switch rType.Kind() {
	case reflect.Ptr:
		header = getModelCSVHeader(rType.Elem(), fieldName, header)
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
			header = getModelCSVHeader(field.Type, childFieldName, header)
		}
	default:
		header = append(header, fieldName)
	}

	return header
}

func getModelCSVLine(rValue reflect.Value, line []string) ([]string, error) {
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
		return getModelCSVLine(rValue.Elem(), line)
	case reflect.Struct:
		rType := rValue.Type()
		fieldCnt := rType.NumField()

		for i := 0; i < fieldCnt; i++ {
			field := rType.Field(i)

			if !field.IsExported() {
				continue
			}

			var err error
			line, err = getModelCSVLine(rValue.Field(i), line)
			if err != nil {
				return line, err
			}
		}
	case reflect.Interface:
		return getModelCSVLine(reflect.ValueOf(rValue.Interface()), line)
	default:
		return line, fmt.Errorf("unhandled type: %s", rValue.Kind())
	}

	return line, nil
}

func consumeCSVModelData(rValue reflect.Value, line []string) ([]string, error) {
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
		return consumeCSVModelData(rValue.Elem(), line)
	case reflect.Struct:
		rType := rValue.Type()
		fieldCnt := rType.NumField()

		for i := 0; i < fieldCnt; i++ {
			field := rType.Field(i)

			if !field.IsExported() {
				continue
			}

			var err error
			line, err = consumeCSVModelData(rValue.Field(i), line)
			if err != nil {
				return line, err
			}
		}
	case reflect.Interface:
		return consumeCSVModelData(reflect.ValueOf(rValue.Interface()), line)
	default:
		return line, fmt.Errorf("unhandled type: %s", rValue.Kind())
	}

	return line, nil
}

func ExportCSV(db *gorm.DB, model any, csvFilePath string) error {
	file, err := os.Create(csvFilePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file %s: %s", csvFilePath, err)
	}
	defer file.Close()

	csvWriter := csv.NewWriter(file)
	defer csvWriter.Flush()

	header := getModelCSVHeader(reflect.TypeOf(model), "", nil)
	err = csvWriter.Write(header)
	if err != nil {
		return fmt.Errorf("failed to write header %s: %s", csvFilePath, err)
	}

	rows, err := db.Model(model).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	fields := getModelFlattenedFieldList(reflect.ValueOf(model), nil)
	line := make([]string, 0, len(fields))
	for rows.Next() {
		db.ScanRows(rows, &model)

		line = line[:0]
		for _, field := range fields {
			line, err = getModelCSVLine(field, line)
			if err != nil {
				return fmt.Errorf("failed to marshar database data: %s", err)
			}
		}

		csvWriter.Write(line)
	}

	return nil
}

func ImportCSV(db *gorm.DB, model any, csvFilePath string) error {
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

	typeHeader := getModelCSVHeader(reflect.TypeOf(model), "", nil)
	modelFields := getModelFlattenedFieldList(reflect.ValueOf(model), nil)
	fieldMap := map[string]reflect.Value{}
	for i, name := range typeHeader {
		fieldMap[name] = modelFields[i]
	}

	targetFields := make([]reflect.Value, 0, len(header))
	for _, name := range header {
		field, ok := fieldMap[name]
		if !ok {
			return fmt.Errorf("invalid field name %s", name)
		}

		targetFields = append(targetFields, field)
	}

	index := 2
	line, err := csvReader.Read()
	for err == nil {
		for fieldIndex, field := range targetFields {
			line, err = consumeCSVModelData(field, line)
			if err != nil {
				return fmt.Errorf("failed to unmarshal line %d field %d: %s", index, fieldIndex, err)
			}
		}

		db.Clauses(clause.OnConflict{UpdateAll: true}).Create(model)

		line, err = csvReader.Read()
		index++
	}

	return nil
}
