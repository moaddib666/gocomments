package cli

import (
	"flag"
	"fmt"
	"io"
	"regexp"

	"gocomments/pkg/gocomments"
)

type listFlags struct {
	offset           int
	limit            int
	json             bool
	includeProtected bool
	diff             string
	commit           string
}

func bindListFlags(fs *flag.FlagSet, lf *listFlags) {
	fs.IntVar(&lf.offset, "offset", 0, "Skip this many rows before the first row returned")
	fs.IntVar(&lf.limit, "limit", 0, "Return at most this many rows (0 or negative = unlimited)")
	fs.BoolVar(&lf.json, "json", false, "Emit JSONL (one object per row); [] when empty")
	fs.BoolVar(&lf.includeProtected, "include-protected", false, "Include protected comments in the listing (default: unprotected only)")
	fs.StringVar(&lf.diff, "diff", "", "Only show comments added relative to this git base ref")
	fs.StringVar(&lf.commit, "commit", "", "Only show comments added by this git commit")
}

// runListOrSearch is the shared list/search runner: flag binding,
// --diff/--commit mutual-exclusion, arg validation, and the
// gather->paginate->write pipeline. search differs only by a required
// --pattern flag, threaded through as re when requirePattern is set.
func runListOrSearch(args []string, stdout, stderr io.Writer, name, usageLine, usageDesc string, requirePattern bool) int {
	valueFlags := commonScanValueFlags()
	if requirePattern {
		valueFlags["pattern"] = true
	}
	args = reorderArgs(args, valueFlags)

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var lf listFlags
	bindListFlags(fs, &lf)
	var pattern *string
	if requirePattern {
		pattern = fs.String("pattern", "", "RE2 regex applied to each comment's text (required)")
	}
	fs.Usage = func() {
		fmt.Fprintln(stderr, usageLine)
		fmt.Fprintln(stderr, usageDesc)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if lf.diff != "" && lf.commit != "" {
		fmt.Fprintln(stderr, "gocomments: --diff and --commit are mutually exclusive")
		return 2
	}

	var re *regexp.Regexp
	if requirePattern {
		if *pattern == "" {
			fmt.Fprintln(stderr, "gocomments: --pattern is required")
			return 2
		}
		var err error
		re, err = regexp.Compile(*pattern)
		if err != nil {
			fmt.Fprintf(stderr, "gocomments: invalid regex: %v\n", err)
			return 2
		}
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	path := fs.Arg(0)

	rows, scanErrs, err := gatherRows(path, re, lf.includeProtected, lf.diff, lf.commit)
	if err != nil {
		fmt.Fprintf(stderr, "gocomments: %v\n", err)
		return 1
	}
	for _, e := range scanErrs {
		fmt.Fprintf(stderr, "%v\n", e)
	}
	rows = Paginate(rows, lf.offset, lf.limit)
	if err := WriteRows(stdout, rows, lf.json); err != nil {
		fmt.Fprintf(stderr, "gocomments: %v\n", err)
		return 1
	}
	if len(scanErrs) > 0 {
		return 1
	}
	return 0
}

func RunList(args []string, stdout, stderr io.Writer) int {
	return runListOrSearch(args, stdout, stderr, "list",
		"Usage: gocomments list <path> [flags]",
		"List every comment found under <path> (a directory or a single .go file).",
		false)
}

func RunSearch(args []string, stdout, stderr io.Writer) int {
	return runListOrSearch(args, stdout, stderr, "search",
		"Usage: gocomments search <path> --pattern <regex> [flags]",
		"List comments whose text matches an RE2 regex.",
		true)
}

func gatherRows(path string, pattern *regexp.Regexp, includeProtected bool, diffRef, commit string) ([]Row, []error, error) {
	comments, scanErrs, err := Scan(path, ScanOptions{DiffRef: diffRef, Commit: commit})
	if err != nil {
		return nil, nil, err
	}
	filtered := make([]gocomments.Comment, 0, len(comments))
	for _, c := range comments {
		if !includeProtected && c.Protected {
			continue
		}
		if pattern != nil && !pattern.MatchString(c.Text) {
			continue
		}
		filtered = append(filtered, c)
	}
	return toRows(filtered), scanErrs, nil
}
