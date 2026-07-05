package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gocomments/pkg/gocomments"
)

// RowSchemaVersion is the JSONL row schema version. Bump it whenever a field
// is added, removed, or changes meaning so agent callers can pin on it.
const RowSchemaVersion = 1

type Row struct {
	Schema    int    `json:"schema"`
	ID        string `json:"id"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Kind      string `json:"kind"`
	Protected bool   `json:"protected"`
	Text      string `json:"text"`
}

func (r Row) tsvFields() []string {
	return []string{r.ID, r.File, strconv.Itoa(r.Line), r.Kind, strconv.FormatBool(r.Protected), r.Text}
}

func toRows(comments []gocomments.Comment) []Row {
	rows := make([]Row, 0, len(comments))
	for _, c := range comments {
		rows = append(rows, Row{
			Schema:    RowSchemaVersion,
			ID:        c.ID,
			File:      c.File,
			Line:      c.Line,
			Kind:      string(c.Kind),
			Protected: c.Protected,
			Text:      c.Text,
		})
	}
	return rows
}

func WriteRows(w io.Writer, rows []Row, asJSON bool) error {
	return writeRows(w, rows, asJSON)
}

// AuditRowSchemaVersion is the JSONL schema for audit rows only; list/search
// rows keep RowSchemaVersion unchanged.
const AuditRowSchemaVersion = 2

type AuditRow struct {
	Schema     int    `json:"schema"`
	ID         string `json:"id"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Kind       string `json:"kind"`
	Protected  bool   `json:"protected"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
	Text       string `json:"text"`
}

func (r AuditRow) tsvFields() []string {
	return []string{r.ID, r.File, strconv.Itoa(r.Line), r.Kind, strconv.FormatBool(r.Protected), r.Reason, r.Confidence, r.Text}
}

func WriteAuditRows(w io.Writer, rows []AuditRow, asJSON bool) error {
	return writeRows(w, rows, asJSON)
}

type tsvRow interface {
	tsvFields() []string
}

func writeRows[T tsvRow](w io.Writer, rows []T, asJSON bool) error {
	if asJSON {
		return writeJSONL(w, rows)
	}
	return writeTSV(w, rows)
}

func writeTSV[T tsvRow](w io.Writer, rows []T) error {
	bw := bufio.NewWriter(w)
	for _, r := range rows {
		if _, err := fmt.Fprintf(bw, "%s\n", strings.Join(r.tsvFields(), "\t")); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func writeJSONL[T any](w io.Writer, rows []T) error {
	if len(rows) == 0 {
		_, err := io.WriteString(w, "[]\n")
		return err
	}
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}
