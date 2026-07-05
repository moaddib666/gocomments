package gocomments

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"unicode"
)

const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
)

// safeStopwords never trip a NEG precision test on their own: articles,
// copulas, prepositions and conjunctions carry no doc-specific meaning.
var safeStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "being": true, "of": true, "to": true, "in": true, "on": true,
	"for": true, "and": true, "or": true, "this": true, "that": true, "it": true, "its": true,
	"what": true, "which": true, "who": true, "whose": true, "when": true, "where": true,
	"how": true, "here": true, "there": true, "as": true, "by": true, "with": true, "from": true,
	"into": true, "at": true, "actually": true, "simply": true, "just": true,
	"also": true, "only": true, "per": true, "via": true, "then": true,
}

// riskyStopwords are generic doc verbs (used/use/new/set/get/store/provide
// and their inflections). Adding a word here requires a matching NEG
// precision test (see Test Plan rows 19-21): a word here can mask a
// qualifier-adding doc as a restated one, so each addition needs proof it
// doesn't over-trigger.
var riskyStopwords = map[string]bool{
	"used": true, "use": true, "uses": true, "returns": true, "return": true,
	"holds": true, "hold": true, "contains": true, "contain": true, "represents": true, "represent": true,
	"stores": true, "store": true, "provides": true, "provide": true, "sets": true, "set": true,
	"gets": true, "get": true, "new": true,
}

func isStopword(tok string) bool {
	return safeStopwords[tok] || riskyStopwords[tok]
}

var tokenRe = regexp.MustCompile(`[A-Za-z0-9]+`)

type heuristic func(Comment) (reason string, confidence string, junk bool)

// Reasons is the canonical, precedence-ordered list of junk reasons. Adding a
// heuristic means appending its reason here and to heuristics, not editing
// AuditComment.
var Reasons = []string{"restates-name", "commented-code", "divider", "low-value"}

var heuristics = []heuristic{restatesName, commentedCode, divider, lowValue}

// AuditComment evaluates the junk heuristics in precedence order and returns
// the first match. Directives are never junk. Precision over recall: an
// uncertain heuristic must return junk=false rather than guess.
func AuditComment(c Comment) (reason string, confidence string, junk bool) {
	if c.Kind == KindDirective {
		return "", "", false
	}
	for _, h := range heuristics {
		if reason, confidence, junk = h(c); junk {
			return reason, confidence, junk
		}
	}
	return "", "", false
}

func restatesName(c Comment) (string, string, bool) {
	if c.DeclName == "" {
		return "", "", false
	}
	identTokens := map[string]bool{}
	identTokens[trimTrailingS(strings.ToLower(c.DeclName))] = true
	for _, w := range splitCamel(c.DeclName) {
		identTokens[trimTrailingS(w)] = true
	}
	content := contentTokens(commentBody(c))
	if len(content) == 0 {
		return "", "", false
	}
	for _, tok := range content {
		if !identTokens[trimTrailingS(tok)] {
			return "", "", false
		}
	}
	confidence := ConfidenceHigh
	if len(content) > 6 {
		confidence = ConfidenceMedium
	}
	return "restates-name", confidence, true
}

func commentedCode(c Comment) (string, string, bool) {
	if c.Trailing {
		return "", "", false
	}
	body := commentBody(c)
	fields := strings.Fields(body)
	if len(fields) < 2 {
		return "", "", false
	}
	if hasSentenceShape(fields) {
		return "", "", false
	}
	switch codeParseKind(body) {
	case codeParseStatement:
		if isLegendAssign(body) {
			return "", "", false
		}
		return "commented-code", ConfidenceHigh, true
	case codeParseExprOnly:
		return "commented-code", ConfidenceMedium, true
	default:
		return "", "", false
	}
}

// isLegendAssign reports whether body parses as a plain `=` assignment
// (never `:=`) whose left- or right-hand side is entirely bare literals —
// e.g. `0 = unlimited` (literal LHS) or `MaxDialogueCount = 2` (literal
// RHS): a legend shape, not an addressable statement that could ever have
// been real code.
func isLegendAssign(body string) bool {
	src := "package p\nfunc _() {\n" + body + "\n}\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return false
	}
	fn := f.Decls[0].(*ast.FuncDecl)
	if len(fn.Body.List) != 1 {
		return false
	}
	assign, ok := fn.Body.List[0].(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN {
		return false
	}
	return allBasicLit(assign.Lhs) || allBasicLit(assign.Rhs)
}

func allBasicLit(exprs []ast.Expr) bool {
	for _, e := range exprs {
		if _, ok := e.(*ast.BasicLit); !ok {
			return false
		}
	}
	return true
}

func divider(c Comment) (string, string, bool) {
	body := commentBody(c)
	if body == "" {
		return "", "", false
	}
	if stripDividerChars(body) == "" {
		return "divider", ConfidenceHigh, true
	}
	return "", "", false
}

func lowValue(c Comment) (string, string, bool) {
	if c.Protected {
		return "", "", false
	}
	if len(contentTokens(commentBody(c))) == 1 {
		return "low-value", ConfidenceMedium, true
	}
	return "", "", false
}

func commentBody(c Comment) string {
	raw := unescapeText(c.Text)
	if c.Kind == KindBlock {
		return stripBlockMarkers(raw)
	}
	return stripLineMarkers(raw)
}

func unescapeText(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	return s
}

func stripLineMarkers(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, l := range lines {
		l = strings.TrimSpace(l)
		l = strings.TrimPrefix(l, "//")
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func stripBlockMarkers(raw string) string {
	raw = strings.TrimPrefix(raw, "/*")
	raw = strings.TrimSuffix(raw, "*/")
	return strings.TrimSpace(raw)
}

func contentTokens(body string) []string {
	var content []string
	for _, tok := range tokenRe.FindAllString(strings.ToLower(body), -1) {
		if isStopword(tok) {
			continue
		}
		content = append(content, tok)
	}
	return content
}

func trimTrailingS(s string) string {
	if len(s) > 1 && strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}

func splitCamel(name string) []string {
	runes := []rune(name)
	var words []string
	var cur []rune
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prevLower := unicode.IsLower(runes[i-1])
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevLower || nextLower {
				words = append(words, strings.ToLower(string(cur)))
				cur = nil
			}
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		words = append(words, strings.ToLower(string(cur)))
	}
	return words
}

func hasSentenceShape(fields []string) bool {
	run := 0
	for _, w := range fields {
		endsPeriod := strings.HasSuffix(w, ".")
		bare := w
		if endsPeriod {
			bare = strings.TrimSuffix(w, ".")
		}
		if bare != "" && isAllAlpha(bare) {
			run++
			if run >= 3 && endsPeriod {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

func isAllAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

type codeParseResult int

const (
	codeParseNone codeParseResult = iota
	codeParseStatement
	codeParseExprOnly
)

// codeParseKind classifies a comment body's parse shape. A body that parses
// as a full non-expression statement (assignment, return, control flow, ...)
// is the confident case. A body that only yields a single bare expression —
// whether it parses via the full func(){ body } path as a lone ExprStmt or
// only via parser.ParseExpr — is the riskier case: prose fragments can
// coincidentally parse as expressions.
func codeParseKind(body string) codeParseResult {
	src := "package p\nfunc _() {\n" + body + "\n}\n"
	fset := token.NewFileSet()
	if f, err := parser.ParseFile(fset, "", src, 0); err == nil {
		fn := f.Decls[0].(*ast.FuncDecl)
		stmts := fn.Body.List
		if len(stmts) == 1 {
			if _, isExpr := stmts[0].(*ast.ExprStmt); isExpr {
				return codeParseExprOnly
			}
		}
		return codeParseStatement
	}
	if _, err := parser.ParseExpr(body); err == nil {
		return codeParseExprOnly
	}
	return codeParseNone
}

func stripDividerChars(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '-', '=', '*', '#', '_', '/', ' ', '\t', '\n':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
