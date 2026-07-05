# CLI Reference

```
gocomments <list|search|delete|audit> <path> [flags]
gocomments --version
```

`<path>` is a directory (walked recursively; `vendor/`, `testdata/`, and hidden directories are skipped, `_test.go` files are included) or a single `.go` file.

Flags may appear **before or after** `<path>` on every subcommand (`gocomments list --json ./x` and `gocomments list ./x --json` are equivalent). A positional that literally begins with `-` requires a `--` terminator.

## Output row

TSV (default, one row per comment, tab-separated) or JSONL (`--json`, one object per line; the literal `[]` when the result is empty):

| Column | Meaning |
|---|---|
| `id` | 12-hex stable comment id (see [id-scheme.md](id-scheme.md)) |
| `file` | slash-separated path relative to the rel base: the git toplevel of the target when it's inside a work tree, otherwise the scan root (or the file's directory in single-file mode) — the same base in every mode, including `--diff`/`--commit` |
| `line` | starting line of the comment group |
| `kind` | `line`, `block`, `doc`, or `directive` |
| `protected` | `true` when deletion requires `--force` |
| `text` | single-line text; newlines escaped as `\n`, tabs as `\t` |

`--json` rows also carry a leading `schema` integer field (currently `1`) identifying the row shape; TSV output is unaffected. Bump-worthy row-shape changes will increment `schema` and the tool `--version` together.

Rows are sorted by (file, byte offset) and are byte-identical across repeated runs on an unchanged tree — the pagination contract.

## list

```
gocomments list <path> [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--offset N` | 0 | Skip N rows |
| `--limit N` | 0 | Return at most N rows; 0 or negative = unlimited |
| `--json` | false | JSONL output |
| `--include-protected` | false | Set `=true` (or bare `--include-protected`) to also show protected comments |
| `--diff <base-ref>` | — | Only comments on lines added vs the ref (includes uncommitted changes) |
| `--commit <sha>` | — | Only comments added by that commit (content read from the commit) |

`--diff` and `--commit` are mutually exclusive and require `<path>` to be inside a git work tree.

## search

```
gocomments search <path> --pattern <regex> [flags]
```

All `list` flags plus required `--pattern` — an RE2 regex matched against each comment's text. Invalid regex → exit 2.

## delete

```
gocomments delete <path> --id <id> [--id ...] | --ids id1,id2 | --stdin [--force] [--dry-run]
```

| Flag | Meaning |
|---|---|
| `--id <id>` | Single id, repeatable |
| `--ids id1,id2` | Comma-separated bulk ids |
| `--stdin` | Newline-separated ids from stdin |
| `--force` | Allow deleting protected comments |
| `--dry-run` | Report what would be deleted without writing |

Ids are resolved against a fresh scan of the working tree. Per deleted id, stdout gets a confirmation line:

```
deleted a1b2c3d4e5f6 pkg/foo/bar.go:42
```

(`would delete ...` under `--dry-run`). Errors go to stderr as one line per problem (`<id>: not found`, `<id>: protected (directive), use --force`, `file:line: message`), followed by a summary:

```
N processed, M skipped
```

Deletion rules: whole-line comments remove the entire line (collapsing double blank lines), trailing comments (`x := 1 // note`) strip back to the code, multi-line blocks are removed as one range. Each rewritten file is re-parsed and format-checked, then replaced atomically (same-directory temp file + rename). A file that fails validation is left untouched and reported; the rest of the batch proceeds.

## audit

```
gocomments audit <path> [flags]
```

Emits only comments flagged as likely junk (including protected doc comments), for review before deletion. Row adds `reason` and `confidence` columns (`"schema":2` in JSON). Read-only. See [audit.md](audit.md) for the heuristics.

| Flag | Meaning |
|---|---|
| `--json` | JSONL output (`schema` 2) |
| `--offset N` / `--limit N` | pagination |
| `--reason <csv>` | keep only these reasons (`restates-name`, `commented-code`, `divider`, `low-value`) |
| `--min-confidence high\|medium` | drop rows below the tier |
| `--diff <base-ref>` / `--commit <sha>` | scope to comments added by a change |

Ids match `list` ids, so `audit` output pipes straight into `delete` (junk doc comments need `--force`).

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Fully clean run |
| 1 | Partial: some files failed to parse, some ids were not found/protected, or a rewrite was skipped — details on stderr |
| 2 | Usage error: bad flags, missing `--pattern`, invalid regex, `--diff`+`--commit` together, unknown subcommand |

## Version

```
gocomments --version    # gocomments 0.4.0
```

Row shape changes bump both `--version` and the JSON `schema` field — agent callers should pin on them. `list`/`search` rows are `schema` 1; `audit` rows are `schema` 2.
