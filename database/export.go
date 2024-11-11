package database

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"time"
)

type CsvConverter struct {
	Headers         []string
	WriteHeaders    bool
	TimeFormat      string
	FloatFormat     string
	Delimiter       rune
	rows            *sql.Rows
	rowPreProcessor CsvPreProcessorFunc
}

func New(rows *sql.Rows) *CsvConverter {
	return &CsvConverter{
		rows:         rows,
		WriteHeaders: true,
		Delimiter:    ',',
	}
}

type CsvPreProcessorFunc func(row []string, columnNames []string) (outputRow bool, processedRow []string)

func (c *CsvConverter) SetRowPreProcessor(processor CsvPreProcessorFunc) {
	c.rowPreProcessor = processor
}

func (c CsvConverter) WriteFile(csvFileName string) error {
	f, err := os.Create(csvFileName)
	if err != nil {
		return err
	}

	err = c.Write(f)
	if err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

func (c CsvConverter) Write(writer io.Writer) error {
	rows := c.rows

	csvWriter := csv.NewWriter(writer)

	if c.Delimiter != '\x00' {
		csvWriter.Comma = c.Delimiter
	}

	columnNames, err := rows.Columns()
	if err != nil {
		return err
	}

	if c.WriteHeaders {
		var headers []string
		if len(c.Headers) > 0 {
			headers = c.Headers
		} else {
			headers = columnNames
		}

		err = csvWriter.Write(headers)
		if err != nil {
			return fmt.Errorf("failed to write headers: %w", err)
		}
	}

	count := len(columnNames)
	values := make([]interface{}, count)
	valuePtrs := make([]interface{}, count)

	for rows.Next() {
		row := make([]string, count)

		for i, _ := range columnNames {
			valuePtrs[i] = &values[i]
		}

		if err = rows.Scan(valuePtrs...); err != nil {
			return err
		}

		for i, _ := range columnNames {
			var value interface{}
			rawValue := values[i]

			byteArray, ok := rawValue.([]byte)
			if ok {
				value = string(byteArray)
			} else {
				value = rawValue
			}

			float64Value, ok := value.(float64)
			if ok && c.FloatFormat != "" {
				value = fmt.Sprintf(c.FloatFormat, float64Value)
			} else {
				float32Value, ok := value.(float32)
				if ok && c.FloatFormat != "" {
					value = fmt.Sprintf(c.FloatFormat, float32Value)
				}
			}

			timeValue, ok := value.(time.Time)
			if ok && c.TimeFormat != "" {
				value = timeValue.Format(c.TimeFormat)
			}

			if value == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", value)
			}
		}

		writeRow := true
		if c.rowPreProcessor != nil {
			writeRow, row = c.rowPreProcessor(row, columnNames)
		}

		if writeRow {
			err = csvWriter.Write(row)
			if err != nil {
				return fmt.Errorf("failed to write data row to csv %w", err)
			}
		}
	}
	err = rows.Err()
	csvWriter.Flush()

	return err
}

func SaveAsCSV(rows *sql.Rows, fileName string) error {
	return New(rows).WriteFile(fileName)
}
