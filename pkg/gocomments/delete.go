package gocomments

import (
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileSkip struct {
	File string
	Err  error
}

func (s FileSkip) Error() string { return fmt.Sprintf("%s: %v", s.File, s.Err) }

// Resolve maps requested ids against a fresh scan, deduplicating and returning
// a per-id error for unknown or (without force) protected ids. It performs no
// I/O — a stale id simply resolves to nothing rather than hitting the wrong
// comment.
func Resolve(all []Comment, ids []string, force bool) (targets []Comment, idErrs map[string]error) {
	idErrs = map[string]error{}
	byID := make(map[string]Comment, len(all))
	for _, c := range all {
		byID[c.ID] = c
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		c, ok := byID[id]
		if !ok {
			idErrs[id] = fmt.Errorf("id not found: %s", id)
			continue
		}
		if c.Protected && !force {
			idErrs[id] = fmt.Errorf("protected comment (reason=%s): refusing without --force", c.Reason)
			continue
		}
		targets = append(targets, c)
	}
	return targets, idErrs
}

// ContainmentRoot returns the directory that writes must stay under for a
// given caller-supplied scan path: the path itself when it is a directory,
// or its containing directory for a single-file scan.
func ContainmentRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return abs, nil
	}
	return filepath.Dir(abs), nil
}

// ApplyDeletions splices the given targets out of their files, grouped and
// spliced back-to-front per file so earlier offsets in the same file stay
// valid. dryRun skips every write and reports the targets as if deleted.
// root is the containment boundary (NFR-SEC-1): a target file resolving
// outside root is skipped rather than written.
func ApplyDeletions(root string, targets []Comment, dryRun bool) (deleted []Comment, skipped []FileSkip) {
	byFile := map[string][]Comment{}
	var order []string
	for _, c := range targets {
		if _, ok := byFile[c.AbsPath]; !ok {
			order = append(order, c.AbsPath)
		}
		byFile[c.AbsPath] = append(byFile[c.AbsPath], c)
	}
	sort.Strings(order)

	for _, absPath := range order {
		cs := byFile[absPath]
		if dryRun {
			deleted = append(deleted, cs...)
			continue
		}
		if err := deleteInFile(root, absPath, cs); err != nil {
			skipped = append(skipped, FileSkip{File: absPath, Err: err})
			continue
		}
		deleted = append(deleted, cs...)
	}
	return deleted, skipped
}

// verifyContained rejects a write path whose containing directory, once
// symlinks are resolved, falls outside root — guards against a `..` in the
// scan path or a symlinked directory planted inside the tree pointing
// elsewhere. When path itself is a symlink (e.g. a symlinked file placed
// directly inside root), its resolved target is checked too, since the
// directory-only check above misses a same-directory file symlink that
// points outside root.
func verifyContained(root, path string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return err
	}
	resolvedDir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err := requireWithinRoot(resolvedRoot, resolvedDir, root, path); err != nil {
		return err
	}
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		resolvedTarget, evalErr := filepath.EvalSymlinks(path)
		if evalErr != nil {
			return evalErr
		}
		return requireWithinRoot(resolvedRoot, resolvedTarget, root, path)
	}
	return nil
}

func requireWithinRoot(resolvedRoot, candidate, root, path string) error {
	rel, err := filepath.Rel(resolvedRoot, candidate)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes scan root %s: %s", root, path)
	}
	return nil
}

func deleteInFile(root, path string, targets []Comment) error {
	if err := verifyContained(root, path); err != nil {
		return err
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].StartOffset > targets[j].StartOffset
	})

	buf := append([]byte(nil), original...)
	for _, c := range targets {
		start, end := c.StartOffset, c.EndOffset
		if ds, de, whole := wholeLineRange(buf, start, end); whole {
			start, end = ds, de
			if ns, ne, collapse := collapseBlankAround(buf, start, end); collapse {
				start, end = ns, ne
			}
		} else {
			start = trailingStart(buf, start)
		}
		buf = append(buf[:start:start], buf[end:]...)
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, path, buf, parser.ParseComments); err != nil {
		return fmt.Errorf("re-parse failed: %w", err)
	}
	if _, err := format.Source(buf); err != nil {
		return fmt.Errorf("format check failed: %w", err)
	}
	return atomicWrite(path, buf)
}

func trailingStart(content []byte, start int) int {
	for start > 0 && (content[start-1] == ' ' || content[start-1] == '\t') {
		start--
	}
	return start
}

func wholeLineRange(content []byte, start, end int) (int, int, bool) {
	lineStart := trailingStart(content, start)
	if lineStart > 0 && content[lineStart-1] != '\n' {
		return 0, 0, false
	}
	lineEnd := end
	for lineEnd < len(content) && (content[lineEnd] == ' ' || content[lineEnd] == '\t') {
		lineEnd++
	}
	if lineEnd < len(content) {
		if content[lineEnd] != '\n' {
			return 0, 0, false
		}
		lineEnd++
	}
	return lineStart, lineEnd, true
}

func collapseBlankAround(content []byte, delStart, delEnd int) (int, int, bool) {
	prevBlank := delStart >= 2 && content[delStart-2] == '\n'
	if !prevBlank {
		return delStart, delEnd, false
	}
	if delEnd < len(content) && content[delEnd] == '\n' {
		return delStart, delEnd + 1, true
	}
	if delEnd == len(content) {
		return delStart - 1, delEnd, true
	}
	return delStart, delEnd, false
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".gocomments-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if info, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmpPath, info.Mode())
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
