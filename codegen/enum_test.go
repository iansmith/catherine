package codegen_test

import (
	"bytes"
	"go/format"
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// ---------------------------------------------------------------------------
// NewEnumDecl — edge / boundary
// ---------------------------------------------------------------------------

func TestEnumDeclEmptyStringValue(t *testing.T) {
	t.Parallel()
	// An enum with an empty-string IDL value must render valid Go and must
	// preserve the empty string as the const value ("" in the output).
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Foo", []string{""}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil for enum with empty-string value")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v (empty-string IDL value must not cause a render failure)", err)
	}
	if !strings.Contains(string(out), `= ""`) {
		t.Errorf("output = %q; empty IDL value must appear as string literal '= \"\"' in const", out)
	}
}

func TestEnumDeclSingleValue(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Color", []string{"red"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	if !diag.IsClean() {
		t.Errorf("unexpected diagnostics for single-value enum: %s", diag.Format())
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), "type Color string") {
		t.Errorf("output must contain 'type Color string':\n%s", out)
	}
	if !strings.Contains(string(out), `"red"`) {
		t.Errorf("output must contain IDL value %q as string literal:\n%s", `"red"`, out)
	}
}

func TestEnumDeclValueWithHyphen(t *testing.T) {
	t.Parallel()
	// "no-change" must produce a const whose name contains no hyphen.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("State", []string{"no-change"}, diag)
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
	// The const name must contain "No" and "Change" (hyphen split → PascalCase).
	if !strings.Contains(s, "StateNoChange") {
		t.Errorf("output = %q; expected const name 'StateNoChange' from IDL value 'no-change'", s)
	}
	// The IDL string literal must still contain the hyphen.
	if !strings.Contains(s, `"no-change"`) {
		t.Errorf("output = %q; IDL value \"no-change\" must appear as string literal in const", s)
	}
}

func TestEnumDeclValueWithSlash(t *testing.T) {
	t.Parallel()
	// "text/plain" contains a slash, which is illegal in a Go identifier.
	// File.Render() must succeed (slash handled in const name); the IDL value
	// must appear unmodified as the string literal.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("ContentType", []string{"text/plain"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v (slash in IDL value must not cause render failure)", err)
	}
	if !strings.Contains(string(out), `"text/plain"`) {
		t.Errorf("output = %q; IDL value \"text/plain\" must be preserved as string literal", out)
	}
}

func TestEnumDeclValueStartingWithDigit(t *testing.T) {
	t.Parallel()
	// "2d" starts with a digit; the const name must not start with a digit.
	// File.Render() validates this via gofmt.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("Context", []string{"2d"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v (digit-leading IDL value must not cause render failure)", err)
	}
	if !strings.Contains(string(out), `"2d"`) {
		t.Errorf("output = %q; IDL value \"2d\" must appear as string literal", out)
	}
}

func TestEnumDeclTypeNameWithHyphen(t *testing.T) {
	t.Parallel()
	// An IDL enum name containing a hyphen must be sanitized to PascalCase.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("ending-type", []string{"transparent"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v (hyphen in IDL enum name must be sanitized)", err)
	}
	if !strings.Contains(string(out), "type EndingType string") {
		t.Errorf("output = %q; IDL name 'ending-type' must produce Go type 'EndingType'", out)
	}
}

// ---------------------------------------------------------------------------
// NewEnumDecl — error / rejection
// ---------------------------------------------------------------------------

func TestEnumDeclCollisionRecordedInDiagnostics(t *testing.T) {
	t.Parallel()
	// "no-change" and "no_change" both sanitize to "NoChange"; that is a
	// const-name collision and must be reported as an error diagnostic.
	diag := codegen.NewDiagnostics()
	codegen.NewEnumDecl("State", []string{"no-change", "no_change"}, diag)
	if diag.IsClean() {
		t.Error("expected collision diagnostic error for 'no-change' vs 'no_change'; diagnostics are clean")
	}
}

func TestEnumDeclCollisionFirstWins(t *testing.T) {
	t.Parallel()
	// On collision the first value keeps its const; the second is dropped.
	// Exactly one const for "StateNoChange" must appear in the output.
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
	count := strings.Count(string(out), "StateNoChange")
	if count != 1 {
		t.Errorf("output contains %d occurrences of 'StateNoChange'; want exactly 1 (collision: first wins)", count)
	}
}

// ---------------------------------------------------------------------------
// NewEnumDecl — cross-feature interaction
// ---------------------------------------------------------------------------

func TestEnumDeclAddedToFileRenderValid(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("EndingType", []string{"transparent", "native"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if len(out) == 0 {
		t.Error("File.Render() returned empty output")
	}
}

func TestEnumDeclRenderIsGofmtIdempotent(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("EndingType", []string{"transparent", "native"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	formatted, err := format.Source(out)
	if err != nil {
		t.Fatalf("go/format.Source on Render() output failed: %v\noutput:\n%s", err, out)
	}
	if !bytes.Equal(out, formatted) {
		t.Errorf("File.Render() is not gofmt-canonical:\ngot:\n%s\nwant:\n%s", out, formatted)
	}
}

func TestEnumDeclCleanDiagnosticsOnHappyPath(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	codegen.NewEnumDecl("EndingType", []string{"transparent", "native"}, diag)
	if !diag.IsClean() {
		t.Errorf("expected clean diagnostics for well-formed enum; got: %s", diag.Format())
	}
}

// ---------------------------------------------------------------------------
// NewEnumDecl — happy path
// ---------------------------------------------------------------------------

func TestEnumDeclTypeDeclaration(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("EndingType", []string{"transparent", "native"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), "type EndingType string") {
		t.Errorf("output must contain 'type EndingType string':\n%s", out)
	}
}

func TestEnumDeclConstNamesAndValues(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("EndingType", []string{"transparent", "native"}, diag)
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
	if !strings.Contains(s, "EndingTypeTransparent") {
		t.Errorf("output must contain const 'EndingTypeTransparent':\n%s", s)
	}
	if !strings.Contains(s, "EndingTypeNative") {
		t.Errorf("output must contain const 'EndingTypeNative':\n%s", s)
	}
	// Const values must be the original IDL string literals
	if !strings.Contains(s, `"transparent"`) {
		t.Errorf("output must contain string literal %q:\n%s", `"transparent"`, s)
	}
	if !strings.Contains(s, `"native"`) {
		t.Errorf("output must contain string literal %q:\n%s", `"native"`, s)
	}
}

func TestEnumDeclParseHelperEmitted(t *testing.T) {
	t.Parallel()
	// The output must contain a ParseEndingType(s string) (EndingType, bool) function.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewEnumDecl("EndingType", []string{"transparent", "native"}, diag)
	if decl == nil {
		t.Fatal("NewEnumDecl returned nil")
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), "ParseEndingType") {
		t.Errorf("output must contain a ParseEndingType helper function:\n%s", out)
	}
}

func TestEnumDeclRoundTripAllValuesRepresented(t *testing.T) {
	t.Parallel()
	// Every non-empty IDL value must appear as a string literal in the output,
	// no more and no less (modulo collision-dropped entries which are covered
	// by TestEnumDeclCollisionFirstWins).
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
	for _, v := range idlValues {
		quoted := `"` + v + `"`
		if !strings.Contains(s, quoted) {
			t.Errorf("IDL value %q not found as string literal in output:\n%s", quoted, s)
		}
	}
}
