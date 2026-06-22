package codegen_test

import (
	"fmt"
	"go/format"
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// ── 1-A: inheritance + own field — both must appear ─────────────────────────

func TestDictDeclInheritanceWithOneOwnFieldBothAppear(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("CustomEventInit", "EventInit", []codegen.DictField{
		{IDLName: "detail", GoType: "any", Optional: true},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "\tEventInit\n") {
		t.Errorf("embedded parent missing:\n%s", src)
	}
	if !strings.Contains(src, "Detail *any") {
		t.Errorf("own optional field Detail *any missing:\n%s", src)
	}
}

// ── 1-B: nil fields + parent → embedding still emitted ──────────────────────

func TestDictDeclNilFieldsWithParentStillEmbedsParent(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Child", "Parent", nil, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "\tParent\n") {
		t.Errorf("nil-fields child must still embed Parent:\n%s", out)
	}
}

// ── 1-C: many fields — loop/capacity smoke test ──────────────────────────────

func TestDictDeclManyFields(t *testing.T) {
	diag := codegen.NewDiagnostics()
	fields := make([]codegen.DictField, 10)
	for i := range fields {
		fields[i] = codegen.DictField{
			IDLName:  fmt.Sprintf("field%d", i),
			GoType:   "string",
			Optional: i%2 == 0,
		}
	}
	decl := codegen.NewDictDecl("BigDict", "", fields, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	for i := range fields {
		goName := fmt.Sprintf("Field%d", i)
		if !strings.Contains(src, goName) {
			t.Errorf("field %s missing from output:\n%s", goName, src)
		}
	}
}

// ── 1-D: single-character dict IDLName ───────────────────────────────────────

func TestDictDeclSingleCharIDLName(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("A", "", []codegen.DictField{
		{IDLName: "x", GoType: "bool", Optional: false},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "type A struct") {
		t.Errorf("single-char IDL name must produce 'type A struct':\n%s", out)
	}
}

// ── 2-A: three-way field collision must produce ≥2 diagnostics ───────────────

func TestDictDeclThreeWayFieldCollisionTwoDiagnostics(t *testing.T) {
	// "a-b", "a_b", "aB" may all sanitize to the same Go name
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Foo", "", []codegen.DictField{
		{IDLName: "no-change", GoType: "string", Optional: false},
		{IDLName: "no_change", GoType: "int32", Optional: false},
		{IDLName: "noChange", GoType: "bool", Optional: false},
	}, diag)
	if len(diag.Errors()) < 2 {
		t.Errorf("three-way collision must produce ≥2 error diagnostics; got %d:\n%s",
			len(diag.Errors()), diag.Format())
	}
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render after three-way collision: %v", err)
	}
	// Only the first field wins
	if strings.Count(string(out), "NoChange ") != 1 {
		t.Errorf("expected exactly one NoChange field after three-way collision:\n%s", out)
	}
}

// ── 2-B: empty field IDLName behavior ────────────────────────────────────────

func TestDictDeclEmptyFieldIDLNameEmitsDiagnostic(t *testing.T) {
	// Empty IDLName on a field has no letter/digit content — must emit error diagnostic.
	diag := codegen.NewDiagnostics()
	codegen.NewDictDecl("Foo", "", []codegen.DictField{
		{IDLName: "", GoType: "string", Optional: false},
	}, diag)
	if diag.IsClean() {
		t.Error("empty field IDLName must produce an error diagnostic")
	}
}

// ── 2-C: all-punct field IDLName behavior ────────────────────────────────────

func TestDictDeclAllPunctFieldIDLNameEmitsDiagnostic(t *testing.T) {
	diag := codegen.NewDiagnostics()
	codegen.NewDictDecl("Foo", "", []codegen.DictField{
		{IDLName: "---", GoType: "string", Optional: false},
	}, diag)
	if diag.IsClean() {
		t.Error("all-punct field IDLName (no letter or digit) must produce an error diagnostic")
	}
}

// ── 2-D: nil diag + collision must not panic ─────────────────────────────────

func TestDictDeclNilDiagWithCollisionNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewDictDecl panicked with nil diag on field collision: %v", r)
		}
	}()
	codegen.NewDictDecl("Foo", "", []codegen.DictField{
		{IDLName: "my-field", GoType: "string", Optional: false},
		{IDLName: "my_field", GoType: "int32", Optional: false},
	}, nil)
}

// ── 3-A: second NewDictDecl call must not clear prior diagnostics ─────────────

func TestDictDeclSecondCallPreservesFirstCallErrors(t *testing.T) {
	diag := codegen.NewDiagnostics()
	// First call: field collision → error
	codegen.NewDictDecl("Foo", "", []codegen.DictField{
		{IDLName: "my-field", GoType: "string", Optional: false},
		{IDLName: "my_field", GoType: "int32", Optional: false},
	}, diag)
	if diag.IsClean() {
		t.Fatal("expected error after field collision in first call")
	}
	prevCount := len(diag.Errors())

	// Second call: clean → must not reset diag
	codegen.NewDictDecl("Bar", "", []codegen.DictField{
		{IDLName: "z", GoType: "bool", Optional: false},
	}, diag)
	if len(diag.Errors()) != prevCount {
		t.Errorf("second NewDictDecl altered error count: before=%d after=%d",
			prevCount, len(diag.Errors()))
	}
}

// ── 3-B: EnumDecl + DictDecl same sanitized name → File.Render error ─────────

func TestFileDuplicateNameAcrossEnumDeclAndDictDecl(t *testing.T) {
	diag := codegen.NewDiagnostics()
	enum := codegen.NewEnumDecl("Status", []string{"open"}, diag)
	dict := codegen.NewDictDecl("Status", "", nil, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(enum)
	f.AddDecl(dict)

	_, err := f.Render()
	if err == nil {
		t.Error("File.Render must return error when EnumDecl and DictDecl share the same sanitized type name")
	}
}

// ── 4-A: no-inheritance test must assert no bare embedding ───────────────────

func TestDictDeclNoInheritanceBodyHasNoBareEmbeddedField(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Root", "", []codegen.DictField{
		{IDLName: "x", GoType: "float64", Optional: false},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	// An embedded field appears as a bare identifier on its own line (tab+Name\n).
	// Only "X float64 `json:\"x\"`" should appear; no bare type-only line.
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "type ") ||
			strings.HasPrefix(trimmed, "package ") ||
			trimmed == "{" || trimmed == "}" {
			continue
		}
		// A bare embedding has zero spaces (single token)
		if strings.Count(trimmed, " ") == 0 && !strings.HasPrefix(trimmed, "//") {
			t.Errorf("no-inheritance struct has a bare embedded field %q:\n%s", trimmed, src)
		}
	}
}

// ── 4-B: digit-leading field name → Go name must be X-prefixed ───────────────

func TestDictDeclDigitLeadingFieldIDLNameGoNameIsXPrefixed(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Canvas", "", []codegen.DictField{
		{IDLName: "2d", GoType: "bool", Optional: false},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	// IdentSanitize("2d") → digit-leading → prepend X → "X2d"
	if !strings.Contains(src, "X2d bool") {
		t.Errorf("digit-leading IDL field '2d' must produce Go field 'X2d bool':\n%s", src)
	}
	if !strings.Contains(src, `json:"2d"`) {
		t.Errorf("JSON tag must preserve original IDL name '2d':\n%s", src)
	}
}

// ── 4-C: declName drives File.Render duplicate detection ─────────────────────

func TestDictDeclDeclNameDrivesFileDuplicateDetection(t *testing.T) {
	// "event-init" and "eventInit" both sanitize to "EventInit"
	diag := codegen.NewDiagnostics()
	d1 := codegen.NewDictDecl("event-init", "", nil, diag)
	d2 := codegen.NewDictDecl("eventInit", "", nil, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(d1)
	f.AddDecl(d2)
	_, err := f.Render()
	if err == nil {
		t.Fatal("File.Render must error for two DictDecls with same sanitized name")
	}
	if !strings.Contains(err.Error(), "EventInit") {
		t.Errorf("error must mention the sanitized name 'EventInit'; got: %v", err)
	}
}

// ── 5-A: type name sanitization is actually exercised ────────────────────────

func TestDictDeclTypeNameIsSanitizedFromIDLName(t *testing.T) {
	// "event-init" → IdentSanitize → "EventInit"; raw name must not appear
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("event-init", "", nil, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "type EventInit struct") {
		t.Errorf("sanitized name 'EventInit' missing:\n%s", src)
	}
	if strings.Contains(src, "event-init") {
		t.Errorf("raw IDL name 'event-init' must not appear in output:\n%s", src)
	}
}

// ── 5-B: required field full declaration (name + type + tag) verified ─────────

func TestDictDeclRequiredFieldFullDeclaration(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Req", "", []codegen.DictField{
		{IDLName: "count", GoType: "int32", Optional: false},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	// Full declaration: Count int32 `json:"count"`
	if !strings.Contains(src, "Count int32") {
		t.Errorf("required field must appear as 'Count int32':\n%s", src)
	}
	want := "Count int32 `json:\"count\"`"
	if !strings.Contains(src, want) {
		t.Errorf("required field full declaration missing %q:\n%s", want, src)
	}
}

// ── 5-C: inheritance embedding is unnamed (not a named field) ─────────────────

func TestDictDeclInheritanceEmbeddingIsUnnamed(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Child", "Parent", nil, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "\tParent\n") {
		t.Errorf("embedding must be unnamed (bare type name on its own line):\n%s", src)
	}
	if strings.Contains(src, "Parent Parent") {
		t.Errorf("embedding must not appear as a named field 'Parent Parent':\n%s", src)
	}
}

// ── 5-D: gofmt-idempotent test also verifies field content ───────────────────

func TestDictDeclRenderGofmtIdempotentAndContainsFields(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Request", "", []codegen.DictField{
		{IDLName: "name", GoType: "string", Optional: false},
		{IDLName: "size", GoType: "int64", Optional: true},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out2, err := format.Source(out)
	if err != nil {
		t.Fatalf("re-format failed: %v", err)
	}
	if string(out) != string(out2) {
		t.Errorf("output is not gofmt-idempotent:\nfirst:\n%s\nsecond:\n%s", out, out2)
	}
	src := string(out)
	if !strings.Contains(src, "Name string") {
		t.Errorf("field 'Name string' missing from gofmt-idempotent output:\n%s", src)
	}
	if !strings.Contains(src, "Size *int64") {
		t.Errorf("field 'Size *int64' missing from gofmt-idempotent output:\n%s", src)
	}
}

// ── 6-A: optional slice GoType → *[]string, not []*string ───────────────────

func TestDictDeclOptionalSliceGoTypeWrappedCorrectly(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Batch", "", []codegen.DictField{
		{IDLName: "items", GoType: "[]string", Optional: true},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "Items *[]string") {
		t.Errorf("optional slice field must be '*[]string' (not '[]*string'):\n%s", src)
	}
	if !strings.Contains(src, `json:"items,omitempty"`) {
		t.Errorf("optional slice field must have omitempty json tag:\n%s", src)
	}
}

// ── 6-B: required `any` field ────────────────────────────────────────────────

func TestDictDeclRequiredAnyField(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Payload", "", []codegen.DictField{
		{IDLName: "data", GoType: "any", Optional: false},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "Data any") {
		t.Errorf("required 'any' field must appear as 'Data any':\n%s", src)
	}
	if strings.Contains(src, "omitempty") {
		t.Errorf("required 'any' field must not have omitempty:\n%s", src)
	}
}

// ── 6-C: optional already-pointer GoType → ** (no stripping) ─────────────────

func TestDictDeclOptionalAlreadyPointerGoTypeProducesDoublePointer(t *testing.T) {
	// GoType "*int32" + Optional=true must produce "**int32" — the implementation
	// wraps whatever GoType the caller provides; callers own type resolution.
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Ptr", "", []codegen.DictField{
		{IDLName: "val", GoType: "*int32", Optional: true},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "Val **int32") {
		t.Errorf("optional field with GoType='*int32' must produce 'Val **int32':\n%s", out)
	}
}

// ── 6-D: inheritance + multiple own fields — embedding survives ───────────────

func TestDictDeclInheritanceWithMultipleOwnFields(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("CustomInit", "EventInit", []codegen.DictField{
		{IDLName: "detail", GoType: "any", Optional: true},
		{IDLName: "bubbles", GoType: "bool", Optional: false},
		{IDLName: "target", GoType: "string", Optional: true},
	}, diag)
	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "\tEventInit\n") {
		t.Errorf("embedded parent must appear even with multiple own fields:\n%s", src)
	}
	if !strings.Contains(src, "Detail *any") {
		t.Errorf("optional field Detail missing:\n%s", src)
	}
	if !strings.Contains(src, "Bubbles bool") {
		t.Errorf("required field Bubbles missing:\n%s", src)
	}
	if !strings.Contains(src, "Target *string") {
		t.Errorf("optional field Target missing:\n%s", src)
	}
}
