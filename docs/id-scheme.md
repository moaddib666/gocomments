# Comment ID Scheme

Every comment group gets a 12-hex-character id:

```
id = hex(sha256(relPath + "\x00" + text + "\x00" + occurrenceIndex))[:12]
```

- `relPath` — slash-separated file path relative to the rel base: the git toplevel of the target path when it is inside a git work tree, otherwise the scan root directory itself (or the containing directory for single-file mode). This applies identically to plain tree scans, single-file scans, and `--diff`/`--commit` scans, so scanning a subdirectory of a repo yields the same ids as scanning from the repo toplevel. Absolute location of the repo on disk never affects ids.
- `text` — the raw comment group text as parsed by `go/ast`.
- `occurrenceIndex` — 0-based index of this exact (relPath, text) pair within the file, ordered by byte offset; disambiguates duplicate comments like repeated `// TODO`.

## Stability guarantees

- Deterministic: the same tree always produces the same ids — repeated `list` runs are byte-identical.
- Line-shift proof: ids do not include line numbers, so editing unrelated code above or below a comment does not change its id. A listing taken earlier stays usable after other edits.
- Duplicate-safe: two identical comments in one file get distinct ids via the occurrence index.

## What changes an id

- Editing the comment's own text.
- Moving/renaming the file (relPath changes).
- Adding or removing an identical duplicate comment earlier in the same file (occurrence index shifts).

## Stale-id behavior

`delete` never trusts positions from an old listing — it re-scans the working tree and resolves ids by content. An id whose comment no longer exists (deleted, edited, file moved) resolves to nothing and fails with `<id>: not found` (exit 1). There is no code path that deletes by line/offset from a stale listing, so a stale id can never remove the wrong comment — the only theoretical exception is the occurrence-index shift described above, which requires an identical duplicate to appear/disappear between list and delete.
