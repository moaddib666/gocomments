package gocomments

import (
	"path/filepath"
	"testing"
)

func TestHashIDStability(t *testing.T) {
	id1 := hashID("a/b.go", "// hello", 0)
	id2 := hashID("a/b.go", "// hello", 0)
	if id1 != id2 {
		t.Fatalf("hashID not stable: %q != %q", id1, id2)
	}
	if len(id1) != 12 {
		t.Fatalf("want 12 hex chars, got %d (%q)", len(id1), id1)
	}
}

func TestHashIDCollisionResistance(t *testing.T) {
	seen := map[string]bool{}
	texts := []string{"// a", "// b", "// c", "/* block */", "// a\n// b"}
	for _, text := range texts {
		for occ := 0; occ < 3; occ++ {
			id := hashID("pkg/file.go", text, occ)
			if seen[id] {
				t.Fatalf("collision for text=%q occ=%d", text, occ)
			}
			seen[id] = true
		}
	}
}

func TestHashIDDistinctPaths(t *testing.T) {
	idA := hashID("x.go", "// same", 0)
	idB := hashID("y.go", "// same", 0)
	if idA == idB {
		t.Fatalf("different relPath must yield different ids")
	}
}

// TestHashIDRelativePathIndependence proves the real independence guarantee:
// the same relPath+text+occurrence yields the same id regardless of where
// the scan root sits on disk, by scanning two identical trees rooted at two
// different t.TempDir() locations.
func TestHashIDRelativePathIndependence(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeFile(t, dirA, "pkg/a.go", fixtureSrc)
	writeFile(t, dirB, "pkg/a.go", fixtureSrc)

	commentsA, errsA := ScanPath(dirA)
	if len(errsA) != 0 {
		t.Fatalf("unexpected errors: %v", errsA)
	}
	commentsB, errsB := ScanPath(dirB)
	if len(errsB) != 0 {
		t.Fatalf("unexpected errors: %v", errsB)
	}
	if len(commentsA) != len(commentsB) {
		t.Fatalf("comment count differs between identical trees: %d vs %d", len(commentsA), len(commentsB))
	}
	if filepath.Clean(dirA) == filepath.Clean(dirB) {
		t.Fatalf("test setup bug: temp dirs must differ")
	}
	for i := range commentsA {
		if commentsA[i].ID != commentsB[i].ID {
			t.Fatalf("id depends on absolute scan-root location: %s vs %s (%+v vs %+v)",
				commentsA[i].ID, commentsB[i].ID, commentsA[i], commentsB[i])
		}
	}
}

func TestComputeIDsOccurrenceIndex(t *testing.T) {
	entries := []rawEntry{
		{file: "a.go", startOffset: 10, rawText: "// dup"},
		{file: "a.go", startOffset: 0, rawText: "// dup"},
		{file: "a.go", startOffset: 20, rawText: "// unique"},
	}
	out := computeIDs(entries)
	if len(out) != 3 {
		t.Fatalf("want 3 comments, got %d", len(out))
	}
	if out[0].StartOffset != 0 || out[1].StartOffset != 10 || out[2].StartOffset != 20 {
		t.Fatalf("comments not sorted by offset: %+v", out)
	}
	if out[0].ID == out[1].ID {
		t.Fatalf("duplicate text at different offsets must get distinct ids via occurrence index")
	}
	wantFirst := hashID("a.go", "// dup", 0)
	wantSecond := hashID("a.go", "// dup", 1)
	if out[0].ID != wantFirst || out[1].ID != wantSecond {
		t.Fatalf("occurrence index not applied by byte-offset order: got %s,%s", out[0].ID, out[1].ID)
	}
}

func TestEscapeText(t *testing.T) {
	cases := map[string]string{
		"// a\nb":   "// a\\nb",
		"a\tb":      "a\\tb",
		"a\r\nb":    "a\\nb",
		"no-change": "no-change",
	}
	for in, want := range cases {
		if got := escapeText(in); got != want {
			t.Errorf("escapeText(%q) = %q, want %q", in, got, want)
		}
	}
}
