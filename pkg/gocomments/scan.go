package gocomments

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ScanPath(path string) ([]Comment, []error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, []error{err}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, []error{err}
	}
	if info.IsDir() {
		return scanTree(abs)
	}
	return scanSingleFile(abs)
}

func scanTree(root string) ([]Comment, []error) {
	root = canonicalPath(root)
	base := resolveScanBase(root, true)
	files, err := walkGoFiles(root)
	if err != nil {
		return nil, []error{err}
	}
	var entries []rawEntry
	var errs []error
	for _, f := range files {
		es, ferr := scanFileEntries(f, base)
		if ferr != nil {
			errs = append(errs, ferr)
			continue
		}
		entries = append(entries, es...)
	}
	return computeIDs(entries), errs
}

func scanSingleFile(path string) ([]Comment, []error) {
	path = canonicalPath(path)
	base := resolveScanBase(path, false)
	entries, err := scanFileEntries(path, base)
	if err != nil {
		return nil, []error{err}
	}
	return computeIDs(entries), nil
}

func walkGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// canonicalPath resolves symlinks in path (e.g. macOS's /var -> /private/var
// TempDir aliasing) so that walked file paths and the git-toplevel-derived
// rel base agree on the same absolute prefix; falls back to path unchanged
// when it can't be resolved (doesn't exist yet, permissions, etc).
func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// resolveScanBase resolves the id/relPath base: the git toplevel of the
// target when it is inside a work tree, otherwise the scan root itself (or
// the containing directory when path is a single file). All scan modes
// (tree, single-file, git-scoped) must agree on this base so the same
// comment produces the same id no matter how it was reached.
func resolveScanBase(path string, isDir bool) string {
	dir := path
	if !isDir {
		dir = filepath.Dir(path)
	}
	if top, err := gitToplevel(dir); err == nil && top != "" {
		return top
	}
	return dir
}

func gitToplevel(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func scanFileEntries(path, root string) ([]rawEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	return scanBytesEntries(path, content, rel)
}

// ScanBytes parses in-memory source (e.g. from `git show <sha>:<file>`) instead
// of reading from disk, so historical file versions can be inventoried too.
func ScanBytes(displayPath string, content []byte, relPath string) ([]Comment, error) {
	entries, err := scanBytesEntries(displayPath, content, relPath)
	if err != nil {
		return nil, err
	}
	return computeIDs(entries), nil
}

func scanBytesEntries(displayPath string, content []byte, rel string) ([]rawEntry, error) {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, displayPath, content, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return classifyFile(fset, astFile, content, displayPath, rel), nil
}

func classifyFile(fset *token.FileSet, f *ast.File, content []byte, absPath, rel string) []rawEntry {
	nameGroups, docGroups, cgoGroups := associateDeclComments(f)
	return buildRawEntries(fset, f, content, absPath, rel, nameGroups, docGroups, cgoGroups)
}

// associateDeclComments walks the file once, mapping each doc/name-bearing
// comment group to the identifier it documents (nameGroups), whether it is a
// protected doc comment (docGroups), and whether it is a cgo `import "C"`
// preamble (cgoGroups).
func associateDeclComments(f *ast.File) (nameGroups map[*ast.CommentGroup]string, docGroups, cgoGroups map[*ast.CommentGroup]bool) {
	nameGroups = map[*ast.CommentGroup]string{}
	docGroups = map[*ast.CommentGroup]bool{}
	cgoGroups = map[*ast.CommentGroup]bool{}

	ast.Inspect(f, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.GenDecl:
			classifyGenDecl(d, nameGroups, docGroups, cgoGroups)
		case *ast.FuncDecl:
			if d.Doc != nil && d.Name.IsExported() {
				nameGroups[d.Doc] = d.Name.Name
				docGroups[d.Doc] = true
			}
		case *ast.StructType:
			classifyFieldList(d.Fields, nameGroups, docGroups)
		case *ast.InterfaceType:
			classifyFieldList(d.Methods, nameGroups, docGroups)
		}
		return true
	})
	return nameGroups, docGroups, cgoGroups
}

func buildRawEntries(fset *token.FileSet, f *ast.File, content []byte, absPath, rel string, nameGroups map[*ast.CommentGroup]string, docGroups, cgoGroups map[*ast.CommentGroup]bool) []rawEntry {
	var entries []rawEntry
	for _, g := range f.Comments {
		startPos := fset.Position(g.Pos())
		endPos := fset.Position(g.End())
		text := string(content[startPos.Offset:endPos.Offset])
		kind, protected, reason := Classify(g, docGroups[g], cgoGroups[g])
		entries = append(entries, rawEntry{
			file:        rel,
			absPath:     absPath,
			startOffset: startPos.Offset,
			endOffset:   endPos.Offset,
			line:        startPos.Line,
			endLine:     endPos.Line,
			rawText:     text,
			kind:        kind,
			protected:   protected,
			reason:      reason,
			declName:    nameGroups[g],
			trailing:    isTrailingComment(fset, g, content),
		})
	}
	return entries
}

// isTrailingComment reports whether the comment group shares its source line
// with preceding code: it shares a line with code when the bytes between the
// start of that line (per the FileSet's line table) and the comment's start
// offset are not all whitespace.
func isTrailingComment(fset *token.FileSet, g *ast.CommentGroup, content []byte) bool {
	startPos := fset.Position(g.Pos())
	lineStart := fset.Position(fset.File(g.Pos()).LineStart(startPos.Line)).Offset
	return len(bytes.TrimSpace(content[lineStart:startPos.Offset])) > 0
}

// classifyFieldList associates each exported struct field or interface
// method's Doc comment with its identifier name (embedded fields/interfaces
// with no Names are left unassociated).
func classifyFieldList(fl *ast.FieldList, nameGroups map[*ast.CommentGroup]string, docGroups map[*ast.CommentGroup]bool) {
	if fl == nil {
		return
	}
	for _, field := range fl.List {
		if field.Doc == nil || len(field.Names) == 0 {
			continue
		}
		if name := field.Names[0]; name.IsExported() {
			nameGroups[field.Doc] = name.Name
			docGroups[field.Doc] = true
		}
	}
}

// classifyGenDecl associates names for audit-restatement checks and marks
// protected doc groups.
func classifyGenDecl(d *ast.GenDecl, nameGroups map[*ast.CommentGroup]string, docGroups map[*ast.CommentGroup]bool, cgoGroups map[*ast.CommentGroup]bool) {
	classifyGenDeclDoc(d, nameGroups, docGroups, cgoGroups)
	classifyGenDeclSpecs(d, nameGroups, docGroups, cgoGroups)
}

// classifyGenDeclDoc handles the GenDecl's own leading doc comment. A d.Doc
// heading a multi-spec group (e.g. a `const ( ... )` block) usually
// documents the group rather than one identifier, so it is marked as a
// protected doc without a DeclName association — unless the doc's first
// token restates the first spec's name, in which case it is a genuine
// restatement of that first spec (e.g. `// UserID is the user id` heading
// `var ( UserID string; ... )`) and is associated with it.
func classifyGenDeclDoc(d *ast.GenDecl, nameGroups map[*ast.CommentGroup]string, docGroups map[*ast.CommentGroup]bool, cgoGroups map[*ast.CommentGroup]bool) {
	if d.Doc == nil {
		return
	}
	if name, ok := firstExportedDeclName(d); ok {
		docGroups[d.Doc] = true
		switch {
		case len(d.Specs) == 1:
			nameGroups[d.Doc] = name
		default:
			if firstName, ok := firstSpecName(d); ok && restatesFirstDocToken(d.Doc, firstName) {
				nameGroups[d.Doc] = firstName
			}
		}
	}
	if d.Tok == token.IMPORT {
		for _, spec := range d.Specs {
			if isImportC(spec) {
				cgoGroups[d.Doc] = true
			}
		}
	}
}

func classifyGenDeclSpecs(d *ast.GenDecl, nameGroups map[*ast.CommentGroup]string, docGroups map[*ast.CommentGroup]bool, cgoGroups map[*ast.CommentGroup]bool) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if s.Doc != nil && s.Name.IsExported() {
				nameGroups[s.Doc] = s.Name.Name
				docGroups[s.Doc] = true
			}
		case *ast.ValueSpec:
			if s.Doc != nil {
				for _, nm := range s.Names {
					if nm.IsExported() {
						nameGroups[s.Doc] = nm.Name
						docGroups[s.Doc] = true
						break
					}
				}
			}
		case *ast.ImportSpec:
			if s.Doc != nil && isImportC(s) {
				cgoGroups[s.Doc] = true
			}
		}
	}
}

// firstSpecName returns the name of the first spec in a GenDecl (its type
// name, or its first identifier for a value spec), regardless of export
// status.
func firstSpecName(d *ast.GenDecl) (string, bool) {
	if len(d.Specs) == 0 {
		return "", false
	}
	switch s := d.Specs[0].(type) {
	case *ast.TypeSpec:
		return s.Name.Name, true
	case *ast.ValueSpec:
		if len(s.Names) > 0 {
			return s.Names[0].Name, true
		}
	}
	return "", false
}

// restatesFirstDocToken reports whether doc's first alphanumeric token
// matches name case-insensitively, i.e. the doc opens by naming it.
func restatesFirstDocToken(doc *ast.CommentGroup, name string) bool {
	tok := tokenRe.FindString(doc.Text())
	return tok != "" && strings.EqualFold(tok, name)
}

func firstExportedDeclName(d *ast.GenDecl) (string, bool) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if s.Name.IsExported() {
				return s.Name.Name, true
			}
		case *ast.ValueSpec:
			for _, nm := range s.Names {
				if nm.IsExported() {
					return nm.Name, true
				}
			}
		}
	}
	return "", false
}

func isImportC(spec ast.Spec) bool {
	s, ok := spec.(*ast.ImportSpec)
	return ok && s.Path != nil && s.Path.Value == `"C"`
}
