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
func TestTokenOffsetMatchesSource(t *testing.T) {
	t.Parallel()
	src := `[Exposed=Window]
interface Foo {
  attribute long x;
};
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
// Error-path cases (PrintIDL with nil/unparseable input)
// ---------------------------------------------------------------------------

// TestPrintIDLNilDefinitions verifies that PrintIDL returns the original source
// when defs is nil — i.e. when the caller had a parse error and passes nil.
// The entire source is treated as trivia and must be preserved verbatim.
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
