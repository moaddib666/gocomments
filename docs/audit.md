# audit — likely-junk detection

`gocomments audit <path>` emits **only** comments that heuristics flag as likely junk, so an agent can review a short candidate list instead of grep/awk over the full inventory. It audits **doc comments too** — `protected` is a deletion-safety flag, not a quality judgment, so a protected doc comment can still be pure noise. `audit` is strictly read-only; its ids resolve in `delete`.

```
gocomments audit <path> [--json] [--offset N] [--limit N] [--reason csv] [--min-confidence high|medium] [--diff <ref> | --commit <sha>]
```

Row shape adds two columns to the standard inventory row: `id file line kind protected reason confidence text` (TSV) / same JSON fields with `"schema":2`.

## Heuristics

Evaluated in precedence order; the first match wins. Directives (`//go:*`, `//nolint`, `//line`, `//export`, `// +build`, cgo) are **never** flagged.

| reason | fires when | confidence |
|---|---|---|
| `restates-name` | the comment documents a **single** decl/field/method and, after dropping English function words (articles, copulas, `is/are/was/what/actually/returns/holds/…`), every remaining content token is covered by the identifier name (camelCase-split, case- and singular/plural-insensitive) | `high` (≤6 content tokens) / `medium` |
| `commented-code` | a **standalone** (non-trailing) comment whose body parses as a Go statement or expression, is not sentence-shaped prose (3+ alphabetic words ending in a period are excluded), and is not a literal-LHS assignment | `high` (full statement — assignment/return/control-flow) / `medium` (expression-only, the riskier prose-adjacent case) |
| `divider` | the text is only rule characters `[-=*#_/ ]` and whitespace | `high` |
| `low-value` | a single content token and the comment is unprotected | `medium` |

`restates-name` is the one that needs the AST: it compares the comment against the exact identifier it documents. Examples from real code:

- `// Expected is what was expected.` on field `Expected` → flagged (`what`, `was` are function words; `expected` echoes the name).
- `// Observed is what was actually observed.` on field `Observed` → flagged.
- `// Key is the raw Ed25519 public key.` on field `Key` → **not** flagged (`raw`, `ed25519`, `public` add information).

## Precision over recall

A false positive risks deleting a good comment, so the heuristics deliberately under-flag when uncertain. Qualifier-adding docs, design references (`DESIGN-001`, `NFR-SEC-3`, `Invariant 72`), and ordinary prose are left alone. Treat `medium` confidence as "read before deleting."

Two categories are deliberately **not** flagged, because a real-world run showed they are usually meaningful:

- **Trailing/inline legend comments** (`maxTokens int // 0 = unlimited`) — a comment sharing its line with code is a note, not commented-out code, so `commented-code` skips it. Literal-LHS assignments like `// 0 = unlimited` are skipped even when standalone.
- **`const`/`var` group headers** (`// LLM events.` heading a multi-constant block) — such a doc documents the group, not a single identifier, so `restates-name` never fires on it. Per-spec docs, struct fields, interface methods, and single-spec decls are still audited.

## audit → delete recipe

```bash
BIN=/Users/moaddib/development/ai-tmp/gocomments/bin/gocomments
# 1. review candidates (high-confidence only)
"$BIN" audit --json --min-confidence high <path>
# 2. collect the ids you agree are junk, dry-run
"$BIN" delete --ids id1,id2,id3 --dry-run <path>
# 3. delete for real — junk doc comments are protected, so --force is required
"$BIN" delete --ids id1,id2,id3 --force <path>
```

Filter by category with `--reason`:

```bash
gocomments audit --reason divider,low-value <path>      # only the safe-to-nuke kinds
gocomments audit --reason restates-name <path>          # only name-echoing docs
```

Scope to a change with `--diff <ref>` / `--commit <sha>` to audit only comments a branch or commit introduced.
