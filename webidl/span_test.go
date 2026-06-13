package webidl

import "testing"

// ---------------------------------------------------------------------------
// Edge / boundary cases
// ---------------------------------------------------------------------------

// TestSpanLeadingWhitespace verifies that leading whitespace shifts the
// definition's Span to the actual first token — not byte 0.
func TestSpanLeadingWhitespace(t *testing.T) {
	t.Parallel()
	src := "\n\n[Exposed=Window]\ninterface Foo {};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	// Two leading newlines; first real token is '[' at line 3, offset 2.
	if iface.Span.Line != 3 {
		t.Errorf("Span.Line = %d, want 3", iface.Span.Line)
	}
	if iface.Span.Offset != 2 {
		t.Errorf("Span.Offset = %d, want 2 (two leading newlines)", iface.Span.Offset)
	}
}

// TestSpanExtAttrCountsAsStart verifies that when a definition has extended
// attributes, the Span points to the opening '[', not the keyword that follows.
func TestSpanExtAttrCountsAsStart(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	// '[' is the very first byte.
	if iface.Span.Line != 1 {
		t.Errorf("Span.Line = %d, want 1", iface.Span.Line)
	}
	if iface.Span.Offset != 0 {
		t.Errorf("Span.Offset = %d, want 0", iface.Span.Offset)
	}
}

// TestSpanNoExtAttrKeyword verifies that when a definition has no extended
// attributes, the Span points to the defining keyword itself.
func TestSpanNoExtAttrKeyword(t *testing.T) {
	t.Parallel()
	src := "   interface Foo {};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	// 3 leading spaces; "interface" starts at offset 3, line 1.
	if iface.Span.Line != 1 {
		t.Errorf("Span.Line = %d, want 1", iface.Span.Line)
	}
	if iface.Span.Offset != 3 {
		t.Errorf("Span.Offset = %d, want 3", iface.Span.Offset)
	}
}

// TestSpanOffsetMatchesTokenOffset verifies that the definition Span.Offset
// agrees with the byte offset recorded on the first token by the tokenizer.
// This cross-checks the two offset systems are consistent.
func TestSpanOffsetMatchesTokenOffset(t *testing.T) {
	t.Parallel()
	src := "/* preamble */\ninterface Foo {};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	tokens, err := Tokenize(src)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	// First non-EOF token should be "interface" and its Offset should match
	// the Span.Offset on the Interface node.
	if len(tokens) == 0 || tokens[0].Kind == TokEOF {
		t.Fatal("no tokens")
	}
	if iface.Span.Offset != tokens[0].Offset {
		t.Errorf("Interface.Span.Offset=%d, first token Offset=%d; want equal",
			iface.Span.Offset, tokens[0].Offset)
	}
}

// ---------------------------------------------------------------------------
// Definition-kind coverage
// ---------------------------------------------------------------------------

// TestSpanDictionary verifies that a dictionary definition carries a correct Span.
func TestSpanDictionary(t *testing.T) {
	t.Parallel()
	src := "\ndictionary Options { required long width; };"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d, ok := defs[0].(*Dictionary)
	if !ok {
		t.Fatalf("expected *Dictionary, got %T", defs[0])
	}
	// One leading newline; "dictionary" starts at line 2, offset 1.
	if d.Span.Line != 2 {
		t.Errorf("Span.Line = %d, want 2", d.Span.Line)
	}
	if d.Span.Offset != 1 {
		t.Errorf("Span.Offset = %d, want 1", d.Span.Offset)
	}
}

// TestSpanEnum verifies that an enum definition carries a correct Span.
func TestSpanEnum(t *testing.T) {
	t.Parallel()
	src := `enum Color { "red" };`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	e, ok := defs[0].(*Enum)
	if !ok {
		t.Fatalf("expected *Enum, got %T", defs[0])
	}
	if e.Span.Line != 1 {
		t.Errorf("Span.Line = %d, want 1", e.Span.Line)
	}
	if e.Span.Offset != 0 {
		t.Errorf("Span.Offset = %d, want 0", e.Span.Offset)
	}
}

// TestSpanTypedef verifies that a typedef carries a correct Span.
func TestSpanTypedef(t *testing.T) {
	t.Parallel()
	src := "  typedef unsigned long long DOMTimeStamp;"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	td, ok := defs[0].(*Typedef)
	if !ok {
		t.Fatalf("expected *Typedef, got %T", defs[0])
	}
	// 2 leading spaces; "typedef" starts at offset 2, line 1.
	if td.Span.Offset != 2 {
		t.Errorf("Span.Offset = %d, want 2", td.Span.Offset)
	}
}

// TestSpanIncludes verifies that an includes statement carries a correct Span.
func TestSpanIncludes(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {};\n  Foo includes EventTarget;"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var inc *Includes
	for _, d := range defs {
		if x, ok := d.(*Includes); ok {
			inc = x
			break
		}
	}
	if inc == nil {
		t.Fatal("no *Includes found")
	}
	// "Foo" starts at line 3, offset 2 (two leading spaces on line 3).
	if inc.Span.Line != 3 {
		t.Errorf("Span.Line = %d, want 3", inc.Span.Line)
	}
}

// TestSpanNamespace verifies that a namespace carries a correct Span.
func TestSpanNamespace(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\nnamespace Math { readonly attribute double PI; };"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ns, ok := defs[0].(*Namespace)
	if !ok {
		t.Fatalf("expected *Namespace, got %T", defs[0])
	}
	if ns.Span.Line != 1 {
		t.Errorf("Span.Line = %d, want 1", ns.Span.Line)
	}
	if ns.Span.Offset != 0 {
		t.Errorf("Span.Offset = %d, want 0", ns.Span.Offset)
	}
}

// TestSpanCallbackFunction verifies that a callback function carries a correct Span.
func TestSpanCallbackFunction(t *testing.T) {
	t.Parallel()
	src := " callback EventHandler = undefined (Event event);"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cb, ok := defs[0].(*CallbackFunction)
	if !ok {
		t.Fatalf("expected *CallbackFunction, got %T", defs[0])
	}
	// 1 leading space; "callback" at offset 1.
	if cb.Span.Offset != 1 {
		t.Errorf("Span.Offset = %d, want 1", cb.Span.Offset)
	}
}

// ---------------------------------------------------------------------------
// Member span tests
// ---------------------------------------------------------------------------

// TestSpanMemberAttribute verifies that an attribute member carries a Span
// pointing to the start of that member's first token within the interface body.
func TestSpanMemberAttribute(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {\n  attribute long x;\n};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	if len(iface.Members) == 0 {
		t.Fatal("expected at least one member")
	}
	attr, ok := iface.Members[0].(*Attribute)
	if !ok {
		t.Fatalf("expected *Attribute member, got %T", iface.Members[0])
	}
	// "attribute" is on line 3.
	if attr.Span.Line != 3 {
		t.Errorf("Attribute.Span.Line = %d, want 3", attr.Span.Line)
	}
}

// TestSpanMemberOperation verifies that an operation member carries a correct Span.
func TestSpanMemberOperation(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {\n  undefined doSomething();\n};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface := defs[0].(*Interface)
	op, ok := iface.Members[0].(*Operation)
	if !ok {
		t.Fatalf("expected *Operation, got %T", iface.Members[0])
	}
	if op.Span.Line != 3 {
		t.Errorf("Operation.Span.Line = %d, want 3", op.Span.Line)
	}
}

// TestSpanMemberAttributeWithExtAttr verifies that when a member has extended
// attributes, the Span points to the opening '[', not to the "attribute" keyword.
func TestSpanMemberAttributeWithExtAttr(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {\n  [LegacyUnforgeable] readonly attribute DOMString id;\n};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	if len(iface.Members) == 0 {
		t.Fatal("expected at least one member")
	}
	attr, ok := iface.Members[0].(*Attribute)
	if !ok {
		t.Fatalf("expected *Attribute, got %T", iface.Members[0])
	}
	// '[LegacyUnforgeable]' is the first token on line 3, at offset 2 (two spaces).
	if attr.Span.Line != 3 {
		t.Errorf("Span.Line = %d, want 3 (line with [LegacyUnforgeable])", attr.Span.Line)
	}
	if attr.Span.Offset != 2 {
		t.Errorf("Span.Offset = %d, want 2 (the opening '[')", attr.Span.Offset)
	}
}

// TestSpanDictionaryField verifies that a dictionary field (a Member-level
// *Field node) carries a correct Span pointing to its first token.
func TestSpanDictionaryField(t *testing.T) {
	t.Parallel()
	src := "dictionary Options { required long width; };"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d, ok := defs[0].(*Dictionary)
	if !ok {
		t.Fatalf("expected *Dictionary, got %T", defs[0])
	}
	if len(d.Members) != 1 {
		t.Fatalf("expected exactly 1 field, got %d", len(d.Members))
	}
	field := d.Members[0]
	// "required" starts at offset 21 ("dictionary Options { " = 21 bytes).
	if field.Span.Line != 1 {
		t.Errorf("Field.Span.Line = %d, want 1", field.Span.Line)
	}
	if field.Span.Offset != 21 {
		t.Errorf("Field.Span.Offset = %d, want 21 (start of 'required')", field.Span.Offset)
	}
}

// TestSpanOperationSpecialKeyword verifies that a getter operation carries a
// Span starting from the "getter" special keyword, not the return type.
func TestSpanOperationSpecialKeyword(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {\n  getter DOMString (unsigned long index);\n};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	if len(iface.Members) == 0 {
		t.Fatal("expected at least one member")
	}
	op, ok := iface.Members[0].(*Operation)
	if !ok {
		t.Fatalf("expected *Operation, got %T", iface.Members[0])
	}
	// "getter" is the first token on line 3, at offset 2.
	if op.Span.Line != 3 {
		t.Errorf("Operation.Span.Line = %d, want 3", op.Span.Line)
	}
	if op.Span.Offset != 2 {
		t.Errorf("Operation.Span.Offset = %d, want 2 (start of 'getter')", op.Span.Offset)
	}
}

// TestSpanMemberOffsetMatchesToken cross-checks that member Span.Offset values
// match the byte offsets recorded by the tokenizer for those same tokens.
// A broken implementation that returns zero for all member Spans would fail here.
func TestSpanMemberOffsetMatchesToken(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {\n  attribute long x;\n  undefined doSomething();\n};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	if len(iface.Members) < 2 {
		t.Fatalf("expected at least 2 members, got %d", len(iface.Members))
	}
	attr, ok := iface.Members[0].(*Attribute)
	if !ok {
		t.Fatalf("expected *Attribute, got %T", iface.Members[0])
	}
	op, ok := iface.Members[1].(*Operation)
	if !ok {
		t.Fatalf("expected *Operation, got %T", iface.Members[1])
	}
	// Distinct members must have distinct offsets.
	if attr.Span.Offset == op.Span.Offset {
		t.Errorf("attribute and operation share Span.Offset=%d; expected distinct", attr.Span.Offset)
	}
	// Both offsets must be valid indices into src.
	if attr.Span.Offset < 0 || attr.Span.Offset >= len(src) {
		t.Errorf("Attribute.Span.Offset=%d out of bounds [0, %d)", attr.Span.Offset, len(src))
	}
	if op.Span.Offset < 0 || op.Span.Offset >= len(src) {
		t.Errorf("Operation.Span.Offset=%d out of bounds [0, %d)", op.Span.Offset, len(src))
	}
	// The byte at each offset must match the first byte of that member's first token.
	if src[attr.Span.Offset] != 'a' { // "attribute"
		t.Errorf("src[attr.Span.Offset=%d] = %q, want 'a' (start of 'attribute')",
			attr.Span.Offset, string(src[attr.Span.Offset]))
	}
	if src[op.Span.Offset] != 'u' { // "undefined"
		t.Errorf("src[op.Span.Offset=%d] = %q, want 'u' (start of 'undefined')",
			op.Span.Offset, string(src[op.Span.Offset]))
	}
}

// TestSpanMemberConstant verifies that a constant member carries a correct Span
// pointing to the "const" keyword.
func TestSpanMemberConstant(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {\n  const long MAX_SIZE = 100;\n};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	iface, ok := defs[0].(*Interface)
	if !ok {
		t.Fatalf("expected *Interface, got %T", defs[0])
	}
	if len(iface.Members) == 0 {
		t.Fatal("expected at least one member")
	}
	c, ok := iface.Members[0].(*Constant)
	if !ok {
		t.Fatalf("expected *Constant, got %T", iface.Members[0])
	}
	// "const" keyword is on line 3, at offset 2 (two spaces of indent).
	if c.Span.Line != 3 {
		t.Errorf("Constant.Span.Line = %d, want 3", c.Span.Line)
	}
	if c.Span.Offset != 2 {
		t.Errorf("Constant.Span.Offset = %d, want 2 (start of 'const')", c.Span.Offset)
	}
}

// TestSpanMultipleDefinitionsDistinct verifies that two definitions in the same
// source file carry distinct, non-zero Spans that both point into the source.
func TestSpanMultipleDefinitionsDistinct(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface A {};\n[Exposed=Window]\ninterface B {};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}
	a := defs[0].(*Interface)
	b := defs[1].(*Interface)
	if a.Span.Offset == b.Span.Offset {
		t.Errorf("both interfaces have the same Span.Offset=%d; expected distinct",
			a.Span.Offset)
	}
	if a.Span.Offset != 0 {
		t.Errorf("A.Span.Offset = %d, want 0", a.Span.Offset)
	}
	// B's ext-attr '[' is on line 3, after "interface A {};\n".
	if b.Span.Line != 3 {
		t.Errorf("B.Span.Line = %d, want 3", b.Span.Line)
	}
}
