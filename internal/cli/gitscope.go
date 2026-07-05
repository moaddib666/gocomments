package cli

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type LineRange struct {
	Start, End int
}

func (r LineRange) Contains(line int) bool { return line >= r.Start && line <= r.End }

var hunkHeaderRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// ParseHunkHeader parses a `@@ -a,b +c,d @@` line into the new-file start
// line and line count. ok is false for non-hunk-header input.
func ParseHunkHeader(line string) (start, count int, ok bool) {
	m := hunkHeaderRe.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, false
	}
	start, _ = strconv.Atoi(m[1])
	count = 1
	if m[2] != "" {
		count, _ = strconv.Atoi(m[2])
	}
	return start, count, true
}

// ParseAddedRanges parses `git diff -U0` output into per-file added-line
// ranges in the new (post-change) file. Zero-count hunks (pure deletions)
// contribute no ranges.
func ParseAddedRanges(diff string) map[string][]LineRange {
	ranges := map[string][]LineRange{}
	var current string
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimPrefix(line, "+++ ")
			if p == "/dev/null" {
				current = ""
				continue
			}
			current = strings.TrimPrefix(p, "b/")
		case strings.HasPrefix(line, "@@ "):
			if current == "" {
				continue
			}
			start, count, ok := ParseHunkHeader(line)
			if !ok || count == 0 {
				continue
			}
			ranges[current] = append(ranges[current], LineRange{Start: start, End: start + count - 1})
		}
	}
	return ranges
}

const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gocomments: %s", msg)
	}
	return stdout.String(), nil
}

func GitToplevel(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DiffAddedRanges returns added-line ranges for path (relative to dir's git
// root, or a pathspec) between baseRef and the working tree.
func DiffAddedRanges(dir, baseRef, pathspec string) (map[string][]LineRange, error) {
	out, err := runGit(dir, "diff", "-U0", baseRef, "--", pathspec, "*.go")
	if err != nil {
		return nil, err
	}
	return ParseAddedRanges(out), nil
}

// CommitAddedRanges returns added-line ranges introduced by commit sha,
// falling back to a diff against the empty tree when sha is a root commit.
func CommitAddedRanges(dir, sha, pathspec string) (map[string][]LineRange, error) {
	if _, err := runGit(dir, "rev-parse", "--verify", sha+"^"); err != nil {
		return DiffAddedRanges(dir, emptyTreeSHA, pathspec)
	}
	return DiffAddedRanges(dir, sha+"^", pathspec)
}

// ShowFile returns the byte content of relPath as it existed at sha.
func ShowFile(dir, sha, relPath string) ([]byte, error) {
	out, err := runGit(dir, "show", sha+":"+relPath)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}
