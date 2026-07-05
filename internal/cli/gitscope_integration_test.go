package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.com",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func setupGitFixture(t *testing.T) (dir string, v1sha, v2sha string) {
	t.Helper()
	requireGit(t)
	dir = t.TempDir()
	runGitCmd(t, dir, "init", "-q")
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t_ = 1\n}\n")
	runGitCmd(t, dir, "add", "a.go")
	runGitCmd(t, dir, "commit", "-q", "-m", "v1")
	v1sha = strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// added by v2\n\t_ = 1\n}\n")
	runGitCmd(t, dir, "add", "a.go")
	runGitCmd(t, dir, "commit", "-q", "-m", "v2")
	v2sha = strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))
	return dir, v1sha, v2sha
}

func TestRunList_CommitScopeShowsOnlyAddedComments(t *testing.T) {
	dir, _, v2sha := setupGitFixture(t)

	var out, errBuf bytes.Buffer
	code := RunList([]string{"--json", "--commit", v2sha, dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "added by v2") {
		t.Fatalf("commit scope should show the comment added by v2: %q", out.String())
	}
	if strings.TrimSpace(out.String()) == "[]" {
		t.Fatalf("expected at least one row, got empty result")
	}
}

func TestRunList_DiffScopeIncludesUncommittedAdditions(t *testing.T) {
	dir, v1sha, _ := setupGitFixture(t)

	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// added by v2\n\t// uncommitted addition\n\t_ = 1\n}\n")

	var out, errBuf bytes.Buffer
	code := RunList([]string{"--json", "--diff", v1sha, dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "uncommitted addition") {
		t.Fatalf("diff scope should include uncommitted working-tree additions: %q", out.String())
	}
}

func TestRunList_SingleFilePath(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {\n\t// only here\n\t_ = 1\n}\n")
	writeGoFile(t, dir, "b.go", "package p\n\nfunc g() {\n\t// not here\n\t_ = 2\n}\n")

	var out, errBuf bytes.Buffer
	code := RunList([]string{filepath.Join(dir, "a.go")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "only here") || strings.Contains(out.String(), "not here") {
		t.Fatalf("single-file path should return just that file's rows: %q", out.String())
	}
}

func TestRunList_GitScope_NonRepoErrors(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package p\n\nfunc f() {}\n")

	var out, errBuf bytes.Buffer
	code := RunList([]string{"--diff", "HEAD", dir}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit for non-git-repo path")
	}
	if !strings.Contains(errBuf.String(), "gocomments:") {
		t.Fatalf("expected gocomments: prefixed error, got %q", errBuf.String())
	}
}

func TestRunList_GitScope_BadRefErrors(t *testing.T) {
	dir, _, _ := setupGitFixture(t)

	var out, errBuf bytes.Buffer
	code := RunList([]string{"--diff", "not-a-real-ref", dir}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown ref")
	}
}

func setupGitFixtureSubdir(t *testing.T) (dir string, v1sha, v2sha string) {
	t.Helper()
	requireGit(t)
	dir = t.TempDir()
	runGitCmd(t, dir, "init", "-q")
	writeGoFile(t, dir, "sub/a.go", "package p\n\nfunc f() {\n\t_ = 1\n}\n")
	runGitCmd(t, dir, "add", "sub/a.go")
	runGitCmd(t, dir, "commit", "-q", "-m", "v1")
	v1sha = strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	writeGoFile(t, dir, "sub/a.go", "package p\n\nfunc f() {\n\t// added by v2\n\t_ = 1\n}\n")
	runGitCmd(t, dir, "add", "sub/a.go")
	runGitCmd(t, dir, "commit", "-q", "-m", "v2")
	v2sha = strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))
	return dir, v1sha, v2sha
}

func TestRunDelete_IDFromGitScopedListingResolvesOnSubdirectoryPath(t *testing.T) {
	dir, _, v2sha := setupGitFixtureSubdir(t)
	sub := filepath.Join(dir, "sub")

	var out, errBuf bytes.Buffer
	code := RunList([]string{"--json", "--commit", v2sha, sub}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("list --commit on subdirectory failed: exit %d, stderr=%s", code, errBuf.String())
	}
	ids := extractIDs(t, out.String())
	if len(ids) != 1 {
		t.Fatalf("want 1 id from commit scope on subdirectory, got %v (%s)", ids, out.String())
	}

	out.Reset()
	errBuf.Reset()
	code = RunDelete([]string{"--id", ids[0], sub}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("id from git-scoped listing on a subdirectory should resolve when delete targets that same subdirectory: exit %d, stderr=%s", code, errBuf.String())
	}
}

func TestRunDelete_IDFromCommitScopeResolvesInWorkingTree(t *testing.T) {
	dir, _, v2sha := setupGitFixture(t)

	var out, errBuf bytes.Buffer
	RunList([]string{"--json", "--commit", v2sha, dir}, &out, &errBuf)
	ids := extractIDs(t, out.String())
	if len(ids) != 1 {
		t.Fatalf("want 1 id from commit scope, got %v (%s)", ids, out.String())
	}

	out.Reset()
	errBuf.Reset()
	code := RunDelete([]string{"--id", ids[0], dir}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("id from --commit scope should resolve in working tree: exit %d, stderr=%s", code, errBuf.String())
	}
}
