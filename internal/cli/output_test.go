package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"gocomments/pkg/gocomments"
)

func TestWriteRows_JSONEmptyIsBracketArray(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteRows(&buf, nil, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("empty json output must be '[]', got %q", got)
	}
}

func TestWriteRows_JSONShapeStableFieldNames(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{Schema: RowSchemaVersion, ID: "abc", File: "a.go", Line: 3, Kind: "doc", Protected: true, Text: "// x"}}
	if err := WriteRows(&buf, rows, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("output not valid JSON per line: %v (%q)", err, buf.String())
	}
	for _, field := range []string{"schema", "id", "file", "line", "kind", "protected", "text"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing field %q in JSON row: %v", field, m)
		}
	}
	if got := m["schema"]; got != float64(1) {
		t.Errorf("schema = %v, want 1", got)
	}
}

func TestToRows_SetsSchemaVersion(t *testing.T) {
	rows := toRows([]gocomments.Comment{{ID: "a", File: "f.go", Line: 1, Kind: gocomments.KindLine}})
	if len(rows) != 1 || rows[0].Schema != RowSchemaVersion {
		t.Fatalf("want schema=%d, got %+v", RowSchemaVersion, rows)
	}
}

func TestWriteRows_TSVDefault(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{ID: "abc", File: "a.go", Line: 3, Kind: "doc", Protected: true, Text: "// x"}}
	if err := WriteRows(&buf, rows, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "abc\ta.go\t3\tdoc\ttrue\t// x\n"
	if buf.String() != want {
		t.Fatalf("tsv row = %q, want %q", buf.String(), want)
	}
}

func TestWriteRows_TSVEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteRows(&buf, nil, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("empty tsv output should be empty, got %q", buf.String())
	}
}
