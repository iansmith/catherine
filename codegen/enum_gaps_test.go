package codegen_test

// Adversary gap tests added after Phase 0 review.

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// ---------------------------------------------------------------------------
// Gap 1a: nil / empty idlValues slice (boundary)
// ---------------------------------------------------------------------------

func TestEnumDeclEmptyValueSlice(t *testing.T) {
	t.Parallel()
	// An empty value slice is degenerate but must not panic.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Empty", []string{}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil for zero-value enum")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	_, err := f.Render()
	// We do not assert err==nil; an empty enum may be legitimately rejected.
	// We assert only that it does not panic.
	_ = err
}

func TestEnumDeclNilValueSlice(t *testing.T) {
	t.Parallel()
	// nil idlValues must be handled the same as an empty slice — no panic.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Nil", nil, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil for nil values slice")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	_, err := f.Render()
	_ = err
}

// ---------------------------------------------------------------------------
// Gap 1b: empty-string value const name (boundary)
// ---------------------------------------------------------------------------

func TestEnumDeclEmptyStringValueDoesNotSilentlyDrop(t *testing.T) {
	t.Parallel()
	// A single empty-string value must produce exactly one const (not zero = silent drop,
	// not two = duplicate emission).
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Foo", []string{""}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	// Count lines that assign to type Foo (const entries): `<Name> Foo = `
	count := strings.Count(string(out), " Foo = ")
	if count != 1 {
		t.Errorf("expected exactly 1 const for single-value enum with \"\"; got %d in:\n%s", count, out)
	}
}

// ---------------------------------------------------------------------------
// Gap 2a: empty-string value does not silently drop from output (error path)
// ---------------------------------------------------------------------------

func TestEnumDeclEmptyStringValueConstPreserved(t *testing.T) {
	t.Parallel()
	// The const for "" must carry the string literal ""; it must not be
	// discarded or merged with another const.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Foo", []string{"a", ""}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), `= ""`) {
		t.Errorf("output = %q; const for empty-string IDL value must carry '= \"\"' literal", out)
	}
}

// ---------------------------------------------------------------------------
// Gap 2b: three-way collision — all dropped values reported (error path)
// ---------------------------------------------------------------------------

func TestEnumDeclThreeWayCollisionAllDroppedReported(t *testing.T) {
	t.Parallel()
	// "no-change", "no_change", and "no.change" all normalise to the same const
	// name (NoChange). Two values must be dropped, generating ≥ 2 error diagnostics.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("State", []string{"no-change", "no_change", "no.change"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	errs := diag.Errors()
	if len(errs) < 2 {
		t.Errorf("expected ≥2 error diagnostics for 3-way collision; got %d: %s",
			len(errs), diag.Format())
	}
	// Only one const for the colliding name must appear in the output.
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if count := strings.Count(string(out), "StateNoChange"); count != 1 {
		t.Errorf("expected exactly 1 'StateNoChange' const; got %d in:\n%s", count, out)
	}
}

// ---------------------------------------------------------------------------
// Gap 2c: slash / space in idlName itself (error path)
// ---------------------------------------------------------------------------

func TestEnumDeclTypeNameWithSlashDoesNotPanic(t *testing.T) {
	t.Parallel()
	// A slash in the IDL enum name must either be sanitised or produce a
	// diagnostic error — it must not produce invalid Go or panic.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("my/type", []string{"val"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	_, err := f.Render()
	// Acceptable: Render() may fail if the type name is unsalvageable.
	// The key requirement is no panic.
	_ = err
}

func TestEnumDeclTypeNameWithSpaceDoesNotPanic(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("my type", []string{"val"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	_, err := f.Render()
	_ = err
}

// ---------------------------------------------------------------------------
// Gap 3a: two EnumDecls in the same File (state interaction)
// ---------------------------------------------------------------------------

func TestEnumDeclTwoDeclarationsInOneFile(t *testing.T) {
	t.Parallel()
	// Two distinct EnumDecls must coexist in one File without name conflicts.
	diag := codegen.NewDiagnostics()
	d1 := codegen.NewEnumDecl("Color", []string{"red", "green"}, diag)
	d2 := codegen.NewEnumDecl("Size", []string{"small", "large"}, diag)
	if d1 == nil || d2 == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(d1)
	f.AddDecl(d2)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() with two EnumDecls error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "type Color string") {
		t.Errorf("output missing 'type Color string':\n%s", s)
	}
	if !strings.Contains(s, "type Size string") {
		t.Errorf("output missing 'type Size string':\n%s", s)
	}
	if !strings.Contains(s, "ParseColor") {
		t.Errorf("output missing ParseColor helper:\n%s", s)
	}
	if !strings.Contains(s, "ParseSize") {
		t.Errorf("output missing ParseSize helper:\n%s", s)
	}
}

// ---------------------------------------------------------------------------
// Gap 3b: pre-existing diagnostics preserved (state interaction)
// ---------------------------------------------------------------------------

func TestEnumDeclPreexistingDiagnosticsPreserved(t *testing.T) {
	t.Parallel()
	// Errors added to diag before NewEnumDecl is called must not be cleared.
	diag := codegen.NewDiagnostics()
	diag.Add("error", "pre-existing error from earlier stage")
	codegen.NewEnumDecl("Color", []string{"red"}, diag)
	errs := diag.Errors()
	if len(errs) == 0 {
		t.Fatal("diag.Errors() is empty after NewEnumDecl; pre-existing errors were lost")
	}
	if !strings.Contains(errs[0].Message, "pre-existing error") {
		t.Errorf("first error = %q; pre-existing error was dropped or reordered", errs[0].Message)
	}
}

// ---------------------------------------------------------------------------
// Gap 4a: parse helper signature — not just name presence (spec drift)
// ---------------------------------------------------------------------------

func TestEnumDeclParseHelperHasCorrectSignature(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("EndingType", []string{"transparent"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), "func ParseEndingType(s string) (EndingType, bool)") {
		t.Errorf("parse helper has wrong or missing signature; want 'func ParseEndingType(s string) (EndingType, bool)' in:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Gap 4b: collision first-wins preserves the first value's literal (spec drift)
// ---------------------------------------------------------------------------

func TestEnumDeclCollisionFirstValueLiteralPreserved(t *testing.T) {
	t.Parallel()
	// When "no-change" (first) and "no_change" (second) collide, the surviving
	// const must carry the literal "no-change", not "no_change".
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("State", []string{"no-change", "no_change"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"no-change"`) {
		t.Errorf("surviving const must carry first value's literal \"no-change\"; got:\n%s", s)
	}
	if strings.Contains(s, `"no_change"`) {
		t.Errorf("dropped second value's literal \"no_change\" must not appear; got:\n%s", s)
	}
}

// ---------------------------------------------------------------------------
// Gap 5a: slash in value → const name (false negative fix)
// ---------------------------------------------------------------------------

func TestEnumDeclValueWithSlashConstName(t *testing.T) {
	t.Parallel()
	// "text/plain" → slash normalised to "_" → "text_plain" → IdentSanitize
	// → "TextPlain". Expected const name: "ContentTypeTextPlain".
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("ContentType", []string{"text/plain"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), "ContentTypeTextPlain") {
		t.Errorf("output = %q; expected const name 'ContentTypeTextPlain' for IDL value 'text/plain'", out)
	}
}

// ---------------------------------------------------------------------------
// Gap 5b: digit-leading value → const name (false negative fix)
// ---------------------------------------------------------------------------

func TestEnumDeclValueStartingWithDigitConstName(t *testing.T) {
	t.Parallel()
	// "2d" → IdentSanitize("2d") → digit-leading → "X2d".
	// Expected const name: "ContextX2d".
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Context", []string{"2d"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), "ContextX2d") {
		t.Errorf("output = %q; expected const name 'ContextX2d' for IDL value '2d'", out)
	}
}

// ---------------------------------------------------------------------------
// Gap 5c: parse helper body covers all IDL values (false negative fix)
// ---------------------------------------------------------------------------

func TestEnumDeclParseHelperCoversAllValues(t *testing.T) {
	t.Parallel()
	// The ParseEndingType function body must include case arms (or equivalent)
	// for every IDL value — not just the function signature.
	idlValues := []string{"transparent", "native", "no-change"}
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("EndingType", idlValues, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	s := string(out)
	parseIdx := strings.Index(s, "func ParseEndingType")
	if parseIdx == -1 {
		t.Fatal("ParseEndingType function not found in output")
	}
	body := s[parseIdx:]
	for _, v := range idlValues {
		quoted := `"` + v + `"`
		if !strings.Contains(body, quoted) {
			t.Errorf("ParseEndingType body missing case for %s:\n%s", quoted, body)
		}
	}
}

// ---------------------------------------------------------------------------
// Gap 6a: dot and space normalisation (coverage asymmetry)
// ---------------------------------------------------------------------------

func TestEnumDeclValueWithDot(t *testing.T) {
	t.Parallel()
	// "1.0" — dot is non-alphanumeric; must be normalised. Render() must succeed
	// and the string literal "1.0" must be preserved.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Version", []string{"1.0"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v (dot in IDL value must not cause render failure)", err)
	}
	s := string(out)
	if !strings.Contains(s, `"1.0"`) {
		t.Errorf("output = %q; IDL value \"1.0\" must be preserved as string literal", s)
	}
	if strings.Contains(s, "Version.") {
		t.Errorf("output = %q; const name must not contain a dot", s)
	}
}

// ---------------------------------------------------------------------------
// Gap 6b: parse helper runtime behaviour — subprocess compilation (coverage)
// ---------------------------------------------------------------------------

func TestEnumDeclParseHelperRuntimeBehaviour(t *testing.T) {
	t.Parallel()
	// Compile and run the generated code in a subprocess to verify that the
	// parse helper actually returns correct values at runtime — not just that
	// it compiles. This is the only test that catches a stub implementation
	// that always returns ("", false).
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("EndingType", []string{"transparent", "native"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("main")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}

	// Append a main() that exercises ParseEndingType and panics on failure.
	// The subprocess exits non-zero on panic, which exec detects.
	src := string(out) + `
func main() {
	v, ok := ParseEndingType("transparent")
	if !ok {
		panic("ParseEndingType(\"transparent\") returned ok=false, want true")
	}
	if v != EndingTypeTransparent {
		panic("ParseEndingType(\"transparent\") returned wrong value")
	}

	v2, ok2 := ParseEndingType("native")
	if !ok2 {
		panic("ParseEndingType(\"native\") returned ok=false, want true")
	}
	if v2 != EndingTypeNative {
		panic("ParseEndingType(\"native\") returned wrong value")
	}

	_, ok3 := ParseEndingType("unknown-value")
	if ok3 {
		panic("ParseEndingType(\"unknown-value\") returned ok=true, want false")
	}
}
`
	dir := t.TempDir()
	srcFile := dir + "/main.go"
	if err := os.WriteFile(srcFile, []byte(src), 0600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", srcFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated ParseEndingType failed at runtime:\n%s\nerror: %v", output, err)
	}
}

func TestEnumDeclValueWithSpace(t *testing.T) {
	t.Parallel()
	// "text plain" — space is non-alphanumeric; must be normalised.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Mime", []string{"text plain"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v (space in IDL value must not cause render failure)", err)
	}
	if !strings.Contains(string(out), `"text plain"`) {
		t.Errorf("output = %q; IDL value \"text plain\" must be preserved as string literal", out)
	}
}

// ---------------------------------------------------------------------------
// Review-finding fixes — regression tests
// ---------------------------------------------------------------------------

func TestEnumDeclNilDiagWithCollisionNoPanic(t *testing.T) {
	t.Parallel()
	// Passing nil for diag must not panic, even when a const-name collision occurs.
	decl := codegen.NewEnumDecl("State", []string{"no-change", "no_change"}, nil)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil with nil diag")
	}
}

func TestEnumDeclEmptyIdlNameEmitsDiagnostic(t *testing.T) {
	t.Parallel()
	// An empty idlName has no letter/digit content and must produce an error
	// diagnostic rather than silently emitting type X.
	diag := codegen.NewDiagnostics()
	codegen.NewEnumDecl("", []string{"a"}, diag)
	if diag.IsClean() {
		t.Error("expected error diagnostic for empty idlName; diagnostics are clean")
	}
}

func TestEnumDeclAllPunctIdlNameEmitsDiagnostic(t *testing.T) {
	t.Parallel()
	// An all-punctuation idlName (e.g. "---") has no alnum content and must
	// produce an error diagnostic.
	diag := codegen.NewDiagnostics()
	codegen.NewEnumDecl("---", []string{"a"}, diag)
	if diag.IsClean() {
		t.Error("expected error diagnostic for all-punct idlName; diagnostics are clean")
	}
}

func TestEnumDeclDuplicateTypenameInFileRendersError(t *testing.T) {
	t.Parallel()
	// "readyState" and "ready-state" both sanitize to "ReadyState". Adding both
	// EnumDecls to one File must produce an error from Render(), not silent
	// invalid Go that only go build would catch.
	diag := codegen.NewDiagnostics()
	d1 := codegen.NewEnumDecl("readyState", []string{"open"}, diag)
	d2 := codegen.NewEnumDecl("ready-state", []string{"closed"}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(d1)
	f.AddDecl(d2)
	_, err := f.Render()
	if err == nil {
		t.Error("File.Render() must return an error when two EnumDecls share the same sanitized type name")
	}
}

func TestEnumDeclEmptyValueParseHelperHasWarningComment(t *testing.T) {
	t.Parallel()
	// When "" is a valid IDL value, the generated ParseFoo function must carry
	// a comment warning callers to always check the bool return.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Foo", []string{"a", ""}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), "always check the bool") {
		t.Errorf("generated ParseFoo must warn about the bool check when \"\" is a valid member:\n%s", out)
	}
}
