package cli

import "testing"

func TestParseHunkHeader(t *testing.T) {
	cases := []struct {
		line      string
		wantStart int
		wantCount int
		wantOK    bool
	}{
		{"@@ -1,3 +1,5 @@", 1, 5, true},
		{"@@ -0,0 +1 @@", 1, 1, true},
		{"@@ -5,2 +5,0 @@", 5, 0, true},
		{"@@ -1 +1 @@", 1, 1, true},
		{"not a hunk header", 0, 0, false},
		{"@@ -1,3 +1,5 @@ func foo() {", 1, 5, true},
	}
	for _, tc := range cases {
		start, count, ok := ParseHunkHeader(tc.line)
		if ok != tc.wantOK {
			t.Errorf("ParseHunkHeader(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if start != tc.wantStart || count != tc.wantCount {
			t.Errorf("ParseHunkHeader(%q) = (%d,%d), want (%d,%d)", tc.line, start, count, tc.wantStart, tc.wantCount)
		}
	}
}

func TestParseAddedRanges_MultiHunkAndZeroCount(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
index 111..222 100644
--- a/foo.go
+++ b/foo.go
@@ -1,0 +2,2 @@
@@ -10,2 +12,0 @@
@@ -20 +18,3 @@
`
	ranges := ParseAddedRanges(diff)
	got := ranges["foo.go"]
	if len(got) != 2 {
		t.Fatalf("want 2 added ranges (zero-count hunk excluded), got %v", got)
	}
	if got[0] != (LineRange{Start: 2, End: 3}) {
		t.Errorf("first range wrong: %+v", got[0])
	}
	if got[1] != (LineRange{Start: 18, End: 20}) {
		t.Errorf("second range wrong: %+v", got[1])
	}
}

func TestParseAddedRanges_MultiFile(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,0 +1,1 @@
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,0 +1,2 @@
`
	ranges := ParseAddedRanges(diff)
	if len(ranges["a.go"]) != 1 || len(ranges["b.go"]) != 1 {
		t.Fatalf("expected ranges tracked per file: %v", ranges)
	}
}

func TestParseAddedRanges_DevNullSkipped(t *testing.T) {
	diff := `diff --git a/gone.go b/gone.go
--- a/gone.go
+++ /dev/null
@@ -1,3 +0,0 @@
`
	ranges := ParseAddedRanges(diff)
	if len(ranges) != 0 {
		t.Fatalf("deleted file must contribute no added ranges: %v", ranges)
	}
}
