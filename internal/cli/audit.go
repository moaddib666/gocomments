package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"gocomments/pkg/gocomments"
)

var auditValueFlags = mergeValueFlags(commonScanValueFlags(), map[string]bool{
	"reason": true, "min-confidence": true,
})

var validReasons = func() map[string]bool {
	m := make(map[string]bool, len(gocomments.Reasons))
	for _, r := range gocomments.Reasons {
		m[r] = true
	}
	return m
}()

func RunAudit(args []string, stdout, stderr io.Writer) int {
	args = reorderArgs(args, auditValueFlags)

	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	offset := fs.Int("offset", 0, "Skip this many rows before the first row returned")
	limit := fs.Int("limit", 0, "Return at most this many rows (0 or negative = unlimited)")
	asJSON := fs.Bool("json", false, "Emit JSONL (one object per row); [] when empty")
	diff := fs.String("diff", "", "Only audit comments added relative to this git base ref")
	commit := fs.String("commit", "", "Only audit comments added by this git commit")
	reasonCSV := fs.String("reason", "", "Comma-separated list of reasons to include (default: all)")
	minConfidence := fs.String("min-confidence", "", "Minimum confidence tier to include: high|medium")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: gocomments audit <path> [flags]")
		fmt.Fprintln(stderr, "List likely-junk comments (including protected doc comments) via AST-aware heuristics. Read-only; ids pipe into delete.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *diff != "" && *commit != "" {
		fmt.Fprintln(stderr, "gocomments: --diff and --commit are mutually exclusive")
		return 2
	}
	if *minConfidence != "" && *minConfidence != "high" && *minConfidence != "medium" {
		fmt.Fprintln(stderr, "gocomments: --min-confidence must be high or medium")
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	path := fs.Arg(0)

	var allowedReasons map[string]bool
	if *reasonCSV != "" {
		allowedReasons = map[string]bool{}
		for _, r := range strings.Split(*reasonCSV, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if !validReasons[r] {
				fmt.Fprintf(stderr, "gocomments: unknown --reason %q, must be one of: %s\n", r, strings.Join(gocomments.Reasons, ", "))
				return 2
			}
			allowedReasons[r] = true
		}
	}

	comments, scanErrs, err := Scan(path, ScanOptions{DiffRef: *diff, Commit: *commit})
	if err != nil {
		fmt.Fprintf(stderr, "gocomments: %v\n", err)
		return 1
	}
	for _, e := range scanErrs {
		fmt.Fprintf(stderr, "%v\n", e)
	}

	rows := make([]AuditRow, 0)
	for _, c := range comments {
		reason, confidence, junk := gocomments.AuditComment(c)
		if !junk {
			continue
		}
		if allowedReasons != nil && !allowedReasons[reason] {
			continue
		}
		if *minConfidence == "high" && confidence != "high" {
			continue
		}
		rows = append(rows, AuditRow{
			Schema:     AuditRowSchemaVersion,
			ID:         c.ID,
			File:       c.File,
			Line:       c.Line,
			Kind:       string(c.Kind),
			Protected:  c.Protected,
			Reason:     reason,
			Confidence: confidence,
			Text:       c.Text,
		})
	}

	rows = Paginate(rows, *offset, *limit)
	if err := WriteAuditRows(stdout, rows, *asJSON); err != nil {
		fmt.Fprintf(stderr, "gocomments: %v\n", err)
		return 1
	}
	if len(scanErrs) > 0 {
		return 1
	}
	return 0
}
