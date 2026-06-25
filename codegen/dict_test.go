package codegen_test

import (
	"go/format"
	"regexp"
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// ── happy path ──────────────────────────────────────────────────────────────

func TestDictDeclTypeDeclaration(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("EventInit", "", []codegen.DictField{
		{IDLName: "bubbles", GoType: "bool", Optional: false},
	}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "type EventInit struct") {
		t.Errorf("missing struct type declaration:\n%s", out)
	}
}

func TestDictDeclRequiredFieldValueTypeNoOmitempty(t *testing.T) {
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

	if strings.Contains(src, "*int32") {
		t.Errorf("required field must be value type, not pointer:\n%s", src)
	}
	if strings.Contains(src, "omitempty") {
		t.Errorf("required field must NOT have omitempty:\n%s", src)
	}
	if !strings.Contains(src, `json:"count"`) {
		t.Errorf("required field must have json tag without omitempty:\n%s", src)
	}
}

func TestDictDeclOptionalFieldPointerTypeAndOmitempty(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Opts", "", []codegen.DictField{
		{IDLName: "timeout", GoType: "int64", Optional: true},
	}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "*int64") {
		t.Errorf("optional field must be pointer type:\n%s", src)
	}
	if !strings.Contains(src, `json:"timeout,omitempty"`) {
		t.Errorf("optional field must have omitempty json tag:\n%s", src)
	}
}

func TestDictDeclRequiredAndOptionalFieldsTogether(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Mixed", "", []codegen.DictField{
		{IDLName: "name", GoType: "string", Optional: false},
		{IDLName: "label", GoType: "string", Optional: true},
	}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)

	// gofmt aligns multi-field structs with extra spaces; collapse whitespace
	// runs so field-and-type assertions don't depend on exact column spacing.
	fields := normalizeSpaces(src)
	if !strings.Contains(fields, "Name string") {
		t.Errorf("required Name must be value type:\n%s", src)
	}
	if !strings.Contains(fields, "Label *string") {
		t.Errorf("optional Label must be pointer type:\n%s", src)
	}
	if !strings.Contains(src, `json:"name"`) {
		t.Errorf("required field must have json tag without omitempty:\n%s", src)
	}
	if !strings.Contains(src, `json:"label,omitempty"`) {
		t.Errorf("optional field must have omitempty json tag:\n%s", src)
	}
}

func TestDictDeclJSONTagUsesIDLNameNotGoName(t *testing.T) {
	// IDL field "my-field" → Go identifier "MyField", but JSON tag must use "my-field"
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Foo", "", []codegen.DictField{
		{IDLName: "my-field", GoType: "string", Optional: false},
	}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "MyField") {
		t.Errorf("sanitized Go name must be MyField:\n%s", src)
	}
	if !strings.Contains(src, `json:"my-field"`) {
		t.Errorf("JSON tag must use original IDL name 'my-field':\n%s", src)
	}
}

func TestDictDeclInheritanceEmbedsParent(t *testing.T) {
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

	// Embedding appears as tab+TypeName on its own line, no preceding field name
	if !strings.Contains(src, "\tEventInit\n") {
		t.Errorf("derived struct must embed EventInit (embedded field, no name):\n%s", src)
	}
}

func TestDictDeclNoInheritanceNoEmbedding(t *testing.T) {
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

	// No embedded field — struct only has its own field
	if strings.Count(src, "struct {") != 1 {
		t.Errorf("expected exactly one struct block:\n%s", src)
	}
}

func TestDictDeclRenderIsGofmtIdempotent(t *testing.T) {
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
}

// ── edge / boundary ─────────────────────────────────────────────────────────

func TestDictDeclEmptyFieldSlice(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Empty", "", []codegen.DictField{}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("empty-field dict must render without error: %v", err)
	}
	if !strings.Contains(string(out), "type Empty struct") {
		t.Errorf("missing struct declaration:\n%s", out)
	}
}

func TestDictDeclNilFieldSlice(t *testing.T) {
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("NilFields", "", nil, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	_, err := f.Render()
	if err != nil {
		t.Fatalf("nil-field dict must render without error: %v", err)
	}
}

func TestDictDeclEmptyIDLNameEmitsDiagnostic(t *testing.T) {
	diag := codegen.NewDiagnostics()
	codegen.NewDictDecl("", "", nil, diag)

	if diag.IsClean() {
		t.Error("empty idlName must produce an error diagnostic")
	}
}

func TestDictDeclAllPunctIDLNameEmitsDiagnostic(t *testing.T) {
	diag := codegen.NewDiagnostics()
	codegen.NewDictDecl("---", "", nil, diag)

	if diag.IsClean() {
		t.Error("all-punct idlName (no letter or digit) must produce an error diagnostic")
	}
}

func TestDictDeclNilDiagNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewDictDecl panicked with nil diag: %v", r)
		}
	}()
	codegen.NewDictDecl("Foo", "", nil, nil)
}

func TestDictDeclFieldWithGoKeywordIDLName(t *testing.T) {
	// IDL field "type" → PascalCase "Type" via IdentSanitize (never a Go keyword)
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Typed", "", []codegen.DictField{
		{IDLName: "type", GoType: "string", Optional: false},
	}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "Type string") {
		t.Errorf("IDL 'type' must produce Go identifier 'Type':\n%s", src)
	}
	if !strings.Contains(src, `json:"type"`) {
		t.Errorf("JSON tag must still be 'type' (the IDL name):\n%s", src)
	}
}

func TestDictDeclFieldWithDigitLeadingIDLName(t *testing.T) {
	// IDL field "2d" → IdentSanitize adds X prefix → "X2d"
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

	if strings.Contains(src, "\t2d") {
		t.Errorf("Go field identifier must not start with digit:\n%s", src)
	}
	if !strings.Contains(src, `json:"2d"`) {
		t.Errorf("JSON tag must preserve original IDL name '2d':\n%s", src)
	}
}

func TestDictDeclInheritanceWithNoOwnFields(t *testing.T) {
	// dictionary Child : Parent {} — only embedding, zero own fields
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Child", "Parent", []codegen.DictField{}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "\tParent\n") {
		t.Errorf("Child struct with no own fields must still embed Parent:\n%s", out)
	}
}

// ── error / rejection ───────────────────────────────────────────────────────

func TestDictDeclFieldNameCollisionFirstWins(t *testing.T) {
	// "my-field" and "my_field" both sanitize to "MyField"
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("Foo", "", []codegen.DictField{
		{IDLName: "my-field", GoType: "string", Optional: false},
		{IDLName: "my_field", GoType: "int32", Optional: false},
	}, diag)

	if diag.IsClean() {
		t.Error("field name collision must produce an error diagnostic")
	}

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render after field collision: %v", err)
	}
	src := string(out)

	// Exactly one MyField entry; first value (string) must win
	if strings.Count(src, "MyField") != 1 {
		t.Errorf("expected exactly one MyField after collision (first wins):\n%s", src)
	}
	if !strings.Contains(src, "MyField string") {
		t.Errorf("first-value type (string) must win field collision:\n%s", src)
	}
}

func TestDictDeclPreexistingDiagnosticsPreserved(t *testing.T) {
	diag := codegen.NewDiagnostics()
	diag.Add("warning", "pre-existing diagnostic")

	codegen.NewDictDecl("Foo", "", []codegen.DictField{
		{IDLName: "x", GoType: "string", Optional: false},
	}, diag)

	if !strings.Contains(diag.Format(), "pre-existing diagnostic") {
		t.Error("NewDictDecl must not clear pre-existing diagnostics")
	}
}

// ── cross-feature interaction ───────────────────────────────────────────────

func TestDictDeclAndEnumDeclInSameFile(t *testing.T) {
	diag := codegen.NewDiagnostics()

	enum := codegen.NewEnumDecl("Status", []string{"open", "closed"}, diag)
	dict := codegen.NewDictDecl("Event", "", []codegen.DictField{
		{IDLName: "status", GoType: "Status", Optional: false},
	}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(enum)
	f.AddDecl(dict)

	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render with enum+dict: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "type Status string") {
		t.Errorf("enum type missing from combined render:\n%s", src)
	}
	if !strings.Contains(src, "type Event struct") {
		t.Errorf("dict type missing from combined render:\n%s", src)
	}
}

func TestDictDeclDuplicateTypeNameInFileRendersError(t *testing.T) {
	// "readyState" and "ready-state" both sanitize to "ReadyState"
	diag := codegen.NewDiagnostics()
	d1 := codegen.NewDictDecl("readyState", "", nil, diag)
	d2 := codegen.NewDictDecl("ready-state", "", nil, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(d1)
	f.AddDecl(d2)

	_, err := f.Render()
	if err == nil {
		t.Error("File.Render must return an error for duplicate DictDecl type names")
	}
}

func TestDictDeclDeclNameIsTypeName(t *testing.T) {
	// declName() must return the Go struct name so File.Render duplicate detection works
	diag := codegen.NewDiagnostics()
	decl := codegen.NewDictDecl("EventInit", "", nil, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(decl)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "type EventInit struct") {
		t.Errorf("declName must match rendered type name:\n%s", out)
	}
}

func TestDictDeclTwoDistinctDictsInOneFile(t *testing.T) {
	diag := codegen.NewDiagnostics()
	d1 := codegen.NewDictDecl("Alpha", "", []codegen.DictField{
		{IDLName: "x", GoType: "string", Optional: false},
	}, diag)
	d2 := codegen.NewDictDecl("Beta", "", []codegen.DictField{
		{IDLName: "y", GoType: "int32", Optional: true},
	}, diag)

	f := codegen.NewFile("gen")
	f.AddDecl(d1)
	f.AddDecl(d2)

	out, err := f.Render()
	if err != nil {
		t.Fatalf("two distinct DictDecls must render without error: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "type Alpha struct") {
		t.Errorf("Alpha missing:\n%s", src)
	}
	if !strings.Contains(src, "type Beta struct") {
		t.Errorf("Beta missing:\n%s", src)
	}
}

// normalizeSpaces collapses each run of spaces and tabs to a single space so
// that "FieldName Type" assertions survive gofmt's column alignment in
// multi-field structs. Newlines are preserved.
var spaceRun = regexp.MustCompile(`[ \t]+`)

func normalizeSpaces(s string) string {
	return spaceRun.ReplaceAllString(s, " ")
}
