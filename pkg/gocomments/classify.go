package gocomments

import (
	"go/ast"
	"strings"
)

func Classify(g *ast.CommentGroup, isExportedDoc, precedesImportC bool) (Kind, bool, string) {
	if isDirective(g) {
		return KindDirective, true, "directive"
	}
	if isExportedDoc {
		return KindDoc, true, "doc"
	}
	kind := KindLine
	if groupIsBlock(g) {
		kind = KindBlock
	}
	if precedesImportC {
		return kind, true, "cgo"
	}
	return kind, false, ""
}

func isDirective(g *ast.CommentGroup) bool {
	for _, c := range g.List {
		t := c.Text
		switch {
		case strings.HasPrefix(t, "//go:") && len(t) > len("//go:") && isLowerASCII(t[len("//go:")]):
			return true
		case strings.HasPrefix(t, "//line "):
			return true
		case strings.HasPrefix(t, "//nolint"):
			return true
		case strings.HasPrefix(t, "//export "):
			return true
		case strings.HasPrefix(t, "// +build"):
			return true
		}
	}
	return false
}

func isLowerASCII(b byte) bool { return b >= 'a' && b <= 'z' }

func groupIsBlock(g *ast.CommentGroup) bool {
	return len(g.List) > 0 && strings.HasPrefix(g.List[0].Text, "/*")
}
