package cli

import (
	"strings"
	"testing"
)

func TestCollectIDs_MergesAllSources(t *testing.T) {
	got, err := CollectIDs([]string{"a", "b"}, "c,d", true, strings.NewReader("e\nf\n\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b", "c", "d", "e", "f"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCollectIDs_NoneGivenIsError(t *testing.T) {
	_, err := CollectIDs(nil, "", false, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error when no ids supplied")
	}
}

func TestCollectIDs_StdinSkipsBlankLines(t *testing.T) {
	got, err := CollectIDs(nil, "", true, strings.NewReader("a\n\n  \nb\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("blank lines should be skipped, got %v", got)
	}
}

func TestCollectIDs_IdsCSVTrimsWhitespace(t *testing.T) {
	got, err := CollectIDs(nil, " a , b ,,c", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
