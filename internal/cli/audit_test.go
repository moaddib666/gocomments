package cli

import (
	"bytes"
	"go/format"
	"os"
	"strings"
	"testing"
)

func TestRunAudit_MixedTreeOnlyJunkRowsWithReasonAndConfidence(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", `package p

type Result struct {
	// Expected is what was expected.
	Expected string
	// Key is the raw Ed25519 public key
	Key string
}

func f() {
	// ----------
	x := 1 // x := f(a,b)
	_ = x
	// noop
}
`)

	var out, errBuf bytes.Buffer
	code := RunAudit([]string{"--json", dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("audit failed: exit %d stderr=%s", code, errBuf.String())
	}
	if strings.Contains(out.String(), "Key is the raw") {
		t.Fatalf("qualifier-adding doc must not appear in audit output: %s", out.String())
	}
	for _, want := range []string{`"reason":"restates-name"`, `"reason":"divider"`, `"reason":"low-value"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %s in audit output, got:\n%s", want, out.String())
		}
	}
	if !strings.Contains(out.String(), `"confidence":`) {
		t.Fatalf("expected confidence field in audit rows: %s", out.String())
	}
	if !strings.Contains(out.String(), `"schema":2`) {
		t.Fatalf("expected schema=2 on audit rows: %s", out.String())
	}
}

func TestRunAudit_EmptyResultIsBracketArray(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\n// Foo is exported and does useful application-specific work.\nfunc Foo() {}\n")

	var out, errBuf bytes.Buffer
	code := RunAudit([]string{"--json", dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("audit failed: %s", errBuf.String())
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("expected empty JSON array, got %q", out.String())
	}
}

func TestRunAudit_IdsResolveInDelete(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// noop\n\t_ = 1\n}\n")

	var out, errBuf bytes.Buffer
	if code := RunAudit([]string{"--json", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("audit failed: %s", errBuf.String())
	}
	ids := extractIDs(t, out.String())
	if len(ids) != 1 {
		t.Fatalf("want 1 audited id, got %v", ids)
	}

	out.Reset()
	errBuf.Reset()
	code := RunDelete([]string{"--id", ids[0], "--force", dir}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("delete of audit id failed: %s", errBuf.String())
	}
	if !strings.Contains(out.String(), "deleted "+ids[0]) {
		t.Fatalf("expected deletion confirmation, got %q", out.String())
	}
}

func TestRunAudit_ProtectedDocRoundtripStaysGofmtClean(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "a.go", `package p

type Result struct {
	// Expected is what was expected.
	Expected string
}
`)

	var out, errBuf bytes.Buffer
	if code := RunAudit([]string{"--json", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("audit failed: %s", errBuf.String())
	}
	ids := extractIDs(t, out.String())
	if len(ids) != 1 {
		t.Fatalf("want 1 audited id, got %v", ids)
	}

	out.Reset()
	errBuf.Reset()
	code := RunDelete([]string{"--id", ids[0], "--force", dir}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("delete failed: %s", errBuf.String())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	formatted, err := format.Source(content)
	if err != nil {
		t.Fatalf("format.Source: %v", err)
	}
	if string(formatted) != string(content) {
		t.Fatalf("file not gofmt-clean after deleting protected junk doc:\n%s", content)
	}
}

func TestRunAudit_NeverWrites(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// noop\n\t_ = 1\n}\n")
	before, _ := os.ReadFile(path)

	var out, errBuf bytes.Buffer
	if code := RunAudit([]string{"--json", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("audit failed: %s", errBuf.String())
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("audit must never write to files")
	}
}

func TestRunAudit_MalformedFileSkipAndContinue(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "good.go", "package p\n\nfunc f() {\n\t// noop\n\t_ = 1\n}\n")
	writeGoFile(t, dir, "bad.go", "package p\nfunc broken( {\n")

	var out, errBuf bytes.Buffer
	code := RunAudit([]string{"--json", dir}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("want exit 1 on partial scan error, got %d", code)
	}
	if !strings.Contains(out.String(), `"reason":"low-value"`) {
		t.Fatalf("good.go should still be audited: %s", out.String())
	}
}

func TestRunAudit_FlagsAnywhere(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// noop\n\t_ = 1\n}\n")

	var before, after bytes.Buffer
	RunAudit([]string{"--json", dir}, &before, &bytes.Buffer{})
	RunAudit([]string{dir, "--json"}, &after, &bytes.Buffer{})
	if before.String() != after.String() {
		t.Fatalf("flags-anywhere: audit output differs by flag position:\n%s\nvs\n%s", before.String(), after.String())
	}
}

func TestFlagsAnywhere_AllSubcommands(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// alpha\n\t_ = 1\n}\n")

	var listBefore, listAfter bytes.Buffer
	RunList([]string{"--json", dir}, &listBefore, &bytes.Buffer{})
	RunList([]string{dir, "--json"}, &listAfter, &bytes.Buffer{})
	if listBefore.String() != listAfter.String() {
		t.Fatalf("list: flags-anywhere mismatch")
	}

	var searchBefore, searchAfter bytes.Buffer
	RunSearch([]string{"--pattern", "alpha", "--json", dir}, &searchBefore, &bytes.Buffer{})
	RunSearch([]string{dir, "--json", "--pattern", "alpha"}, &searchAfter, &bytes.Buffer{})
	if searchBefore.String() != searchAfter.String() {
		t.Fatalf("search: flags-anywhere mismatch")
	}

	dir2 := t.TempDir()
	writeGoFile(t, dir2, "a.go", "package p\n\nfunc f() {\n\t// beta\n\t_ = 1\n}\n")
	dir3 := t.TempDir()
	writeGoFile(t, dir3, "a.go", "package p\n\nfunc f() {\n\t// beta\n\t_ = 1\n}\n")

	var listOut bytes.Buffer
	RunList([]string{"--json", dir2}, &listOut, &bytes.Buffer{})
	ids := extractIDs(t, listOut.String())
	if len(ids) != 1 {
		t.Fatalf("want 1 id, got %v", ids)
	}

	var delBefore, delAfter bytes.Buffer
	RunDelete([]string{"--id", ids[0], "--dry-run", dir2}, strings.NewReader(""), &delBefore, &bytes.Buffer{})
	RunDelete([]string{dir3, "--dry-run", "--id", ids[0]}, strings.NewReader(""), &delAfter, &bytes.Buffer{})
	if delBefore.String() != delAfter.String() {
		t.Fatalf("delete: flags-anywhere mismatch: %q vs %q", delBefore.String(), delAfter.String())
	}
}

func TestRunAudit_ReasonAndMinConfidenceFilters(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// ----------\n\tx := 1\n\t// noop\n\t_ = x\n}\n")

	var out, errBuf bytes.Buffer
	code := RunAudit([]string{"--json", "--reason", "divider", dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("audit failed: %s", errBuf.String())
	}
	if strings.Contains(out.String(), "low-value") {
		t.Fatalf("--reason filter should exclude low-value rows: %s", out.String())
	}
	if !strings.Contains(out.String(), "divider") {
		t.Fatalf("--reason filter should keep divider rows: %s", out.String())
	}

	out.Reset()
	errBuf.Reset()
	code = RunAudit([]string{"--json", "--min-confidence", "high", dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("audit failed: %s", errBuf.String())
	}
	if strings.Contains(out.String(), `"confidence":"medium"`) {
		t.Fatalf("--min-confidence=high should exclude medium rows: %s", out.String())
	}
}

func TestRunAudit_UnknownReasonExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {}\n")

	var out, errBuf bytes.Buffer
	code := RunAudit([]string{"--reason", "bogus-reason", dir}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("expected exit 2 for unknown --reason, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "bogus-reason") {
		t.Fatalf("expected error message to name the unknown reason, got %q", errBuf.String())
	}
}

func TestRunAudit_InvalidMinConfidence(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {}\n")

	var out, errBuf bytes.Buffer
	code := RunAudit([]string{"--min-confidence", "bogus", dir}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("expected exit 2 for invalid --min-confidence, got %d", code)
	}
}

func TestListSearchJSONOutput_ByteIdenticalToV02Schema(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// note\n\t_ = 1\n}\n")

	var out, errBuf bytes.Buffer
	if code := RunList([]string{"--json", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("list failed: %s", errBuf.String())
	}
	line := strings.TrimSpace(out.String())
	if strings.Contains(line, "reason") || strings.Contains(line, "confidence") {
		t.Fatalf("list JSON row must not gain reason/confidence fields: %s", line)
	}
	if !strings.Contains(line, `"schema":1`) {
		t.Fatalf("list JSON row schema must remain 1: %s", line)
	}
	wantFields := []string{`"schema"`, `"id"`, `"file"`, `"line"`, `"kind"`, `"protected"`, `"text"`}
	for _, f := range wantFields {
		if !strings.Contains(line, f) {
			t.Fatalf("list JSON row missing expected field %s: %s", f, line)
		}
	}
}
