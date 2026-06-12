package ui

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Cell is one rendered table value plus an optional semantic style applied when color is enabled.
type Cell struct {
	Text  string
	Style Style
}

// Table is a tabular view: column headers and rows of cells aligned to those headers. It drives the
// table and csv formats; JSON is rendered from a separate, CLI-shaped view value.
type Table struct {
	Headers []string
	Rows    [][]Cell
	// Note is an optional human summary printed after the table (table format only, e.g. "12 shown").
	Note string
}

// Row is a small helper to build a row from plain strings (no styling).
func Row(values ...string) []Cell {
	cells := make([]Cell, len(values))
	for i, v := range values {
		cells[i] = Cell{Text: v}
	}
	return cells
}

// Render writes data in the environment's selected format. table supplies headers/rows for the
// table and csv formats; view is the value marshalled for json (a CLI-shaped struct, never the raw
// SDK types, so the JSON contract stays stable and free of PascalCase noise).
func (e *Env) Render(table Table, view any) error {
	switch e.Format {
	case FormatJSON:
		enc := json.NewEncoder(e.out())
		enc.SetIndent("", "  ")
		return enc.Encode(view)
	case FormatCSV:
		return e.renderCSV(table)
	default:
		return e.renderTable(table)
	}
}

// renderTable lays out columns by hand (rather than text/tabwriter) so ANSI color codes, which would
// otherwise inflate the measured width, never break alignment: widths are computed from plain text
// and color is applied only to the visible content, with padding added afterwards.
func (e *Env) renderTable(t Table) error {
	w := e.out()
	cols := len(t.Headers)
	widths := make([]int, cols)
	for i, h := range t.Headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range t.Rows {
		for i := 0; i < cols && i < len(row); i++ {
			if n := utf8.RuneCountInString(row[i].Text); n > widths[i] {
				widths[i] = n
			}
		}
	}

	const gap = 2
	var sb strings.Builder
	writeRow := func(cells func(i int) (string, Style)) {
		sb.Reset()
		for i := 0; i < cols; i++ {
			text, style := cells(i)
			sb.WriteString(e.Color(text, style))
			if i < cols-1 {
				sb.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(text)+gap))
			}
		}
		fmt.Fprintln(w, strings.TrimRight(sb.String(), " "))
	}

	writeRow(func(i int) (string, Style) { return t.Headers[i], StyleHeader })
	for _, row := range t.Rows {
		writeRow(func(i int) (string, Style) {
			if i < len(row) {
				return row[i].Text, row[i].Style
			}
			return "", StyleNone
		})
	}
	if t.Note != "" {
		fmt.Fprintf(w, "\n%s\n", e.Color(t.Note, StyleMuted))
	}
	return nil
}

func (e *Env) renderCSV(t Table) error {
	cw := csv.NewWriter(e.out())
	if err := cw.Write(t.Headers); err != nil {
		return err
	}
	for _, row := range t.Rows {
		rec := make([]string, len(t.Headers))
		for i := range rec {
			if i < len(row) {
				rec[i] = row[i].Text
			}
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
