package gocomments

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func firstGroup(t *testing.T, src string) *ast.CommentGroup {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Comments) == 0 {
		t.Fatalf("no comments found in fixture")
	}
	return f.Comments[0]
}

func TestIsDirective(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"go-build-directive", "//go:build linux\npackage p\n", true},
		{"go-build-with-space-not-directive", "// go:build linux\npackage p\n", false},
		{"nolint-no-arg", "//nolint\npackage p\n", true},
		{"nolint-with-arg", "//nolint:errcheck\npackage p\n", true},
		{"nolint-with-space-not-directive", "// nolint\npackage p\n", false},
		{"plus-build", "// +build linux\npackage p\n", true},
		{"line-directive", "//line foo.go:10\npackage p\n", true},
		{"export-directive", "//export Foo\npackage p\n", true},
		{"block-comment-not-directive", "/* go:build linux */\npackage p\n", false},
		{"plain-comment", "// just a note\npackage p\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := firstGroup(t, tc.src)
			if got := isDirective(g); got != tc.want {
				t.Errorf("isDirective(%q) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestClassifyPriority(t *testing.T) {
	g := firstGroup(t, "//go:generate foo\npackage p\n")
	kind, protected, reason := Classify(g, true, true)
	if kind != KindDirective || !protected || reason != "directive" {
		t.Errorf("directive must win over doc/cgo: %v %v %v", kind, protected, reason)
	}
}

func TestClassifyDoc(t *testing.T) {
	g := firstGroup(t, "// Foo does a thing.\npackage p\n")
	kind, protected, reason := Classify(g, true, false)
	if kind != KindDoc || !protected || reason != "doc" {
		t.Errorf("doc classification wrong: %v %v %v", kind, protected, reason)
	}
}

func TestClassifyCgo(t *testing.T) {
	g := firstGroup(t, "// #include <stdio.h>\npackage p\n")
	kind, protected, reason := Classify(g, false, true)
	if kind != KindLine || !protected || reason != "cgo" {
		t.Errorf("cgo classification wrong: %v %v %v", kind, protected, reason)
	}
}

func TestClassifyUnprotected(t *testing.T) {
	g := firstGroup(t, "// just a note\npackage p\n")
	kind, protected, reason := Classify(g, false, false)
	if kind != KindLine || protected || reason != "" {
		t.Errorf("plain comment must be unprotected: %v %v %q", kind, protected, reason)
	}

	gb := firstGroup(t, "/* block note */\npackage p\n")
	kind, protected, reason = Classify(gb, false, false)
	if kind != KindBlock || protected {
		t.Errorf("plain block comment must be unprotected block: %v %v %q", kind, protected, reason)
	}
}
