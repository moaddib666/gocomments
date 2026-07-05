package cli

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"gocomments/pkg/gocomments"
)

func RunDelete(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	args = reorderArgs(args, map[string]bool{"id": true, "ids": true})

	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var ids idList
	fs.Var(&ids, "id", "Comment id to delete (repeatable)")
	idsCSV := fs.String("ids", "", "Comma-separated comment ids to delete")
	useStdin := fs.Bool("stdin", false, "Read newline-separated comment ids from stdin")
	force := fs.Bool("force", false, "Allow deleting protected comments")
	dryRun := fs.Bool("dry-run", false, "Print what would be deleted without writing")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: gocomments delete <path> --id <id> [--id ...] | --ids id1,id2 | --stdin [--force] [--dry-run]")
		fmt.Fprintln(stderr, "Delete comments by id via byte-range splice on the original source.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	path := fs.Arg(0)

	requested, err := CollectIDs([]string(ids), *idsCSV, *useStdin, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "gocomments: %v\n", err)
		return 2
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(stderr, "gocomments: %v\n", err)
		return 1
	}

	root, err := gocomments.ContainmentRoot(abs)
	if err != nil {
		fmt.Fprintf(stderr, "gocomments: %v\n", err)
		return 1
	}

	all, scanErrs := gocomments.ScanPath(abs)
	for _, e := range scanErrs {
		fmt.Fprintf(stderr, "%v\n", e)
	}

	targets, idErrs := gocomments.Resolve(all, requested, *force)
	exit := 0
	for _, id := range requested {
		if err, ok := idErrs[id]; ok {
			fmt.Fprintf(stderr, "%s: %v\n", id, err)
			exit = 1
		}
	}

	deleted, skipped := gocomments.ApplyDeletions(root, targets, *dryRun)
	for _, s := range skipped {
		fmt.Fprintf(stderr, "%s: %v\n", s.File, s.Err)
		exit = 1
	}
	for _, c := range deleted {
		verb := "deleted"
		if *dryRun {
			verb = "would delete"
		}
		fmt.Fprintf(stdout, "%s %s %s:%d\n", verb, c.ID, c.File, c.Line)
	}

	fmt.Fprintf(stderr, "%d processed, %d skipped\n", len(deleted), len(skipped)+len(idErrs))
	if len(scanErrs) > 0 {
		exit = 1
	}
	return exit
}
