package cli

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moaddib666/gocomments/pkg/gocomments"
)

func scanForTest(t *testing.T, dir string) ([]gocomments.Comment, []error) {
	t.Helper()
	return gocomments.ScanPath(dir)
}

func writeGoFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestRunList_TSVAndJSONDeterminism(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// keep\n\t_ = 1\n}\n")
	writeGoFile(t, dir, "b.go", "package p\n\nfunc g() {\n\t// keep too\n\t_ = 2\n}\n")

	const iterations = 3
	var first string
	for i := 0; i < iterations; i++ {
		var out, errBuf bytes.Buffer
		if code := RunList([]string{"--json", dir}, &out, &errBuf); code != 0 {
			t.Fatalf("iteration %d: exit %d: %s", i, code, errBuf.String())
		}
		if i == 0 {
			first = out.String()
			continue
		}
		if out.String() != first {
			t.Fatalf("--json output not byte-identical across runs, iteration %d:\n%s\nvs\n%s", i, out.String(), first)
		}
	}
	if strings.Count(first, "\n") != 2 {
		t.Fatalf("expected 2 rows, got:\n%s", first)
	}
}

func TestRunList_UnknownPathError(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := RunList([]string{"/no/such/path"}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown path")
	}
}

func TestRunSearch_FiltersAndInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// alpha note\n\t_ = 1\n\t// beta note\n\t_ = 2\n}\n")

	var out, errBuf bytes.Buffer
	if code := RunSearch([]string{"--pattern", "alpha", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "alpha") || strings.Contains(out.String(), "beta") {
		t.Fatalf("search did not filter correctly: %q", out.String())
	}

	out.Reset()
	errBuf.Reset()
	code := RunSearch([]string{"--pattern", "(", dir}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid regex")
	}
	if !strings.Contains(errBuf.String(), "regex") {
		t.Fatalf("expected regex error message, got %q", errBuf.String())
	}
}

func TestRunSearch_MultilineBlockFlattenedToSingleRow(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\n/*\nmulti\nline\n*/\nfunc f() {}\n")

	var out, errBuf bytes.Buffer
	if code := RunList([]string{dir}, &out, &errBuf); code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("multi-line block comment must flatten to one row, got %d: %q", len(lines), out.String())
	}
	if strings.Contains(lines[0], "\n") {
		t.Fatalf("row must not contain a literal newline: %q", lines[0])
	}
}

func TestRunDelete_SingleBulkStdinAndUnknownID(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// one\n\t_ = 1\n\t// two\n\t_ = 2\n\t// three\n\t_ = 3\n}\n")

	var out, errBuf bytes.Buffer
	if code := RunList([]string{"--json", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("list failed: %s", errBuf.String())
	}
	ids := extractIDs(t, out.String())
	if len(ids) != 3 {
		t.Fatalf("want 3 ids, got %v", ids)
	}

	out.Reset()
	errBuf.Reset()
	code := RunDelete([]string{"--id", ids[0], dir}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("single delete failed: %s", errBuf.String())
	}
	if !strings.Contains(out.String(), "deleted "+ids[0]) {
		t.Fatalf("missing confirmation line: %q", out.String())
	}

	out.Reset()
	errBuf.Reset()
	code = RunDelete([]string{"--ids", "nonexistent-id", dir}, strings.NewReader(""), &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown id")
	}
	if !strings.Contains(errBuf.String(), "nonexistent-id") {
		t.Fatalf("expected unknown id error, got %q", errBuf.String())
	}

	out.Reset()
	errBuf.Reset()
	code = RunDelete([]string{"--stdin", dir}, strings.NewReader(ids[1]+"\n"), &out, &errBuf)
	if code != 0 {
		t.Fatalf("stdin delete failed: %s", errBuf.String())
	}

	remaining, errBuf2 := scanForTest(t, dir)
	if len(errBuf2) != 0 {
		t.Fatalf("file must still parse: %v", errBuf2)
	}
	if len(remaining) != 1 {
		t.Fatalf("want 1 comment left, got %d: %+v", len(remaining), remaining)
	}
}

func TestRunDelete_ProtectedRefusalThenForce(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\n// Foo is exported.\nfunc Foo() {}\n")

	before, _ := os.ReadFile(filepath.Join(dir, "a.go"))

	var out, errBuf bytes.Buffer
	RunList([]string{"--json", "--include-protected", dir}, &out, &errBuf)
	ids := extractIDs(t, out.String())
	if len(ids) != 1 {
		t.Fatalf("want 1 id, got %v", ids)
	}

	out.Reset()
	errBuf.Reset()
	code := RunDelete([]string{"--id", ids[0], dir}, strings.NewReader(""), &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit for protected delete without --force")
	}
	if !strings.Contains(errBuf.String(), "protected") {
		t.Fatalf("expected protected error, got %q", errBuf.String())
	}
	after, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	if string(before) != string(after) {
		t.Fatalf("file must be untouched after refused protected delete")
	}

	out.Reset()
	errBuf.Reset()
	code = RunDelete([]string{"--id", ids[0], "--force", dir}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("--force delete should succeed: %s", errBuf.String())
	}
}

func TestRunDelete_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// note\n\t_ = 1\n}\n")
	before, _ := os.ReadFile(path)

	var out, errBuf bytes.Buffer
	RunList([]string{"--json", dir}, &out, &errBuf)
	ids := extractIDs(t, out.String())

	out.Reset()
	errBuf.Reset()
	code := RunDelete([]string{"--id", ids[0], "--dry-run", dir}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("dry-run should succeed: %s", errBuf.String())
	}
	if !strings.Contains(out.String(), "would delete") {
		t.Fatalf("dry-run should say 'would delete', got %q", out.String())
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("dry-run must not modify the file")
	}
}

func TestRunDelete_MultiFileBatchInOneInvocation(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// a-note\n\t_ = 1\n}\n")
	writeGoFile(t, dir, "b.go", "package p\n\nfunc g() {\n\t// b-note\n\t_ = 2\n}\n")

	var out, errBuf bytes.Buffer
	if code := RunList([]string{"--json", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("list failed: %s", errBuf.String())
	}
	ids := extractIDs(t, out.String())
	if len(ids) != 2 {
		t.Fatalf("want 2 ids, got %v", ids)
	}

	out.Reset()
	errBuf.Reset()
	code := RunDelete([]string{"--ids", strings.Join(ids, ","), dir}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("multi-file batch delete failed: %s", errBuf.String())
	}

	for _, name := range []string{"a.go", "b.go"} {
		path := filepath.Join(dir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(content), "note") {
			t.Fatalf("%s: comment should have been deleted, got:\n%s", name, content)
		}
		formatted, err := format.Source(content)
		if err != nil {
			t.Fatalf("%s: format.Source: %v", name, err)
		}
		if string(formatted) != string(content) {
			t.Fatalf("%s: not gofmt-clean after batch delete", name)
		}
	}

	remaining, errs := scanForTest(t, dir)
	if len(errs) != 0 || len(remaining) != 0 {
		t.Fatalf("expected no comments left, got %+v (errs=%v)", remaining, errs)
	}
}

func TestRunDelete_MixedProtectedAndUnprotectedBatchWithoutForce(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\n// Foo is exported.\nfunc Foo() {}\n\nfunc g() {\n\t// plain note\n\t_ = 1\n}\n")

	comments, errs := scanForTest(t, dir)
	if len(errs) != 0 {
		t.Fatalf("scan errors: %v", errs)
	}
	var protectedID, plainID string
	for _, c := range comments {
		if c.Protected {
			protectedID = c.ID
		} else {
			plainID = c.ID
		}
	}
	if protectedID == "" || plainID == "" {
		t.Fatalf("fixture must contain one protected and one unprotected comment: %+v", comments)
	}

	var out, errBuf bytes.Buffer
	code := RunDelete([]string{"--id", protectedID, "--id", plainID, dir}, strings.NewReader(""), &out, &errBuf)
	if code != 1 {
		t.Fatalf("mixed batch without --force should exit 1, got %d (stderr=%s)", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), protectedID) || !strings.Contains(errBuf.String(), "protected") {
		t.Fatalf("expected per-id protected error on stderr, got %q", errBuf.String())
	}
	if !strings.Contains(out.String(), "deleted "+plainID) {
		t.Fatalf("expected unprotected id to be deleted, got %q", out.String())
	}

	remaining, errs2 := scanForTest(t, dir)
	if len(errs2) != 0 {
		t.Fatalf("file must still parse: %v", errs2)
	}
	found := false
	for _, c := range remaining {
		if c.ID == protectedID {
			found = true
		}
	}
	if !found {
		t.Fatalf("protected comment must remain after refusal without --force: %+v", remaining)
	}
}

func TestGatherRows_TenThousandCommentsPaginationCorrectness(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	sb.WriteString("package p\n\n")
	const n = 10000
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "// c%d\nvar V%d = %d\n", i, i, i)
	}
	writeGoFile(t, dir, "big.go", sb.String())

	rows, scanErrs, err := gatherRows(dir, nil, true, "", "")
	if err != nil {
		t.Fatalf("gatherRows error: %v", err)
	}
	if len(scanErrs) != 0 {
		t.Fatalf("unexpected scan errors: %v", scanErrs)
	}
	if len(rows) != n {
		t.Fatalf("want %d rows, got %d", n, len(rows))
	}

	tail := Paginate(rows, n-5, 10)
	if len(tail) != 5 {
		t.Fatalf("want 5 rows at tail, got %d", len(tail))
	}
	for i, r := range tail {
		if r.ID != rows[n-5+i].ID {
			t.Fatalf("tail pagination mismatch at %d: %s != %s", i, r.ID, rows[n-5+i].ID)
		}
	}

	mid := Paginate(rows, 4000, 100)
	if len(mid) != 100 {
		t.Fatalf("want 100 rows, got %d", len(mid))
	}
	for i, r := range mid {
		if r.ID != rows[4000+i].ID {
			t.Fatalf("mid pagination mismatch at %d: %s != %s", i, r.ID, rows[4000+i].ID)
		}
	}
}

func TestFlagsAnywhere_UnknownFlagStillErrors(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {}\n")

	cases := []func([]string, *bytes.Buffer, *bytes.Buffer) int{
		func(a []string, o, e *bytes.Buffer) int { return RunList(a, o, e) },
		func(a []string, o, e *bytes.Buffer) int { return RunAudit(a, o, e) },
		func(a []string, o, e *bytes.Buffer) int { return RunSearch(a, o, e) },
		func(a []string, o, e *bytes.Buffer) int {
			return RunDelete(a, strings.NewReader(""), o, e)
		},
	}
	for _, run := range cases {
		var out, errBuf bytes.Buffer
		code := run([]string{dir, "--nope"}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("expected exit 2 for unknown flag, got %d (stderr=%s)", code, errBuf.String())
		}
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func TestRunDelete_DotPathResolvesToCWD(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// dot-note\n\t_ = 1\n}\n")
	chdir(t, dir)

	var out, errBuf bytes.Buffer
	if code := RunList([]string{"--json", "."}, &out, &errBuf); code != 0 {
		t.Fatalf("list . failed: %s", errBuf.String())
	}
	ids := extractIDs(t, out.String())
	if len(ids) != 1 {
		t.Fatalf("want 1 id, got %v", ids)
	}

	out.Reset()
	errBuf.Reset()
	code := RunDelete([]string{"--id", ids[0], "."}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("delete . failed: %s", errBuf.String())
	}
	content, err := os.ReadFile(filepath.Join(dir, "a.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(content), "dot-note") {
		t.Fatalf("delete . should have removed the comment, got:\n%s", content)
	}
}

func TestRunDelete_RelativeSubdirAndBarePathResolve(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "sub/x.go", "package p\n\nfunc f() {\n\t// sub-note\n\t_ = 1\n}\n")
	writeGoFile(t, dir, "bare.go", "package p\n\nfunc g() {\n\t// bare-note\n\t_ = 2\n}\n")
	chdir(t, dir)

	var out, errBuf bytes.Buffer
	if code := RunList([]string{"--json", "./sub/x.go"}, &out, &errBuf); code != 0 {
		t.Fatalf("list ./sub/x.go failed: %s", errBuf.String())
	}
	subIDs := extractIDs(t, out.String())
	if len(subIDs) != 1 {
		t.Fatalf("want 1 id, got %v", subIDs)
	}
	out.Reset()
	errBuf.Reset()
	if code := RunDelete([]string{"--id", subIDs[0], "./sub/x.go"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("delete ./sub/x.go failed: %s", errBuf.String())
	}
	subContent, err := os.ReadFile(filepath.Join(dir, "sub", "x.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(subContent), "sub-note") {
		t.Fatalf("delete ./sub/x.go should have removed the comment, got:\n%s", subContent)
	}

	out.Reset()
	errBuf.Reset()
	if code := RunList([]string{"--json", "bare.go"}, &out, &errBuf); code != 0 {
		t.Fatalf("list bare.go failed: %s", errBuf.String())
	}
	bareIDs := extractIDs(t, out.String())
	if len(bareIDs) != 1 {
		t.Fatalf("want 1 id, got %v", bareIDs)
	}
	out.Reset()
	errBuf.Reset()
	if code := RunDelete([]string{"--id", bareIDs[0], "bare.go"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("delete bare.go failed: %s", errBuf.String())
	}
	bareContent, err := os.ReadFile(filepath.Join(dir, "bare.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(bareContent), "bare-note") {
		t.Fatalf("delete bare.go should have removed the comment, got:\n%s", bareContent)
	}
}

func extractIDs(t *testing.T, jsonl string) []string {
	t.Helper()
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(jsonl), "\n") {
		if line == "" || line == "[]" {
			continue
		}
		i := strings.Index(line, `"id":"`)
		if i < 0 {
			t.Fatalf("no id field in row: %q", line)
		}
		rest := line[i+len(`"id":"`):]
		end := strings.Index(rest, `"`)
		ids = append(ids, rest[:end])
	}
	return ids
}
