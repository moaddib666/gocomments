---
name: find-junk-comments
description: Use the gocomments CLI to find and remove non-meaningful ("junk") comments in a Go source tree — restating doc comments, commented-out code, divider banners, and low-value noise — via an AST-aware `audit` command, then delete them by stable id. Trigger when the user asks to find/list/audit/remove junk or redundant comments in a Go project, "run gocomments", or clean up comments. Go source only.
---

# find-junk-comments

Drive the `gocomments` CLI to find and remove non-meaningful comments from a Go source tree. Primary entry point is `audit` (it flags likely-junk comments); you review, then `delete`.

## Prerequisite

The `gocomments` binary must be on PATH:

```bash
gocomments --version   # expect 0.4.0+
```

If it's missing, run the **install-gocomments** skill first (or `go install github.com/moaddib666/gocomments/cmd/gocomments@latest`).

## Audit-and-fix on any Go project (one-shot)

Point it at any Go module or directory and clean it up end to end:

```bash
ROOT=.                         # the Go project (repo root, package, or a single .go file)
gocomments audit "$ROOT" --json --min-confidence high > /tmp/junk.jsonl   # 1. audit
# 2. review /tmp/junk.jsonl — keep meaningful comments, collect the ids that are real junk
IDS=$(python3 -c "import json;print(','.join(json.loads(l)['id'] for l in open('/tmp/junk.jsonl')))")
gocomments delete "$ROOT" --ids "$IDS" --dry-run                          # 3. preview
gocomments delete "$ROOT" --ids "$IDS" --force                           # 4. fix (byte-splice, gofmt-clean)
go build ./... && gofmt -l .                                             # 5. verify nothing broke
```

Works on any project because ids are content-derived and paths are relative — no per-repo setup. Step 2 (human/agent judgment) is not optional: the heuristics favor precision, but `low-value` in particular flags section/enum labels worth keeping. For a fully unattended sweep of only the safest categories, restrict with `--reason divider,commented-code` and skip individual review.

## Key principle: protected ≠ meaningful

`protected=true` on a comment is a **deletion-safety** flag (doc comments and compiler directives), NOT a signal that the comment has value. A protected doc comment like `// Expected is what was expected.` is pure noise. Audit doc comments **by content**, not by protection — `audit` does this for you. Only directives (`//go:*`, `//nolint`, `//line`, `//export`, `// +build`, cgo) are never flagged, since deleting them changes program behavior.

## Workflow

1. **Audit** — a short, reasoned candidate list (not the full inventory):
   ```bash
   gocomments audit <path> --json --min-confidence high > junk.jsonl
   ```
   Each row has a `reason` and `confidence`:
   - `restates-name` — a doc/field/method comment that just echoes its identifier (`// Name is the name`). AST-detected.
   - `commented-code` — a standalone comment whose body parses as Go (dead code).
   - `divider` — a banner/rule (`---`/`===`/`***`) with no text.
   - `low-value` — a single-token unprotected comment (`// TODO`, `// ok`).
   Filter with `--reason restates-name,commented-code,divider,low-value`.

2. **Judge, don't bulk-nuke.** Precision is favored, but read each row — keep comments that explain a non-obvious *why*; `low-value` in particular flags section/enum labels that are often worth keeping. Collect the ids you agree are junk.

3. **Dry-run:**
   ```bash
   gocomments delete <path> --ids id1,id2 --dry-run
   ```

4. **Delete** — junk doc comments are `protected`, so removing them needs `--force`:
   ```bash
   gocomments delete <path> --ids id1,id2 --force
   ```
   Deletion is a byte-range splice (no reformat), re-parsed and written atomically. Exit `0` = clean, `1` = partial (read stderr), `2` = usage error.

Flags may go before or after `<path>`; `<path>` may be a directory or a single `.go` file, and `.`/relative paths work.

## Scope to a change (PR review)

```bash
gocomments audit <path> --diff main       # comments added vs main (incl. uncommitted)
gocomments audit <path> --commit <sha>    # comments added by that commit
```

## Manual inventory / search

```bash
gocomments list <path> --json --include-protected     # every comment, protected included
gocomments search <path> --pattern '(?i)todo|fixme'   # RE2 regex over comment text
```

## Reporting back

Summarize: how many comments audited vs flagged, the reason breakdown, and a shortlist (id + file:line + reason + text). Never delete without an explicit go-ahead unless asked to clean up directly; when you do, dry-run then delete (with `--force` for doc comments) and report the diff.
