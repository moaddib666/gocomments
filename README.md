# gocomments

AST-based CLI for inventorying and deleting comments in Go source trees. Built for LLM agents that need to strip non-meaningful comments safely: every comment gets a stable content-derived id, output is deterministic and machine-parseable, deletion is a surgical byte-range splice that never reformats surrounding code, and compiler/tool directives are protected from accidental removal.

## Quickstart

```bash
make build            # -> bin/gocomments
bin/gocomments audit ./myproject --json          # <- find likely-junk comments
bin/gocomments list ./myproject --limit 20
bin/gocomments search ./myproject --pattern '(?i)todo|fixme'
bin/gocomments delete ./myproject --id a1b2c3d4e5f6
```

Flags may go before or after `<path>` on any subcommand.

## Commands

| Command | Purpose |
|---|---|
| `audit <path> [--reason csv --min-confidence high\|medium --json ...]` | **Find likely-junk comments** (restating docs, commented-out code, dividers, low-value) via AST-aware heuristics; row adds `reason` + `confidence`; read-only; ids pipe into `delete` |
| `list <path> [--offset N --limit N --json --include-protected --diff <ref> \| --commit <sha>]` | Paginated inventory of unprotected comments (pass `--include-protected` to also show protected ones); `<path>` is a directory tree or a single `.go` file |
| `search <path> --pattern <RE2-regex> [same flags]` | Same rows, filtered by regex over comment text |
| `delete <path> --id <id> [--id ...] \| --ids id1,id2 \| --stdin [--force --dry-run]` | Remove comments by id; protected ones require `--force` |

Row shape (TSV columns / JSONL fields): `id  file  line  kind  protected  text` (`audit` adds `reason  confidence`).

`--diff <base-ref>` limits output to comments on lines added relative to a git ref (including uncommitted changes); `--commit <sha>` shows comments added by that specific commit, reading file content from the commit itself.

## LLM usage recipe

1. `gocomments audit <root> --json --min-confidence high` — get a short list of likely-junk comments, each with a `reason`. This audits **doc comments too** — being `protected` only means deletion needs `--force`, not that a comment is meaningful.
2. Read each flagged row and confirm it's genuinely junk (the heuristics favor precision, but `medium` confidence deserves a look).
3. `gocomments delete <root> --ids <csv> --dry-run` then re-run without `--dry-run` (add `--force` for protected doc comments).
4. Exit code `0` = fully clean, `1` = partial (some ids/files skipped — parse stderr), `2` = usage error.

Ids survive line shifts (hash of relative path + comment text + occurrence index), so a listing stays valid while other edits happen; a stale id simply fails with "not found" instead of deleting the wrong thing.

## Documentation

- [docs/audit.md](docs/audit.md) — junk heuristics, reasons, confidence, audit→delete recipe
- [docs/architecture.md](docs/architecture.md) — module layout and pipeline
- [docs/cli.md](docs/cli.md) — full flag, output, and exit-code reference
- [docs/id-scheme.md](docs/id-scheme.md) — id hashing, stability guarantees, stale-id behavior

## Development

```bash
make test   # go test ./... -count=1
make vet    # go vet ./...
make fmt    # gofmt -l -w .
```

Stdlib only — no external dependencies.
