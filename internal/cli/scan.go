package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/moaddib666/gocomments/pkg/gocomments"
)

type ScanOptions struct {
	DiffRef string
	Commit  string
}

func Scan(path string, opts ScanOptions) ([]gocomments.Comment, []error, error) {
	if opts.DiffRef == "" && opts.Commit == "" {
		comments, errs := gocomments.ScanPath(path)
		return comments, errs, nil
	}
	return scanGitScoped(path, opts.DiffRef, opts.Commit)
}

// gitSource is the single dispatch point selecting the added-ranges source
// and file-content reader for diff vs commit mode. scanGitScoped and
// scanOneGitFile consume it instead of branching on the mode themselves.
type gitSource struct {
	addedRanges func(top, pathspec string) (map[string][]LineRange, error)
	readFile    func(top, relFile string) ([]byte, error) // nil => read the working tree from disk
}

func selectGitSource(diffRef, commit string) gitSource {
	if commit != "" {
		return gitSource{
			addedRanges: func(top, pathspec string) (map[string][]LineRange, error) {
				return CommitAddedRanges(top, commit, pathspec)
			},
			readFile: func(top, relFile string) ([]byte, error) {
				return ShowFile(top, commit, relFile)
			},
		}
	}
	return gitSource{
		addedRanges: func(top, pathspec string) (map[string][]LineRange, error) {
			return DiffAddedRanges(top, diffRef, pathspec)
		},
	}
}

func scanGitScoped(path, diffRef, commit string) ([]gocomments.Comment, []error, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	dir := path
	if !info.IsDir() {
		dir = filepath.Dir(path)
	}
	top, err := GitToplevel(dir)
	if err != nil {
		return nil, nil, err
	}

	src := selectGitSource(diffRef, commit)
	ranges, err := src.addedRanges(top, path)
	if err != nil {
		return nil, nil, err
	}

	var comments []gocomments.Comment
	var errs []error
	for relFile, lineRanges := range ranges {
		fileComments, ferr := scanOneGitFile(top, relFile, src)
		if ferr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", relFile, ferr))
			continue
		}
		for _, c := range fileComments {
			if intersectsAny(c.Line, c.EndLine, lineRanges) {
				comments = append(comments, c)
			}
		}
	}

	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].File != comments[j].File {
			return comments[i].File < comments[j].File
		}
		return comments[i].StartOffset < comments[j].StartOffset
	})
	return comments, errs, nil
}

func scanOneGitFile(top, relFile string, src gitSource) ([]gocomments.Comment, error) {
	if src.readFile == nil {
		comments, errs := gocomments.ScanPath(filepath.Join(top, relFile))
		if len(errs) > 0 {
			return nil, errs[0]
		}
		return comments, nil
	}
	content, err := src.readFile(top, relFile)
	if err != nil {
		return nil, err
	}
	return gocomments.ScanBytes(filepath.Join(top, relFile), content, relFile)
}

func intersectsAny(start, end int, ranges []LineRange) bool {
	for _, r := range ranges {
		if start <= r.End && end >= r.Start {
			return true
		}
	}
	return false
}
