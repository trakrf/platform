// Package output renders command results in the three formats the CLI supports:
// a human-readable lipgloss-styled table (default), JSON (raw API value, for
// piping to jq), and CSV (for spreadsheet workflows).
package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	ltable "github.com/charmbracelet/lipgloss/table"
)

// Supported output formats.
const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatCSV   = "csv"
)

// Table is a columnar projection of a result set, used for the table and CSV
// formats. JSON output ignores it in favor of the raw value.
type Table struct {
	Columns []string
	Rows    [][]string
}

// Renderable bundles both views of a result: the raw value (marshaled for JSON)
// and a columnar projection (for table/CSV). Commands populate both.
type Renderable struct {
	JSON  any
	Table Table
}

// ParseFormat resolves the effective format from the --format value and the
// --json boolean shorthand. --json always wins; empty --format means table.
func ParseFormat(format string, jsonShorthand bool) (string, error) {
	if jsonShorthand {
		return FormatJSON, nil
	}
	switch format {
	case "", FormatTable:
		return FormatTable, nil
	case FormatJSON:
		return FormatJSON, nil
	case FormatCSV:
		return FormatCSV, nil
	default:
		return "", fmt.Errorf("unknown format %q (want table, json, or csv)", format)
	}
}

// Render writes r to w in the given format.
func Render(w io.Writer, format string, r Renderable) error {
	switch format {
	case FormatJSON:
		return renderJSON(w, r.JSON)
	case FormatCSV:
		return renderCSV(w, r.Table)
	case FormatTable, "":
		return renderTable(w, r.Table)
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func renderCSV(w io.Writer, t Table) error {
	cw := csv.NewWriter(w)
	if len(t.Columns) > 0 {
		if err := cw.Write(t.Columns); err != nil {
			return err
		}
	}
	if err := cw.WriteAll(t.Rows); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func renderTable(w io.Writer, t Table) error {
	if len(t.Rows) == 0 {
		_, err := fmt.Fprintln(w, "No results.")
		return err
	}
	headerStyle := lipgloss.NewStyle().Bold(true)
	tbl := ltable.New().
		Border(lipgloss.NormalBorder()).
		Headers(t.Columns...).
		Rows(t.Rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == ltable.HeaderRow {
				return headerStyle.Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	_, err := fmt.Fprintln(w, tbl.Render())
	return err
}
