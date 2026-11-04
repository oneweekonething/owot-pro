// Package excelx writes XLSX exports without leaking workbook dependencies into app.
package excelx

import (
	"context"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"
)

type DictTranslator interface {
	Label(context.Context, string, string) (string, error)
	Value(context.Context, string, string) (string, error)
}

// Write streams already-authorized values. Row values use SetRow, so strings that
// begin with '=' are stored as text rather than executable spreadsheet formulas.
func Write(ctx context.Context, writer io.Writer, headers []string, rows [][]any) error {
	book := excelize.NewFile()
	defer func() { _ = book.Close() }()
	stream, err := book.NewStreamWriter("Sheet1")
	if err != nil {
		return err
	}
	header := make([]any, len(headers))
	for i, h := range headers {
		header[i] = h
	}
	if err := stream.SetRow("A1", header); err != nil {
		return err
	}
	for i, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := stream.SetRow(fmt.Sprintf("A%d", i+2), row); err != nil {
			return err
		}
	}
	if err := stream.Flush(); err != nil {
		return err
	}
	return book.Write(writer)
}
