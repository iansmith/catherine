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
