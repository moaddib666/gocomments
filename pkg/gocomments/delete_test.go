package gocomments

import (
	"go/format"
	"os"
	"path/filepath"
	"testing"
)

func mustGofmtClean(t *testing.T, path string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	formatted, err := format.Source(got)
	if err != nil {
		t.Fatalf("format.Source(%s): %v", path, err)
	}
	if string(formatted) != string(got) {
		t.Errorf("%s not gofmt-clean after delete:\n--- got ---\n%s\n--- want ---\n%s", path, got, formatted)
	}
}

func TestResolve_UnknownID(t *testing.T) {
	all := []Comment{{ID: "abc123"}}
	targets, errs := Resolve(all, []string{"missing"}, false)
	if len(targets) != 0 {
		t.Fatalf("no targets expected, got %v", targets)
	}
	if err, ok := errs["missing"]; !ok || err == nil {
		t.Fatalf("expected error for unknown id, got %v", errs)
	}
}

func TestResolve_ProtectedRequiresForce(t *testing.T) {
	all := []Comment{{ID: "abc123", Protected: true, Reason: "doc"}}
	targets, errs := Resolve(all, []string{"abc123"}, false)
	if len(targets) != 0 {
		t.Fatalf("protected id must not resolve without force: %v", targets)
	}
	if errs["abc123"] == nil {
		t.Fatalf("expected protected-without-force error")
	}

	targets, errs = Resolve(all, []string{"abc123"}, true)
	if len(targets) != 1 || len(errs) != 0 {
		t.Fatalf("force should resolve protected id, got targets=%v errs=%v", targets, errs)
	}
}

func TestResolve_DedupesRepeatedIDs(t *testing.T) {
	all := []Comment{{ID: "x"}}
	targets, errs := Resolve(all, []string{"x", "x"}, false)
	if len(targets) != 1 || len(errs) != 0 {
		t.Fatalf("duplicate ids should resolve once, got %v %v", targets, errs)
	}
}

func TestApplyDeletions_WholeLineTrailingBlockAndEOF(t *testing.T) {
	dir := t.TempDir()
	src := `package p

// leading whole-line comment
func F() {
	x := 1 // trailing note
	_ = x
}

/*
multi
line
block
*/
func G() {}

// eof comment without newline after`
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected scan errors: %v", errs)
	}
	if len(comments) != 4 {
		t.Fatalf("want 4 comments, got %d: %+v", len(comments), comments)
	}

	var ids []string
	for _, c := range comments {
		ids = append(ids, c.ID)
	}
	targets, errsMap := Resolve(comments, ids, true)
	if len(errsMap) != 0 {
		t.Fatalf("unexpected resolve errors: %v", errsMap)
	}

	deleted, skipped := ApplyDeletions(dir, targets, false)
	if len(skipped) != 0 {
		t.Fatalf("unexpected skips: %v", skipped)
	}
	if len(deleted) != 4 {
		t.Fatalf("want 4 deleted, got %d", len(deleted))
	}

	remaining, errs2 := ScanPath(dir)
	if len(errs2) != 0 {
		t.Fatalf("file must still parse after delete: %v", errs2)
	}
	if len(remaining) != 0 {
		t.Fatalf("all comments should be gone, found %+v", remaining)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "\tx := 1\n\t_ = x"; !contains(string(got), want) {
		t.Errorf("trailing comment should strip back to last code byte, got:\n%s", got)
	}
	mustGofmtClean(t, path)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestApplyDeletions_BackToFrontSameFileBulk(t *testing.T) {
	dir := t.TempDir()
	src := `package p

func a() {
	// one
	_ = 1
	// two
	_ = 2
	// three
	_ = 3
}
`
	path := filepath.Join(dir, "a.go")
	os.WriteFile(path, []byte(src), 0o644)

	comments, _ := ScanPath(dir)
	targets, _ := Resolve(comments, []string{comments[0].ID, comments[2].ID}, false)
	deleted, skipped := ApplyDeletions(dir, targets, false)
	if len(skipped) != 0 || len(deleted) != 2 {
		t.Fatalf("bulk delete failed: deleted=%v skipped=%v", deleted, skipped)
	}

	remaining, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("file must parse: %v", errs)
	}
	if len(remaining) != 1 || remaining[0].Text != "// two" {
		t.Fatalf("want only '// two' left, got %+v", remaining)
	}
	mustGofmtClean(t, path)
}

func TestApplyDeletions_ProtectedRefusalLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\n// Foo is exported.\nfunc Foo() {}\n"
	path := filepath.Join(dir, "a.go")
	os.WriteFile(path, []byte(src), 0o644)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	comments, _ := ScanPath(dir)
	_, errsMap := Resolve(comments, []string{comments[0].ID}, false)
	if len(errsMap) != 1 {
		t.Fatalf("expected protected refusal, got %v", errsMap)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("file must be untouched after refused protected delete")
	}
}

func TestApplyDeletions_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\nfunc f() {\n\t// note\n\t_ = 1\n}\n"
	path := filepath.Join(dir, "a.go")
	os.WriteFile(path, []byte(src), 0o644)

	comments, _ := ScanPath(dir)
	targets, _ := Resolve(comments, []string{comments[0].ID}, false)
	deleted, skipped := ApplyDeletions(dir, targets, true)
	if len(skipped) != 0 || len(deleted) != 1 {
		t.Fatalf("dry-run should report the planned deletion: deleted=%v skipped=%v", deleted, skipped)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(after) != src {
		t.Fatalf("dry-run must not modify the file")
	}
}

func TestContainmentRoot_ResolvesRelativeAndDotPaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package p\n\nfunc f() {}\n")

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	root, err := ContainmentRoot(".")
	if err != nil {
		t.Fatalf("ContainmentRoot(.): %v", err)
	}
	if !filepath.IsAbs(root) {
		t.Fatalf("ContainmentRoot(.) must return an absolute path, got %q", root)
	}

	fileRoot, err := ContainmentRoot("a.go")
	if err != nil {
		t.Fatalf("ContainmentRoot(a.go): %v", err)
	}
	if !filepath.IsAbs(fileRoot) {
		t.Fatalf("ContainmentRoot(a.go) must return an absolute path, got %q", fileRoot)
	}
	if fileRoot != root {
		t.Fatalf("ContainmentRoot(a.go) = %q, want %q", fileRoot, root)
	}
}

func TestApplyDeletions_DeleteListDeleteRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\nfunc f() {\n\t// a\n\t_ = 1\n\t// b\n\t_ = 2\n}\n"
	path := filepath.Join(dir, "a.go")
	os.WriteFile(path, []byte(src), 0o644)

	first, _ := ScanPath(dir)
	targets, _ := Resolve(first, []string{first[0].ID}, false)
	ApplyDeletions(dir, targets, false)

	second, _ := ScanPath(dir)
	if len(second) != 1 {
		t.Fatalf("want 1 remaining comment, got %+v", second)
	}
	_, errsMap := Resolve(second, []string{first[0].ID}, false)
	if errsMap[first[0].ID] == nil {
		t.Fatalf("stale id from first scan must not resolve after deletion")
	}
	mustGofmtClean(t, path)
}
