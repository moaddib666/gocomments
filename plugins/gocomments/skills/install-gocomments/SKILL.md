---
name: install-gocomments
description: Install and verify the gocomments CLI binary (AST-based Go comment audit/removal tool). Use when gocomments is not on PATH, when a find/remove-junk-comments request needs the tool first, or when the user asks to install/update/set up gocomments. Runs a self-check install checklist end to end.
---

# install-gocomments

Get the `gocomments` binary installed and confirmed working. Run this whenever `gocomments` is missing or out of date; it is a prerequisite for the `find-junk-comments` skill.

## Self-check install checklist

Run each step; do not proceed past a failing step until it's resolved.

### 1. Is it already installed and current?

```bash
gocomments --version
```

- Prints `gocomments 0.4.0` (or newer) → **done**, skip to "Confirm it works".
- `command not found` or an older version → continue.

### 2. Is Go available? (required to install)

```bash
go version
```

- Prints `go1.20` or newer → continue.
- Missing → install Go from https://go.dev/dl/ (or `brew install go` on macOS), then re-check. gocomments needs Go 1.20+.

### 3. Install the binary

```bash
go install github.com/moaddib666/gocomments/cmd/gocomments@latest
```

This builds and drops `gocomments` into `$(go env GOPATH)/bin`.

### 4. Is that bin dir on PATH?

```bash
gocomments --version || echo "not on PATH"
```

- Prints a version → **PATH is fine**.
- `not on PATH` → add the Go bin dir to PATH:
  ```bash
  echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc
  export PATH="$(go env GOPATH)/bin:$PATH"
  ```
  Re-run `gocomments --version`.

### 5. Confirm it works (smoke test)

```bash
tmp=$(mktemp -d)
printf 'package p\n\ntype T struct {\n\t// Name is the name.\n\tName string\n}\n' > "$tmp/x.go"
gocomments audit --json "$tmp"        # should flag the restating "// Name is the name." doc comment
rm -rf "$tmp"
```

Seeing one `restates-name` row means the install is healthy.

## Alternative: build from source

```bash
git clone https://github.com/moaddib666/gocomments && cd gocomments && make build
# binary at ./bin/gocomments
```

## Next

Once `gocomments --version` works, use the **find-junk-comments** skill to audit and remove junk comments in a Go project.
