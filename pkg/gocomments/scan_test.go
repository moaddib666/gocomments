package gocomments

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func requireGitForTest(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	requireGitForTest(t)
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

const fixtureSrc = `package p

// Foo is exported and documented.
func Foo() {
	x := 1 // trailing note
	_ = x
}

//go:generate mockgen -source=a.go

//nolint:unused
func bar() {}
`

func TestScanTree_MultiFileDeterminism(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", fixtureSrc)
	writeFile(t, dir, "b.go", "package p\n\n// just a note\nfunc Baz() {}\n")

	first, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for i := 0; i < 3; i++ {
		again, errs2 := ScanPath(dir)
		if len(errs2) != 0 {
			t.Fatalf("unexpected errors on rerun: %v", errs2)
		}
		if len(again) != len(first) {
			t.Fatalf("nondeterministic comment count: %d vs %d", len(again), len(first))
		}
		for i := range first {
			if first[i].ID != again[i].ID || first[i].File != again[i].File || first[i].Text != again[i].Text {
				t.Fatalf("nondeterministic run at index %d: %+v vs %+v", i, first[i], again[i])
			}
		}
	}

	var files []string
	for _, c := range first {
		files = append(files, c.File)
	}
	sort.Strings(files)
	if files[0] != "a.go" {
		t.Errorf("expected a.go to sort first, got %v", files)
	}
}

func TestScanTree_SkipsVendorTestdataAndHidden(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package p\n\n// keep\nfunc F() {}\n")
	writeFile(t, dir, "vendor/dep.go", "package dep\n\n// skip\nfunc D() {}\n")
	writeFile(t, dir, "testdata/fixture.go", "package td\n\n// skip\nfunc T() {}\n")
	writeFile(t, dir, ".hidden/x.go", "package h\n\n// skip\nfunc H() {}\n")

	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(comments) != 1 {
		t.Fatalf("want 1 comment (vendor/testdata/hidden skipped), got %d: %+v", len(comments), comments)
	}
	if comments[0].File != "main.go" {
		t.Errorf("want main.go, got %s", comments[0].File)
	}
}

func TestScanTree_MalformedFileSkipAndContinue(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.go", "package p\n\n// ok\nfunc F() {}\n")
	writeFile(t, dir, "bad.go", "package p\nfunc broken( {\n")

	comments, errs := ScanPath(dir)
	if len(errs) != 1 {
		t.Fatalf("want 1 parse error, got %d: %v", len(errs), errs)
	}
	if len(comments) != 1 || comments[0].File != "good.go" {
		t.Fatalf("good.go should still be scanned: %+v", comments)
	}
}

func TestScanTree_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(comments) != 0 {
		t.Fatalf("want 0 comments, got %d", len(comments))
	}
}

func TestScanSingleFile_TreeConsistentIDs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", fixtureSrc)

	treeComments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	single, errs2 := ScanPath(filepath.Join(dir, "a.go"))
	if len(errs2) != 0 {
		t.Fatalf("unexpected errors scanning single file: %v", errs2)
	}
	if len(single) != len(treeComments) {
		t.Fatalf("single-file scan count %d != tree scan count %d", len(single), len(treeComments))
	}
	for i := range single {
		if single[i].ID != treeComments[i].ID {
			t.Errorf("single-file id %s should match tree id %s (text=%q)", single[i].ID, treeComments[i].ID, single[i].Text)
		}
	}
}

func TestScanTree_DirectiveAndDocClassification(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", fixtureSrc)

	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	byText := map[string]Comment{}
	for _, c := range comments {
		byText[c.Text] = c
	}

	doc, ok := byText["// Foo is exported and documented."]
	if !ok || doc.Kind != KindDoc || !doc.Protected {
		t.Errorf("doc comment misclassified: %+v", doc)
	}
	trailing, ok := byText["// trailing note"]
	if !ok || trailing.Protected {
		t.Errorf("trailing comment should be unprotected: %+v", trailing)
	}
	directive, ok := byText["//go:generate mockgen -source=a.go"]
	if !ok || directive.Kind != KindDirective || !directive.Protected {
		t.Errorf("directive misclassified: %+v", directive)
	}
	nolint, ok := byText["//nolint:unused"]
	if !ok || nolint.Kind != KindDirective || !nolint.Protected {
		t.Errorf("nolint directive misclassified: %+v", nolint)
	}
}

func TestScanTree_SubdirRelBaseMatchesGitToplevel(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeFile(t, dir, "sub/a.go", fixtureSrc)

	fromToplevel, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	fromSubdir, errs2 := ScanPath(filepath.Join(dir, "sub"))
	if len(errs2) != 0 {
		t.Fatalf("unexpected errors: %v", errs2)
	}
	if len(fromToplevel) != len(fromSubdir) {
		t.Fatalf("comment count differs: toplevel=%d subdir=%d", len(fromToplevel), len(fromSubdir))
	}
	for i := range fromToplevel {
		if fromToplevel[i].ID != fromSubdir[i].ID || fromToplevel[i].File != fromSubdir[i].File {
			t.Fatalf("id/file mismatch scanning subdir vs toplevel at %d: %+v vs %+v", i, fromToplevel[i], fromSubdir[i])
		}
	}
}

func TestWalkGoFiles_SkipsSymlinks(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, outside, "secret.go", "package p\n\n// secret comment\nfunc S() {}\n")

	root := t.TempDir()
	writeFile(t, root, "main.go", "package p\n\n// keep\nfunc F() {}\n")
	if err := os.Symlink(filepath.Join(outside, "secret.go"), filepath.Join(root, "link.go")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	comments, errs := ScanPath(root)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(comments) != 1 || comments[0].File != "main.go" {
		t.Fatalf("symlinked file must not be walked/scanned, got %+v", comments)
	}
}

func TestVerifyContained_WithinRootAccepted(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := verifyContained(root, filepath.Join(sub, "a.go")); err != nil {
		t.Fatalf("unexpected error for path within root: %v", err)
	}
}

func TestVerifyContained_DotDotEscapeRejected(t *testing.T) {
	root := t.TempDir()
	escaping := filepath.Join(root, "..", "escape.go")
	if err := verifyContained(root, escaping); err == nil {
		t.Fatalf("expected error for path escaping root via ..")
	}
}

func TestApplyDeletions_RejectsPathEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "evil.go")
	src := "package p\n\n// x\nfunc F() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	targets := []Comment{{
		ID:          "escape1",
		AbsPath:     path,
		StartOffset: strings.Index(src, "// x"),
		EndOffset:   strings.Index(src, "// x") + len("// x"),
	}}
	deleted, skipped := ApplyDeletions(root, targets, false)
	if len(deleted) != 0 {
		t.Fatalf("target escaping root must not be deleted, got %v", deleted)
	}
	if len(skipped) != 1 {
		t.Fatalf("want 1 skip for path escaping root, got %v", skipped)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != src {
		t.Fatalf("file outside root must be left untouched, got:\n%s", got)
	}
}

func TestVerifyContained_SymlinkEscapeRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.go")
	if err := os.WriteFile(outsideFile, []byte("package p\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	linkPath := filepath.Join(root, "link.go")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	if err := verifyContained(root, linkPath); err == nil {
		t.Fatalf("expected error for symlink escaping root")
	}
}

func TestVerifyContained_SymlinkedDirEscapeRejected(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	linkDir := filepath.Join(root, "linkdir")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	if err := verifyContained(root, filepath.Join(linkDir, "a.go")); err == nil {
		t.Fatalf("expected error for path under a symlinked dir escaping root")
	}
}

func TestApplyDeletions_RejectsSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.go")
	src := "package p\n\n// x\nfunc F() {}\n"
	if err := os.WriteFile(outsideFile, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	linkPath := filepath.Join(root, "link.go")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	targets := []Comment{{
		ID:          "escapelink",
		AbsPath:     linkPath,
		StartOffset: strings.Index(src, "// x"),
		EndOffset:   strings.Index(src, "// x") + len("// x"),
	}}
	deleted, skipped := ApplyDeletions(root, targets, false)
	if len(deleted) != 0 {
		t.Fatalf("target via symlink escaping root must not be deleted, got %v", deleted)
	}
	if len(skipped) != 1 {
		t.Fatalf("want 1 skip for symlink escaping root, got %v", skipped)
	}
	got, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != src {
		t.Fatalf("file outside root must be left untouched via symlink, got:\n%s", got)
	}
}

func TestScanTree_TenThousandCommentsUniqueFormattedIDs(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	sb.WriteString("package p\n\n")
	const n = 10000
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "// comment number %d\nvar V%d = %d\n", i, i, i)
	}
	writeFile(t, dir, "big.go", sb.String())

	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(comments) != n {
		t.Fatalf("want %d comments, got %d", n, len(comments))
	}

	idRe := regexp.MustCompile(`^[0-9a-f]{12}$`)
	seen := make(map[string]bool, len(comments))
	for _, c := range comments {
		if !idRe.MatchString(c.ID) {
			t.Fatalf("id %q does not match ^[0-9a-f]{12}$", c.ID)
		}
		if seen[c.ID] {
			t.Fatalf("duplicate id %q", c.ID)
		}
		seen[c.ID] = true
	}
}
