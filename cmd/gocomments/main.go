package main

import (
	"fmt"
	"os"

	"gocomments/internal/cli"
)

const version = "0.4.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "list":
		return cli.RunList(args[1:], os.Stdout, os.Stderr)
	case "search":
		return cli.RunSearch(args[1:], os.Stdout, os.Stderr)
	case "delete":
		return cli.RunDelete(args[1:], os.Stdin, os.Stdout, os.Stderr)
	case "audit":
		return cli.RunAudit(args[1:], os.Stdout, os.Stderr)
	case "-h", "--help", "help":
		usage()
		return 0
	case "--version", "-version", "version":
		fmt.Fprintf(os.Stdout, "gocomments %s\n", version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "gocomments: unknown subcommand %q\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: gocomments <list|search|delete|audit> <path> [flags]")
	fmt.Fprintln(os.Stderr, "Run `gocomments <subcommand> --help` for subcommand flags.")
}
