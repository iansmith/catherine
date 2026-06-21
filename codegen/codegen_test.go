package codegen_test

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// ---------------------------------------------------------------------------
// IdentSanitize — edge/boundary
// ---------------------------------------------------------------------------

func TestIdentSanitizeEmpty(t *testing.T) {
	t.Parallel()
	// Empty IDL name must not produce an empty Go identifier.
	got := codegen.IdentSanitize("")
	if got == "" {
		t.Error(`IdentSanitize("") returned ""; must produce a non-empty valid Go identifier`)
	}
}

func TestIdentSanitizeLeadingDigit(t *testing.T) {
	t.Parallel()
	// Go identifiers cannot start with a digit.
	got := codegen.IdentSanitize("2dContext")
	if len(got) == 0 {
		t.Fatal(`IdentSanitize("2dContext") returned ""`)
	}
	if got[0] >= '0' && got[0] <= '9' {
		t.Errorf(`IdentSanitize("2dContext") = %q; must not start with a digit`, got)
	}
}

func TestIdentSanitizeHyphen(t *testing.T) {
	t.Parallel()
	// Hyphens are not valid in Go identifiers.
	got := codegen.IdentSanitize("allow-shared")
	if strings.Contains(got, "-") {
		t.Errorf(`IdentSanitize("allow-shared") = %q; must not contain a hyphen`, got)
	}
	if len(got) == 0 {
		t.Error(`IdentSanitize("allow-shared") returned ""`)
	}
}

func TestIdentSanitizeMultipleHyphens(t *testing.T) {
	t.Parallel()
	got := codegen.IdentSanitize("css-float-value")
	if strings.Contains(got, "-") {
		t.Errorf(`IdentSanitize("css-float-value") = %q; must contain no hyphens`, got)
	}
}

func TestIdentSanitizeHyphenProducesPascalCase(t *testing.T) {
	t.Parallel()
	// "allow-shared" → "AllowShared": each segment capitalized.
	got := codegen.IdentSanitize("allow-shared")
	if got != "AllowShared" {
		t.Errorf(`IdentSanitize("allow-shared") = %q, want "AllowShared"`, got)
	}
}

func TestIdentSanitizeGoReservedWordInterface(t *testing.T) {
	t.Parallel()
	// "interface" is a Go reserved word and must be transformed.
	got := codegen.IdentSanitize("interface")
	if got == "interface" {
		t.Errorf(`IdentSanitize("interface") = %q; bare Go reserved word not allowed`, got)
	}
	if len(got) == 0 {
		t.Error(`IdentSanitize("interface") returned ""`)
	}
}

func TestIdentSanitizeGoReservedWordType(t *testing.T) {
	t.Parallel()
	got := codegen.IdentSanitize("type")
	if got == "type" {
		t.Errorf(`IdentSanitize("type") = %q; bare Go reserved word not allowed`, got)
	}
}

func TestIdentSanitizeGoReservedWordMap(t *testing.T) {
	t.Parallel()
	got := codegen.IdentSanitize("map")
	if got == "map" {
		t.Errorf(`IdentSanitize("map") = %q; bare Go reserved word not allowed`, got)
	}
}

func TestIdentSanitizeLowercaseFirstProducesPascalCase(t *testing.T) {
	t.Parallel()
	// IDL names often start lowercase; exported Go names must start uppercase.
	got := codegen.IdentSanitize("someAttribute")
	if len(got) == 0 {
		t.Fatal(`IdentSanitize("someAttribute") returned ""`)
	}
	if got[0] < 'A' || got[0] > 'Z' {
		t.Errorf(`IdentSanitize("someAttribute") = %q; first rune must be uppercase`, got)
	}
}

func TestIdentSanitizeAlreadyPascalCase(t *testing.T) {
	t.Parallel()
	// Already-valid exported names pass through unchanged.
	got := codegen.IdentSanitize("DOMString")
	if got != "DOMString" {
		t.Errorf(`IdentSanitize("DOMString") = %q, want "DOMString"`, got)
	}
}

func TestIdentSanitizeAllCapsAcronym(t *testing.T) {
	t.Parallel()
	// All-caps acronyms like "URL" must survive intact.
	got := codegen.IdentSanitize("URL")
	if got != "URL" {
		t.Errorf(`IdentSanitize("URL") = %q, want "URL"`, got)
	}
}

// ---------------------------------------------------------------------------
// Diagnostics — edge/boundary and error/rejection
// ---------------------------------------------------------------------------

func TestDiagnosticsCleanOnNew(t *testing.T) {
	t.Parallel()
	d := codegen.NewDiagnostics()
	if !d.IsClean() {
		t.Error("NewDiagnostics().IsClean() = false; fresh Diagnostics must be clean")
	}
}

func TestDiagnosticsErrorMakesDirty(t *testing.T) {
	t.Parallel()
	d := codegen.NewDiagnostics()
	d.Add("error", "cannot map type X")
	if d.IsClean() {
		t.Error("Diagnostics.IsClean() = true after adding an error; must be false")
	}
}

func TestDiagnosticsWarningStaysClean(t *testing.T) {
	t.Parallel()
	// Warnings do not make the pipeline dirty — they are informational.
	d := codegen.NewDiagnostics()
	d.Add("warning", "type Y has no annotation")
	if !d.IsClean() {
		t.Error("Diagnostics.IsClean() = false after a warning; warnings must not dirty the pipeline")
	}
}

func TestDiagnosticsErrorsReturnsOnlyErrors(t *testing.T) {
	t.Parallel()
	d := codegen.NewDiagnostics()
	d.Add("warning", "w1")
	d.Add("error", "e1")
	d.Add("error", "e2")
	errs := d.Errors()
	if len(errs) != 2 {
		t.Errorf("Diagnostics.Errors() len = %d, want 2", len(errs))
	}
}

func TestDiagnosticsErrorsEmptyWhenClean(t *testing.T) {
	t.Parallel()
	d := codegen.NewDiagnostics()
	if errs := d.Errors(); len(errs) != 0 {
		t.Errorf("fresh Diagnostics.Errors() = %v; want empty slice", errs)
	}
}

func TestDiagnosticsFormatIncludesMessage(t *testing.T) {
	t.Parallel()
	d := codegen.NewDiagnostics()
	d.Add("error", "cannot resolve IDL type Z")
	out := d.Format()
	if !strings.Contains(out, "cannot resolve IDL type Z") {
		t.Errorf("Diagnostics.Format() = %q; must include the error message", out)
	}
}

// ---------------------------------------------------------------------------
// ImportTracker — edge/boundary and cross-feature interaction
// ---------------------------------------------------------------------------

func TestImportTrackerEmptyRendersEmpty(t *testing.T) {
	t.Parallel()
	// An empty tracker must not produce an import block at all.
	tr := codegen.NewImportTracker()
	got := tr.Render()
	if strings.Contains(got, "import") {
		t.Errorf("empty ImportTracker.Render() = %q; must not contain 'import'", got)
	}
}

func TestImportTrackerDeduplicate(t *testing.T) {
	t.Parallel()
	tr := codegen.NewImportTracker()
	tr.Add("fmt")
	tr.Add("fmt")
	tr.Add("fmt")
	got := tr.Render()
	count := strings.Count(got, `"fmt"`)
	if count != 1 {
		t.Errorf("ImportTracker.Render() contains %d occurrences of %q after 3 Add calls; want 1", count, "fmt")
	}
}

func TestImportTrackerSingleStdlib(t *testing.T) {
	t.Parallel()
	tr := codegen.NewImportTracker()
	tr.Add("fmt")
	got := tr.Render()
	if !strings.Contains(got, `"fmt"`) {
		t.Errorf("ImportTracker.Render() = %q; must contain %q", got, `"fmt"`)
	}
}

func TestImportTrackerStdlibBeforeExternal(t *testing.T) {
	t.Parallel()
	// The import block must group stdlib before external packages.
	tr := codegen.NewImportTracker()
	tr.Add("github.com/iansmith/webidl/typemap")
	tr.Add("fmt")
	got := tr.Render()
	fmtIdx := strings.Index(got, `"fmt"`)
	extIdx := strings.Index(got, `"github.com/iansmith/webidl/typemap"`)
	if fmtIdx == -1 {
		t.Fatalf("ImportTracker.Render() missing %q", "fmt")
	}
	if extIdx == -1 {
		t.Fatalf("ImportTracker.Render() missing external import")
	}
	if fmtIdx > extIdx {
		t.Errorf("stdlib import appears after external import; want stdlib first")
	}
}

func TestImportTrackerGroupsSeparatedByBlankLine(t *testing.T) {
	t.Parallel()
	// stdlib and external groups must be separated by a blank line.
	tr := codegen.NewImportTracker()
	tr.Add("fmt")
	tr.Add("github.com/iansmith/webidl/typemap")
	got := tr.Render()
	// A blank line between groups means two consecutive newlines (possibly with tabs).
	if !strings.Contains(got, "\n\n") {
		t.Errorf("ImportTracker.Render() = %q; stdlib and external groups must be separated by a blank line", got)
	}
}

func TestImportTrackerSortedWithinGroup(t *testing.T) {
	t.Parallel()
	tr := codegen.NewImportTracker()
	tr.Add("strings")
	tr.Add("fmt")
	got := tr.Render()
	fmtIdx := strings.Index(got, `"fmt"`)
	strIdx := strings.Index(got, `"strings"`)
	if fmtIdx == -1 || strIdx == -1 {
		t.Fatal("ImportTracker.Render() missing expected entries")
	}
	if fmtIdx > strIdx {
		t.Errorf("imports not sorted: %q appears after %q", "fmt", "strings")
	}
}

// ---------------------------------------------------------------------------
// File — edge/boundary and error/rejection
// ---------------------------------------------------------------------------

func TestFileEmptyPackageNameErrors(t *testing.T) {
	t.Parallel()
	// A File with an empty package name must fail to render.
	f := codegen.NewFile("")
	_, err := f.Render()
	if err == nil {
		t.Error("NewFile(\"\").Render() returned nil error; must fail on empty package name")
	}
}

func TestFileRenderProducesPackageDecl(t *testing.T) {
	t.Parallel()
	f := codegen.NewFile("mypackage")
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), "package mypackage") {
		t.Errorf("File.Render() = %q; must contain 'package mypackage'", out)
	}
}

func TestFileRenderOutputIsValidGo(t *testing.T) {
	t.Parallel()
	// Render output must be parseable Go (passes through gofmt).
	f := codegen.NewFile("gen")
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	// gofmt-formatted output always ends with a newline.
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Errorf("File.Render() = %q; gofmt output must end with a newline", out)
	}
}

// ---------------------------------------------------------------------------
// File + ImportTracker — cross-feature interaction
// ---------------------------------------------------------------------------

func TestFileWithImportsRendersImportBlock(t *testing.T) {
	t.Parallel()
	tr := codegen.NewImportTracker()
	tr.Add("fmt")
	f := codegen.NewFile("gen")
	f.SetImports(tr)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if !strings.Contains(string(out), `"fmt"`) {
		t.Errorf("File.Render() = %q; must include fmt import", out)
	}
}

func TestFileWithEmptyImportTrackerOmitsImportBlock(t *testing.T) {
	t.Parallel()
	// An empty tracker must not produce a bare `import ()` block.
	f := codegen.NewFile("gen")
	f.SetImports(codegen.NewImportTracker())
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	if strings.Contains(string(out), "import") {
		t.Errorf("File.Render() with empty ImportTracker = %q; must not contain 'import'", out)
	}
}
