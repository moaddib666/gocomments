package gocomments

import "testing"

func TestAuditComment_RestatesName(t *testing.T) {
	cases := []struct {
		name       string
		declName   string
		text       string
		wantJunk   bool
		wantReason string
	}{
		{"real target Expected", "Expected", "// Expected is what was expected.", true, "restates-name"},
		{"real target Observed", "Observed", "// Observed is what was actually observed.", true, "restates-name"},
		{"camelCase UserID", "UserID", "// UserID is the user id.", true, "restates-name"},
		{"plural Items", "Items", "// Items are the items.", true, "restates-name"},
		{"NEG qualifier doc Key", "Key", "// Key is the raw Ed25519 public key", false, ""},
		{"NEG design ref DESIGN-001", "Foo", "// see DESIGN-001", false, ""},
		{"NEG design ref NFR-SEC-3", "Foo", "// see NFR-SEC-3", false, ""},
		{"NEG design ref Invariant 72", "Foo", "// Invariant 72", false, ""},
		{"NEG timeout qualifier", "Timeout", "// Timeout is 30s by default", false, ""},
		{"NEG no decl name", "", "// Foo is what Foo is.", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Comment{DeclName: tc.declName, Text: tc.text, Kind: KindDoc, Protected: true}
			reason, _, junk := AuditComment(c)
			if junk != tc.wantJunk {
				t.Fatalf("AuditComment(%+v) junk = %v, want %v (reason=%q)", c, junk, tc.wantJunk, reason)
			}
			if junk && reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

func TestAuditComment_RestatesNameConfidenceTiers(t *testing.T) {
	c := Comment{DeclName: "Expected", Text: "// Expected is what was expected.", Kind: KindDoc, Protected: true}
	_, confidence, junk := AuditComment(c)
	if !junk || confidence != ConfidenceHigh {
		t.Fatalf("want high confidence for short restated doc, got junk=%v confidence=%q", junk, confidence)
	}
}

func TestAuditComment_CommentedCode(t *testing.T) {
	cases := []struct {
		name           string
		text           string
		kind           Kind
		trailing       bool
		wantJunk       bool
		wantConfidence string
	}{
		{"assignment call", "// x := f(a,b)", KindLine, false, true, ConfidenceHigh},
		{"block return statement", "/* return err */", KindBlock, false, true, ConfidenceHigh},
		{"bare expression prose-risky", "// db.Query(sql, args)", KindLine, false, true, ConfidenceMedium},
		{"prose with function call", "// Example: F(1,2) returns 3", KindLine, false, false, ""},
		{"sentence prose", "// This function does something useful here.", KindLine, false, false, ""},
		{"empty body no panic", "//", KindLine, false, false, ""},
		{"standalone literal-LHS legend", "// 0 = unlimited", KindLine, false, false, ""},
		{"trailing legend zero unlimited", "// 0 = unlimited", KindLine, true, false, ""},
		{"trailing legend const assign", "// MaxDialogueCount = 2", KindLine, true, false, ""},
		{"trailing legend port strip", "// strip :port", KindLine, true, false, ""},
		{"trailing assignment call still not flagged", "// x := f(a,b)", KindLine, true, false, ""},
		{"standalone literal-RHS legend identifier LHS", "// MaxDialogueCount = 2", KindLine, false, false, ""},
		{"standalone literal-RHS legend selector LHS", "// config.Timeout = 30", KindLine, false, false, ""},
		{"standalone assign with call RHS still flagged", "// x = compute()", KindLine, false, true, ConfidenceHigh},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Comment{Text: tc.text, Kind: tc.kind, Trailing: tc.trailing}
			reason, confidence, junk := AuditComment(c)
			if junk != tc.wantJunk {
				t.Fatalf("AuditComment(%q trailing=%v) junk = %v, want %v (reason=%q)", tc.text, tc.trailing, junk, tc.wantJunk, reason)
			}
			if junk && reason != "commented-code" {
				t.Errorf("reason = %q, want commented-code", reason)
			}
			if junk && confidence != tc.wantConfidence {
				t.Errorf("confidence = %q, want %q", confidence, tc.wantConfidence)
			}
		})
	}
}

// divider is tested directly (unit-level): a short label like
// "===== Section =====" is not a divider but does trip the separate
// low-value heuristic, so it is exercised in isolation here rather than via
// the full AuditComment precedence chain.
func TestDivider(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		wantJunk bool
	}{
		{"rule only dashes", "// ----------", true},
		{"underscores", "// ____", true},
		{"hashes", "// #####", true},
		{"asterisk spaced", "// * * *", true},
		{"NEG labeled rule", "// ===== Section =====", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Comment{Text: tc.text, Kind: KindLine}
			reason, _, junk := divider(c)
			if junk != tc.wantJunk {
				t.Fatalf("divider(%q) junk = %v, want %v (reason=%q)", tc.text, junk, tc.wantJunk, reason)
			}
			if junk && reason != "divider" {
				t.Errorf("reason = %q, want divider", reason)
			}
		})
	}
}

func TestLowValue(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		protected bool
		wantJunk  bool
	}{
		{"single token unprotected", "// noop", false, true},
		{"single token protected not low-value", "// noop", true, false},
		{"two content tokens not low-value", "// keep note", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Comment{Text: tc.text, Kind: KindLine, Protected: tc.protected}
			reason, _, junk := lowValue(c)
			if junk != tc.wantJunk {
				t.Fatalf("lowValue(%q protected=%v) junk = %v, want %v (reason=%q)", tc.text, tc.protected, junk, tc.wantJunk, reason)
			}
			if junk && reason != "low-value" {
				t.Errorf("reason = %q, want low-value", reason)
			}
		})
	}
}

func TestAuditComment_DirectivesNeverJunk(t *testing.T) {
	c := Comment{Text: "//go:generate mockgen -source=a.go", Kind: KindDirective, Protected: true, Reason: "directive"}
	if _, _, junk := AuditComment(c); junk {
		t.Fatalf("directive must never be flagged as junk")
	}
}

func TestAuditComment_ProtectedDocRestatingNameStillFlagged(t *testing.T) {
	c := Comment{DeclName: "Expected", Text: "// Expected is what was expected.", Kind: KindDoc, Protected: true, Reason: "doc"}
	reason, _, junk := AuditComment(c)
	if !junk || reason != "restates-name" {
		t.Fatalf("protected doc restating its name must still be flagged, got junk=%v reason=%q", junk, reason)
	}
}

func TestClassifyFile_IdentifierAssociation(t *testing.T) {
	src := `package p

type S struct {
	// Expected is what was expected.
	Expected string
	// Key is the raw Ed25519 public key
	Key string
}

type I interface {
	// Run executes the operation.
	Run() error
}

// Foo does a thing.
func Foo() {}
`
	dir := t.TempDir()
	writeFile(t, dir, "a.go", src)
	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	byText := map[string]Comment{}
	for _, c := range comments {
		byText[c.Text] = c
	}

	expected, ok := byText["// Expected is what was expected."]
	if !ok || expected.DeclName != "Expected" || expected.Kind != KindDoc || !expected.Protected {
		t.Errorf("struct field doc association wrong: %+v", expected)
	}
	key, ok := byText["// Key is the raw Ed25519 public key"]
	if !ok || key.DeclName != "Key" || key.Kind != KindDoc || !key.Protected {
		t.Errorf("struct field doc association wrong: %+v", key)
	}
	run, ok := byText["// Run executes the operation."]
	if !ok || run.DeclName != "Run" || run.Kind != KindDoc || !run.Protected {
		t.Errorf("interface method doc association wrong: %+v", run)
	}
	foo, ok := byText["// Foo does a thing."]
	if !ok || foo.DeclName != "Foo" || foo.Kind != KindDoc || !foo.Protected {
		t.Errorf("func doc association regressed: %+v", foo)
	}
}

func TestAuditComment_RealTargetsViaFullPipeline(t *testing.T) {
	src := `package p

type Result struct {
	// Expected is what was expected.
	Expected string
	// Observed is what was actually observed.
	Observed string
	// Key is the raw Ed25519 public key
	Key string
}
`
	dir := t.TempDir()
	writeFile(t, dir, "a.go", src)
	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	byText := map[string]Comment{}
	for _, c := range comments {
		byText[c.Text] = c
	}

	for _, text := range []string{"// Expected is what was expected.", "// Observed is what was actually observed."} {
		c, ok := byText[text]
		if !ok {
			t.Fatalf("missing fixture comment %q", text)
		}
		reason, _, junk := AuditComment(c)
		if !junk || reason != "restates-name" {
			t.Errorf("%q: want junk=true reason=restates-name, got junk=%v reason=%q", text, junk, reason)
		}
	}

	key, ok := byText["// Key is the raw Ed25519 public key"]
	if !ok {
		t.Fatalf("missing Key fixture comment")
	}
	if _, _, junk := AuditComment(key); junk {
		t.Errorf("qualifier-adding doc on Key must not be flagged as junk")
	}
}

func TestClassifyGenDecl_MultiSpecGroupDocSuppressesNameButStaysProtected(t *testing.T) {
	src := `package p

// LLM events.
const (
	EventLLMRequestStarted   = "llm.request.started"
	EventLLMRequestCompleted = "llm.request.completed"
)

// Idempotency outcomes.
const (
	OutcomeSkipped = "skipped"
	OutcomeApplied = "applied"
)

// Foo is the foo
const Foo = 1
`
	dir := t.TempDir()
	writeFile(t, dir, "a.go", src)
	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	byText := map[string]Comment{}
	for _, c := range comments {
		byText[c.Text] = c
	}

	for _, text := range []string{"// LLM events.", "// Idempotency outcomes."} {
		c, ok := byText[text]
		if !ok {
			t.Fatalf("missing fixture comment %q", text)
		}
		if c.DeclName != "" {
			t.Errorf("%q: DeclName should be suppressed for multi-spec group doc, got %q", text, c.DeclName)
		}
		if !c.Protected || c.Kind != KindDoc {
			t.Errorf("%q: multi-spec group doc must remain Protected doc, got Protected=%v Kind=%v", text, c.Protected, c.Kind)
		}
		if reason, _, junk := AuditComment(c); junk {
			t.Errorf("%q: must not be flagged as junk, got reason=%q", text, reason)
		}
	}

	foo, ok := byText["// Foo is the foo"]
	if !ok {
		t.Fatalf("missing Foo fixture comment")
	}
	if foo.DeclName != "Foo" {
		t.Errorf("single-spec const doc must keep DeclName, got %q", foo.DeclName)
	}
	if reason, _, junk := AuditComment(foo); !junk || reason != "restates-name" {
		t.Errorf("single-spec const doc restating its name must still be flagged, got junk=%v reason=%q", junk, reason)
	}
}

func TestClassifyGenDecl_MultiSpecDocRestatesFirstSpecName(t *testing.T) {
	src := `package p

// UserID is the user id
var (
	UserID string
	tmp    int
)

// LLM events.
const (
	EventLLMRequestStarted   = "llm.request.started"
	EventLLMRequestCompleted = "llm.request.completed"
)
`
	dir := t.TempDir()
	writeFile(t, dir, "a.go", src)
	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	byText := map[string]Comment{}
	for _, c := range comments {
		byText[c.Text] = c
	}

	userID, ok := byText["// UserID is the user id"]
	if !ok {
		t.Fatalf("missing UserID fixture comment")
	}
	if userID.DeclName != "UserID" {
		t.Errorf("multi-spec doc restating first spec name must associate DeclName, got %q", userID.DeclName)
	}
	if !userID.Protected || userID.Kind != KindDoc {
		t.Errorf("multi-spec doc must remain Protected doc, got Protected=%v Kind=%v", userID.Protected, userID.Kind)
	}
	if reason, _, junk := AuditComment(userID); !junk || reason != "restates-name" {
		t.Errorf("first-spec restatement must be flagged, got junk=%v reason=%q", junk, reason)
	}

	llm, ok := byText["// LLM events."]
	if !ok {
		t.Fatalf("missing LLM fixture comment")
	}
	if llm.DeclName != "" {
		t.Errorf("group header doc must stay suppressed when first token != first spec name, got %q", llm.DeclName)
	}
	if !llm.Protected || llm.Kind != KindDoc {
		t.Errorf("group header doc must remain Protected doc, got Protected=%v Kind=%v", llm.Protected, llm.Kind)
	}
	if reason, _, junk := AuditComment(llm); junk {
		t.Errorf("group header doc must not be flagged, got reason=%q", reason)
	}
}

func TestAuditComment_RestatesNameFuncAndInterfaceMethod(t *testing.T) {
	src := `package p

// Reset resets.
func Reset() {}

type I interface {
	// Close closes.
	Close() error
}
`
	dir := t.TempDir()
	writeFile(t, dir, "a.go", src)
	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	byText := map[string]Comment{}
	for _, c := range comments {
		byText[c.Text] = c
	}

	reset, ok := byText["// Reset resets."]
	if !ok {
		t.Fatalf("missing Reset fixture comment")
	}
	if reason, _, junk := AuditComment(reset); !junk || reason != "restates-name" {
		t.Errorf("func Reset restating its name: want junk=true reason=restates-name, got junk=%v reason=%q", junk, reason)
	}

	closeC, ok := byText["// Close closes."]
	if !ok {
		t.Fatalf("missing Close fixture comment")
	}
	if reason, _, junk := AuditComment(closeC); !junk || reason != "restates-name" {
		t.Errorf("interface method Close restating its name: want junk=true reason=restates-name, got junk=%v reason=%q", junk, reason)
	}
}

func TestClassifyFile_TrailingDetection(t *testing.T) {
	src := `package p

// standalone doc
func F() {
	x := 1 // shares line with code
	_ = x
}

/*
block trailing
*/
var _ = 1

var z = 1 /* trailing block */

// 0 = unlimited
var y int
`
	dir := t.TempDir()
	writeFile(t, dir, "a.go", src)
	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	byText := map[string]Comment{}
	for _, c := range comments {
		byText[c.Text] = c
	}

	standalone, ok := byText["// standalone doc"]
	if !ok || standalone.Trailing {
		t.Errorf("standalone leading comment must not be Trailing: %+v", standalone)
	}
	trailing, ok := byText["// shares line with code"]
	if !ok || !trailing.Trailing {
		t.Errorf("comment sharing its line with code must be Trailing: %+v", trailing)
	}
	blockTrailing, ok := byText["/*\\nblock trailing\\n*/"]
	if !ok || blockTrailing.Trailing {
		t.Errorf("standalone block comment on its own line must not be Trailing: %+v", blockTrailing)
	}
	blockShareLine, ok := byText["/* trailing block */"]
	if !ok || !blockShareLine.Trailing {
		t.Errorf("block comment sharing its line with code must be Trailing: %+v", blockShareLine)
	}
	standaloneLegend, ok := byText["// 0 = unlimited"]
	if !ok || standaloneLegend.Trailing {
		t.Errorf("standalone legend comment on its own line must not be Trailing: %+v", standaloneLegend)
	}
}

func TestAuditComment_TrailingLegendNotFlaggedViaFullPipeline(t *testing.T) {
	src := `package p

func f(maxTokens int) {
	_ = maxTokens // 0 = unlimited
}
`
	dir := t.TempDir()
	writeFile(t, dir, "a.go", src)
	comments, errs := ScanPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	byText := map[string]Comment{}
	for _, c := range comments {
		byText[c.Text] = c
	}
	c, ok := byText["// 0 = unlimited"]
	if !ok {
		t.Fatalf("missing fixture comment")
	}
	if !c.Trailing {
		t.Fatalf("expected trailing comment, got %+v", c)
	}
	if reason, _, junk := AuditComment(c); junk {
		t.Errorf("trailing legend comment must not be flagged, got reason=%q", reason)
	}
}
