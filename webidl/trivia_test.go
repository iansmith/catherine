package webidl

import "testing"

// ---------------------------------------------------------------------------
// Edge / boundary cases
// ---------------------------------------------------------------------------

// TestTokenOffsetNonZero verifies that a token not at the start of the source
// has a non-zero Offset matching its actual byte position.
// "interface" (9 bytes) + " " (1) = "Foo" at offset 10.
func TestTokenOffsetNonZero(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};"
	tokens, err := Tokenize(src)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	var fooTok *Token
	for i := range tokens {
		if tokens[i].Value == "Foo" {
			fooTok = &tokens[i]
			break
		}
	}
	if fooTok == nil {
		t.Fatal("no 'Foo' token found in token stream")
	}
	if fooTok.Offset != 10 {
		t.Errorf("expected Foo.Offset=10, got %d", fooTok.Offset)
	}
}

// TestTokenOffsetAfterLeadingWhitespace verifies that leading whitespace shifts
// the first meaningful token's Offset — it must NOT be 0 when trivia precedes it.
func TestTokenOffsetAfterLeadingWhitespace(t *testing.T) {
	t.Parallel()
	src := "   interface Foo {};"
	tokens, err := Tokenize(src)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) == 0 || tokens[0].Kind == TokEOF {
		t.Fatal("expected at least one non-EOF token")
	}
	// "interface" starts at byte 3 (three leading spaces).
	if tokens[0].Value != "interface" {
		t.Fatalf("expected first token 'interface', got %q", tokens[0].Value)
	}
	if tokens[0].Offset != 3 {
		t.Errorf("expected Offset=3 for 'interface' with 3 leading spaces, got %d", tokens[0].Offset)
	}
}

// TestPrintIDLWhitespaceOnly verifies that a source string that is entirely
// whitespace (no definitions) round-trips to the original bytes.
func TestPrintIDLWhitespaceOnly(t *testing.T) {
	t.Parallel()
	src := "   \n\t  \n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("whitespace-only round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestPrintIDLCommentOnly verifies that a source string consisting entirely of
// a comment (and optional surrounding whitespace) round-trips exactly.
func TestPrintIDLCommentOnly(t *testing.T) {
	t.Parallel()
	src := "// just a comment\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("comment-only round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestPrintIDLNoTrailingNewline verifies that a source string without a
// trailing newline is reproduced exactly — the printer must not append one.
func TestPrintIDLNoTrailingNewline(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {};"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("no-trailing-newline round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestTokenOffsetMatchesSource verifies that for every non-EOF token, the
// byte at src[token.Offset] matches the first byte of token.Value.
// This is a universal invariant — any offset mis-assignment breaks it.
// The corpus intentionally includes a TokString token ("red") to cover the
// string-literal emit path in addition to keywords and punctuation.
func TestTokenOffsetMatchesSource(t *testing.T) {
	t.Parallel()
	src := `[Exposed=Window]
interface Foo {
  attribute long x;
};
enum Color { "red" };
`
	tokens, err := Tokenize(src)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	for _, tok := range tokens {
		if tok.Kind == TokEOF {
			continue
		}
		if tok.Offset < 0 || tok.Offset >= len(src) {
			t.Errorf("token %q: Offset=%d is out of range [0, %d)", tok.Value, tok.Offset, len(src))
			continue
		}
		if src[tok.Offset] != tok.Value[0] {
			t.Errorf("token %q: src[%d]=%q, want %q (first byte of Value)",
				tok.Value, tok.Offset, string(src[tok.Offset]), string(tok.Value[0]))
		}
	}
}

// ---------------------------------------------------------------------------
// Cross-feature interaction cases
// ---------------------------------------------------------------------------

// TestRoundTripInlineComment verifies that a single-line comment between
// tokens is preserved byte-for-byte in the printed output.
func TestRoundTripInlineComment(t *testing.T) {
	t.Parallel()
	src := `// A comment before the interface
[Exposed=Window]
interface Foo {
  // inline member comment
  attribute long x;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("inline-comment round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestRoundTripBlockComment verifies that block comments are preserved
// byte-for-byte, including multi-line block comments.
func TestRoundTripBlockComment(t *testing.T) {
	t.Parallel()
	src := `/*
 * Block comment.
 */
[Exposed=Window]
interface Foo {
  attribute long /* inline block */ x;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("block-comment round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestRoundTripMultipleDefinitions verifies that inter-definition whitespace
// and comments are preserved when multiple definitions are present.
func TestRoundTripMultipleDefinitions(t *testing.T) {
	t.Parallel()
	src := `[Exposed=Window]
interface A {
  attribute long x;
};

// Between defs.

[Exposed=Window]
interface B {
  attribute long y;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("multi-definition round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestRoundTripWithExtAttrs verifies that extended-attribute syntax
// round-trips without loss — `[Exposed=Window, SecureContext]` must not be
// reformatted.
func TestRoundTripWithExtAttrs(t *testing.T) {
	t.Parallel()
	src := `[Exposed=Window, SecureContext]
interface Foo {
  attribute long x;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("ext-attr round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestRoundTripDictionary verifies that a dictionary definition round-trips
// (exercising a different definition kind from the interface tests above).
func TestRoundTripDictionary(t *testing.T) {
	t.Parallel()
	src := `dictionary Options {
  required long width;
  long height = 0;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("dictionary round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// ---------------------------------------------------------------------------
// Additional adversary gap tests (findings 1–5)
// ---------------------------------------------------------------------------

// TestTokenOffsetEOFBoundary verifies that the synthetic EOF token's Offset
// equals len(src) — marking the end of the source exactly.
func TestTokenOffsetEOFBoundary(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};"
	tokens, err := Tokenize(src)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	eof := tokens[len(tokens)-1]
	if eof.Kind != TokEOF {
		t.Fatalf("last token is not TokEOF: %v", eof.Kind)
	}
	if eof.Offset != len(src) {
		t.Errorf("EOF.Offset=%d, want %d (=len(src))", eof.Offset, len(src))
	}
}

// TestTokenOffsetSingleTokenLeadingSpace verifies that a single meaningful
// token preceded only by whitespace has an Offset equal to the whitespace
// byte count — not 0.
func TestTokenOffsetSingleTokenLeadingSpace(t *testing.T) {
	t.Parallel()
	src := "   Foo"
	tokens, err := Tokenize(src)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if tokens[0].Value != "Foo" {
		t.Fatalf("expected first token 'Foo', got %q", tokens[0].Value)
	}
	if tokens[0].Offset != 3 {
		t.Errorf("expected 'Foo'.Offset=3 (three leading spaces), got %d", tokens[0].Offset)
	}
}

// TestTokenOffsetMultipleOccurrences verifies that two tokens with the same
// value that appear at different positions in the source have distinct Offsets
// that each correctly point to their respective byte positions.
func TestTokenOffsetMultipleOccurrences(t *testing.T) {
	t.Parallel()
	// Two interfaces each with an attribute named "x" — two "x" tokens at
	// different byte offsets.
	src := "[Exposed=Window]\ninterface A {\n  attribute long x;\n};\n" +
		"[Exposed=Window]\ninterface B {\n  attribute long x;\n};\n"
	tokens, err := Tokenize(src)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	var xOffsets []int
	for _, tok := range tokens {
		if tok.Value == "x" {
			xOffsets = append(xOffsets, tok.Offset)
		}
	}
	if len(xOffsets) != 2 {
		t.Fatalf("expected 2 'x' tokens, got %d", len(xOffsets))
	}
	if xOffsets[0] == xOffsets[1] {
		t.Errorf("both 'x' tokens have the same Offset=%d; expected distinct offsets", xOffsets[0])
	}
	// Each offset must actually point to 'x' in src.
	for _, off := range xOffsets {
		if off < 0 || off >= len(src) || src[off] != 'x' {
			t.Errorf("offset %d does not point to 'x' in source (src[%d]=%q)", off, off, string(src[off]))
		}
	}
}

// TestRoundTripTypedef verifies that a typedef definition round-trips
// byte-for-byte — exercising a definition kind not covered by the interface
// and dictionary tests above.
func TestRoundTripTypedef(t *testing.T) {
	t.Parallel()
	src := "typedef unsigned long long DOMTimeStamp;\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("typedef round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestTokenOffsetUTF8InComment verifies that multi-byte UTF-8 sequences in a
// comment do not shift the Offset of subsequent tokens. A byte-naive
// implementation that counts runes instead of bytes would place "interface" at
// offset 8 instead of 9 for the source below.
//
// "// café\n" is 9 bytes:
//
//	"//"   = 2 bytes (ASCII)
//	" "    = 1 byte  (ASCII space)
//	"café" = 5 bytes (c=1, a=1, f=1, é=2 as [0xC3,0xA9])
//	"\n"   = 1 byte
func TestTokenOffsetUTF8InComment(t *testing.T) {
	t.Parallel()
	src := "// café\ninterface Foo {};"
	tokens, err := Tokenize(src)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if tokens[0].Value != "interface" {
		t.Fatalf("expected first token 'interface', got %q", tokens[0].Value)
	}
	const wantOffset = 9 // "// café\n" is exactly 9 bytes in UTF-8
	if tokens[0].Offset != wantOffset {
		t.Errorf("expected 'interface'.Offset=%d (UTF-8 byte offset), got %d", wantOffset, tokens[0].Offset)
	}
}

// ---------------------------------------------------------------------------
// Nil / edge-input cases
// ---------------------------------------------------------------------------

// TestPrintIDLNilDefinitions verifies that PrintIDL tolerates a nil defs slice.
// defs is currently unused, so nil must not panic or produce empty output.
func TestPrintIDLNilDefinitions(t *testing.T) {
	t.Parallel()
	src := "// A comment\n[Exposed=Window]\ninterface Foo {};\n"
	got := PrintIDL(src, nil)
	if got != src {
		t.Errorf("nil-defs PrintIDL failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestPrintIDLMalformedSource verifies that PrintIDL reproduces the source
// bytes for lexically valid but structurally incomplete IDL — it must not
// panic or return empty string just because the AST is missing.
// (The tokenizer can still lex an incomplete interface; PrintIDL should
// reconstruct verbatim using the token offsets.)
func TestPrintIDLMalformedSource(t *testing.T) {
	t.Parallel()
	// Lexically valid tokens but an incomplete interface definition.
	src := "interface Foo { attribute long"
	got := PrintIDL(src, nil)
	if got != src {
		t.Errorf("malformed-source PrintIDL failed\ngot:  %q\nwant: %q", got, src)
	}
}

// ---------------------------------------------------------------------------
// Write path: modified-AST printing (CATH-27)
// ---------------------------------------------------------------------------

// TestPrintIDLChangedNameLonger verifies the write path when Interface.Name is
// replaced with a longer identifier. The extra bytes must not shift or corrupt
// any surrounding trivia.
func TestPrintIDLChangedNameLonger(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "VeryLongInterfaceName"
	got := PrintIDL(src, defs)
	want := "interface VeryLongInterfaceName {};\n"
	if got != want {
		t.Errorf("longer-name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNameShorter verifies the write path when Interface.Name is
// replaced with a shorter identifier — the closing tokens must not be truncated.
func TestPrintIDLChangedNameShorter(t *testing.T) {
	t.Parallel()
	src := "interface FooBarBaz {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "X"
	got := PrintIDL(src, defs)
	want := "interface X {};\n"
	if got != want {
		t.Errorf("shorter-name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNameWithExtAttrs verifies that the write path correctly
// skips the extended-attribute block when locating the name token — a
// definition preceded by [Exposed=Window] must still have its name replaced.
func TestPrintIDLChangedNameWithExtAttrs(t *testing.T) {
	t.Parallel()
	src := "[Exposed=Window]\ninterface Foo {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Bar"
	got := PrintIDL(src, defs)
	want := "[Exposed=Window]\ninterface Bar {};\n"
	if got != want {
		t.Errorf("ext-attr name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNameOnlyOneOfTwo verifies that modifying one interface
// name in a two-definition file leaves the other definition byte-for-byte
// identical — trivia between definitions must also be preserved.
func TestPrintIDLChangedNameOnlyOneOfTwo(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};\n\n// separator\n\ninterface Bar {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Renamed"
	got := PrintIDL(src, defs)
	want := "interface Renamed {};\n\n// separator\n\ninterface Bar {};\n"
	if got != want {
		t.Errorf("partial-rename write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedInterfaceNamePreservesTrivia is the canonical DoD test:
// a file with a leading comment, an interface with a member, and a trailing
// newline. Changing Interface.Name must produce the new name in the output
// while keeping all surrounding whitespace and comments intact.
func TestPrintIDLChangedInterfaceNamePreservesTrivia(t *testing.T) {
	t.Parallel()
	src := "// header\ninterface Foo {\n  attribute long x;\n};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Bar"
	got := PrintIDL(src, defs)
	want := "// header\ninterface Bar {\n  attribute long x;\n};\n"
	if got != want {
		t.Errorf("trivia-preserve write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedDictionaryName verifies the write path for Dictionary
// definitions — the same mechanism that locates Interface.Name must also work
// for the dictionary keyword.
func TestPrintIDLChangedDictionaryName(t *testing.T) {
	t.Parallel()
	src := "dictionary Options {\n  long width;\n};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Dictionary).Name = "Config"
	got := PrintIDL(src, defs)
	want := "dictionary Config {\n  long width;\n};\n"
	if got != want {
		t.Errorf("dictionary name write: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Write path: adversary gap tests (CATH-27)
// ---------------------------------------------------------------------------

// TestPrintIDLChangedEnumName verifies the write path for Enum definitions.
// An enum body contains TokString values that must not be mistaken for the
// name token when scanning forward from the keyword position.
func TestPrintIDLChangedEnumName(t *testing.T) {
	t.Parallel()
	src := "enum Color {\n  \"red\",\n  \"green\"\n};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Enum).Name = "Palette"
	got := PrintIDL(src, defs)
	want := "enum Palette {\n  \"red\",\n  \"green\"\n};\n"
	if got != want {
		t.Errorf("enum name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedTypedefName verifies the write path for Typedef.
// The name token appears after the type expression, not immediately after the
// keyword — a naive "first identifier after keyword" scan would misidentify it.
func TestPrintIDLChangedTypedefName(t *testing.T) {
	t.Parallel()
	src := "typedef unsigned long long DOMTimeStamp;\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Typedef).Name = "EpochMs"
	got := PrintIDL(src, defs)
	want := "typedef unsigned long long EpochMs;\n"
	if got != want {
		t.Errorf("typedef name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNamespaceName verifies the write path for Namespace.
// A missing case in the implementation's type switch would silently emit the
// original name with no error.
func TestPrintIDLChangedNamespaceName(t *testing.T) {
	t.Parallel()
	src := "namespace Math {\n  readonly attribute double PI;\n};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Namespace).Name = "Numerics"
	got := PrintIDL(src, defs)
	want := "namespace Numerics {\n  readonly attribute double PI;\n};\n"
	if got != want {
		t.Errorf("namespace name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedCallbackFunctionName verifies the write path for
// CallbackFunction. The keyword is "callback" (shared with callback interface)
// and the name immediately precedes a "=" token.
func TestPrintIDLChangedCallbackFunctionName(t *testing.T) {
	t.Parallel()
	src := "callback EventHandler = undefined (Event event);\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*CallbackFunction).Name = "OnEvent"
	got := PrintIDL(src, defs)
	want := "callback OnEvent = undefined (Event event);\n"
	if got != want {
		t.Errorf("callback-function name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedIncludesTarget verifies renaming the left-hand (target)
// name in an includes statement.
func TestPrintIDLChangedIncludesTarget(t *testing.T) {
	t.Parallel()
	src := "WindowProxy includes WindowOrWorkerGlobalScope;\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Includes).Target = "WorkerProxy"
	got := PrintIDL(src, defs)
	want := "WorkerProxy includes WindowOrWorkerGlobalScope;\n"
	if got != want {
		t.Errorf("includes target write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedIncludesMixin verifies renaming the right-hand (mixin)
// name in an includes statement — a distinct token that Span.Offset does not
// point to, requiring a separate scan strategy.
func TestPrintIDLChangedIncludesMixin(t *testing.T) {
	t.Parallel()
	src := "WindowProxy includes WindowOrWorkerGlobalScope;\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Includes).Includes = "GlobalMixin"
	got := PrintIDL(src, defs)
	want := "WindowProxy includes GlobalMixin;\n"
	if got != want {
		t.Errorf("includes mixin write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedInheritance verifies that Interface.Inheritance can be
// replaced while leaving Interface.Name and all trivia intact.
func TestPrintIDLChangedInheritance(t *testing.T) {
	t.Parallel()
	src := "interface Foo : OldBase {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Inheritance = "NewBase"
	got := PrintIDL(src, defs)
	want := "interface Foo : NewBase {};\n"
	if got != want {
		t.Errorf("inheritance write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNameWithInheritance verifies that changing Interface.Name
// does not accidentally clobber the inheritance name when both are present.
func TestPrintIDLChangedNameWithInheritance(t *testing.T) {
	t.Parallel()
	src := "interface Foo : Base {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Bar"
	got := PrintIDL(src, defs)
	want := "interface Bar : Base {};\n"
	if got != want {
		t.Errorf("name-only write with inheritance: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNameAndInheritanceSimultaneously verifies that two
// replacements within the same definition token span are both applied correctly
// without offset-delta interference.
func TestPrintIDLChangedNameAndInheritanceSimultaneously(t *testing.T) {
	t.Parallel()
	src := "interface Foo : OldBase {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Bar"
	defs[0].(*Interface).Inheritance = "NewBase"
	got := PrintIDL(src, defs)
	want := "interface Bar : NewBase {};\n"
	if got != want {
		t.Errorf("name+inheritance simultaneous write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNameToKeywordString verifies that a replacement name that
// is a WebIDL keyword is emitted verbatim — the implementation must not
// re-classify or skip it based on token kind.
func TestPrintIDLChangedNameToKeywordString(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "long"
	got := PrintIDL(src, defs)
	want := "interface long {};\n"
	if got != want {
		t.Errorf("keyword-as-name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLNoOpRename verifies that setting Interface.Name to its current
// value produces output byte-for-byte identical to the unmodified source.
func TestPrintIDLNoOpRename(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Foo"
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("no-op rename: got %q, want %q", got, src)
	}
}

// TestPrintIDLChangedPartialInterfaceName verifies the write path for a partial
// interface. The "partial" keyword precedes "interface", adding one more token
// between Span.Offset and the name identifier.
func TestPrintIDLChangedPartialInterfaceName(t *testing.T) {
	t.Parallel()
	src := "partial interface Foo {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Bar"
	got := PrintIDL(src, defs)
	want := "partial interface Bar {};\n"
	if got != want {
		t.Errorf("partial interface name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedMixinName verifies the write path for interface mixin.
// There is an extra "mixin" keyword token between "interface" and the name.
func TestPrintIDLChangedMixinName(t *testing.T) {
	t.Parallel()
	src := "interface mixin Foo {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Bar"
	got := PrintIDL(src, defs)
	want := "interface mixin Bar {};\n"
	if got != want {
		t.Errorf("mixin name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedCallbackInterfaceName verifies the write path for
// callback interface — distinct from CallbackFunction despite the shared
// "callback" keyword prefix.
func TestPrintIDLChangedCallbackInterfaceName(t *testing.T) {
	t.Parallel()
	src := "callback interface EventListener {\n  undefined handleEvent(Event event);\n};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Observer"
	got := PrintIDL(src, defs)
	want := "callback interface Observer {\n  undefined handleEvent(Event event);\n};\n"
	if got != want {
		t.Errorf("callback interface name write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNameOnlySecondOfTwo verifies that modifying defs[1] in a
// two-definition file applies the replacement correctly while leaving defs[0]
// byte-for-byte identical. A directional offset-accumulation bug would corrupt
// defs[1]'s position when defs[0] is unchanged.
func TestPrintIDLChangedNameOnlySecondOfTwo(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};\n\n// separator\n\ninterface Bar {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[1].(*Interface).Name = "Renamed"
	got := PrintIDL(src, defs)
	want := "interface Foo {};\n\n// separator\n\ninterface Renamed {};\n"
	if got != want {
		t.Errorf("second-of-two rename: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedBothNamesInTwoDefinitions verifies that two simultaneous
// name replacements in the same PrintIDL call both land correctly. An
// offset-accumulation bug would corrupt the second replacement's position.
func TestPrintIDLChangedBothNamesInTwoDefinitions(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};\ninterface Bar {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "Alpha"
	defs[1].(*Interface).Name = "Beta"
	got := PrintIDL(src, defs)
	want := "interface Alpha {};\ninterface Beta {};\n"
	if got != want {
		t.Errorf("both-renamed write: got %q, want %q", got, want)
	}
}

// TestPrintIDLChangedNameWithLeadingUnderscore verifies that a replacement
// name starting with "_" (a legal WebIDL escape prefix) is emitted verbatim.
func TestPrintIDLChangedNameWithLeadingUnderscore(t *testing.T) {
	t.Parallel()
	src := "interface Foo {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defs[0].(*Interface).Name = "_Foo"
	got := PrintIDL(src, defs)
	want := "interface _Foo {};\n"
	if got != want {
		t.Errorf("underscore-prefix name write: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Happy-path cases
// ---------------------------------------------------------------------------

// TestRoundTripSimpleInterface is the canonical happy-path: a single interface
// with one member and standard formatting must reproduce exactly.
func TestRoundTripSimpleInterface(t *testing.T) {
	t.Parallel()
	src := `[Exposed=Window]
interface Foo {
  attribute long x;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("simple-interface round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}

// TestRoundTripEscapedIdentifier verifies that a definition whose name is an
// escaped identifier (_foo) round-trips byte-for-byte. The parser unescapes the
// leading '_' when storing d.Name; recordSub must not treat this as a change.
func TestRoundTripEscapedIdentifier(t *testing.T) {
	t.Parallel()
	src := "interface _reserved {};\n"
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("escaped-identifier round-trip: got %q, want %q", got, src)
	}
}

// TestRoundTripEnum verifies that an enum definition (value strings, commas,
// braces) round-trips exactly — exercising the string-literal token path.
func TestRoundTripEnum(t *testing.T) {
	t.Parallel()
	src := `enum Color {
  "red",
  "green",
  "blue"
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := PrintIDL(src, defs)
	if got != src {
		t.Errorf("enum round-trip failed\ngot:  %q\nwant: %q", got, src)
	}
}
