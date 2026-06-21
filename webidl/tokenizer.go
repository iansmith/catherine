package webidl

import (
	"fmt"
	"strings"
)

// TypeNameKeywords are reserved identifiers that act as type names.
// Mirrors webidl2.js's typeNameKeywords.
var TypeNameKeywords = []string{
	"ArrayBuffer",
	"SharedArrayBuffer",
	"DataView",
	"Int8Array",
	"Int16Array",
	"Int32Array",
	"Uint8Array",
	"Uint16Array",
	"Uint32Array",
	"Uint8ClampedArray",
	"BigInt64Array",
	"BigUint64Array",
	"Float16Array",
	"Float32Array",
	"Float64Array",
	"any",
	"object",
	"symbol",
}

// StringTypes are the three built-in IDL string types.
var StringTypes = []string{"ByteString", "DOMString", "USVString"}

// Type-modifier extended attribute names. These attributes appear on IDLType
// nodes and affect codegen semantics without changing the Go type name.
const (
	ExtAttrClamp        = "Clamp"
	ExtAttrEnforceRange = "EnforceRange"
	ExtAttrAllowShared  = "AllowShared"
)

// ArgumentNameKeywords may appear as argument names despite being keywords.
var ArgumentNameKeywords = []string{
	"async",
	"attribute",
	"callback",
	"const",
	"constructor",
	"deleter",
	"dictionary",
	"enum",
	"getter",
	"includes",
	"inherit",
	"interface",
	"iterable",
	"maplike",
	"namespace",
	"partial",
	"required",
	"setlike",
	"setter",
	"static",
	"stringifier",
	"typedef",
	"unrestricted",
}

// nonKeywordTerminals are identifier-shaped tokens that should be classified
// as TokInline (keywords/operators) rather than TokIdentifier.
var nonKeywordTerminals = buildNonRegexTerminals()

func buildNonRegexTerminals() map[string]struct{} {
	base := []string{
		"-Infinity",
		"FrozenArray",
		"Infinity",
		"NaN",
		"ObservableArray",
		"Promise",
		"async_iterable",
		"async_sequence",
		"bigint",
		"boolean",
		"byte",
		"double",
		"false",
		"float",
		"long",
		"mixin",
		"null",
		"octet",
		"optional",
		"or",
		"readonly",
		"record",
		"sequence",
		"short",
		"true",
		"undefined",
		"unsigned",
		"void",
	}
	m := make(map[string]struct{}, len(base)+len(ArgumentNameKeywords)+len(StringTypes)+len(TypeNameKeywords))
	for _, s := range base {
		m[s] = struct{}{}
	}
	for _, s := range ArgumentNameKeywords {
		m[s] = struct{}{}
	}
	for _, s := range StringTypes {
		m[s] = struct{}{}
	}
	for _, s := range TypeNameKeywords {
		m[s] = struct{}{}
	}
	return m
}

// reservedIdentifiers are identifier-shaped tokens that are illegal anywhere.
var reservedIdentifiers = map[string]struct{}{
	"_constructor": {},
	"toString":     {},
	"_toString":    {},
}


// TokenizeError reports a lexical error.
type TokenizeError struct {
	Line    int
	Message string
}

func (e *TokenizeError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

// Tokenize lexes a WebIDL source string into a slice of tokens. A trailing
// TokEOF token is always emitted.
func Tokenize(src string) ([]Token, error) {
	// Real-world IDL averages roughly one token per ~5 source bytes; a /4
	// preallocation under-shoots intentionally to avoid wasting space on
	// whitespace-heavy files.
	t := &tokenState{src: src, line: 1, tokens: make([]Token, 0, len(src)/5+8)}
	for t.pos < len(t.src) {
		if err := t.step(); err != nil {
			return nil, err
		}
	}
	t.tokens = append(t.tokens, Token{Kind: TokEOF, Value: "", Line: t.line, Index: t.nextIndex, Offset: t.pos})
	return t.tokens, nil
}

type tokenState struct {
	src       string
	pos       int
	line      int
	tokens    []Token
	nextIndex int
}

// emit appends a token to the stream. Callers must invoke emit before
// advancing t.pos past the token so that Offset captures the token's start
// byte in the source.
func (t *tokenState) emit(kind TokenKind, value string) {
	t.tokens = append(t.tokens, Token{Kind: kind, Value: value, Line: t.line, Index: t.nextIndex, Offset: t.pos})
	t.nextIndex++
}

func (t *tokenState) step() error {
	c := t.src[t.pos]

	// Whitespace.
	if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
		for t.pos < len(t.src) {
			c := t.src[t.pos]
			if c == '\n' {
				t.line++
			}
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				t.pos++
				continue
			}
			break
		}
		return nil
	}

	// Comments: //... or /* ... */
	if c == '/' && t.pos+1 < len(t.src) {
		next := t.src[t.pos+1]
		if next == '/' {
			end := strings.IndexByte(t.src[t.pos:], '\n')
			if end < 0 {
				t.pos = len(t.src)
			} else {
				t.pos += end // stop on the newline; the whitespace branch eats it next iteration
			}
			return nil
		}
		if next == '*' {
			closeIdx := strings.Index(t.src[t.pos+2:], "*/")
			if closeIdx < 0 {
				return &TokenizeError{Line: t.line, Message: "unterminated block comment"}
			}
			// Count newlines inside the comment.
			block := t.src[t.pos : t.pos+2+closeIdx+2]
			t.line += strings.Count(block, "\n")
			t.pos += 2 + closeIdx + 2
			return nil
		}
	}

	// Numeric / identifier dispatch: -, ., 0-9, A-Z, _, a-z
	if c == '-' || c == '.' || (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' {
		if n := t.matchDecimal(); n > 0 {
			t.emit(TokDecimal, t.src[t.pos:t.pos+n])
			t.pos += n
			return nil
		}
		if n := t.matchInteger(); n > 0 {
			t.emit(TokInteger, t.src[t.pos:t.pos+n])
			t.pos += n
			return nil
		}
		if n := t.matchIdentifier(); n > 0 {
			val := t.src[t.pos : t.pos+n]
			if _, ok := reservedIdentifiers[val]; ok {
				return &TokenizeError{Line: t.line, Message: fmt.Sprintf("%s is a reserved identifier and must not be used.", unescape(val))}
			}
			kind := TokIdentifier
			if _, ok := nonKeywordTerminals[val]; ok {
				kind = TokInline
			}
			t.emit(kind, val)
			t.pos += n
			return nil
		}
		// Fall through to punctuation/other.
	}

	// String. webidl2.js's regex permits any non-`"` byte inside, including
	// newlines; preserve that, but update line accounting so errors after a
	// multi-line literal still report the right line.
	if c == '"' {
		end := strings.IndexByte(t.src[t.pos+1:], '"')
		if end < 0 {
			return &TokenizeError{Line: t.line, Message: "unterminated string literal"}
		}
		val := t.src[t.pos : t.pos+1+end+1]
		// Emit at the token's start line, then advance t.line for tokens that follow.
		t.emit(TokString, val)
		t.line += strings.Count(val, "\n")
		t.pos += 1 + end + 1
		return nil
	}

	// Punctuation. Single-byte forms dispatch on the byte; `...` is the only
	// multi-byte case.
	switch c {
	case '(', ')', ',', ':', ';', '<', '=', '>', '?', '*', '[', ']', '{', '}':
		t.emit(TokInline, string(c))
		t.pos++
		return nil
	case '.':
		if t.pos+2 < len(t.src) && t.src[t.pos+1] == '.' && t.src[t.pos+2] == '.' {
			t.emit(TokInline, "...")
			t.pos += 3
			return nil
		}
	}

	// "Other" — single character.
	t.emit(TokOther, string(c))
	t.pos++
	return nil
}

// matchDecimal returns the length of a decimal literal starting at t.pos, or 0.
// Decimal requires either a '.' or an exponent letter in the mantissa.
//
//	-?(([0-9]+\.[0-9]*|[0-9]*\.[0-9]+)([Ee][-+]?[0-9]+)?|[0-9]+[Ee][-+]?[0-9]+)
func (t *tokenState) matchDecimal() int {
	s := t.src[t.pos:]
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	// case A: digits '.' digits? exponent?
	// case B: digits? '.' digits  exponent?
	// case C: digits exponent
	intStart := i
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	hadInt := i > intStart
	if i < len(s) && s[i] == '.' {
		i++
		fracStart := i
		for i < len(s) && isDigit(s[i]) {
			i++
		}
		hadFrac := i > fracStart
		if !hadInt && !hadFrac {
			return 0
		}
		// optional exponent
		if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
			i++
			if i < len(s) && (s[i] == '+' || s[i] == '-') {
				i++
			}
			expStart := i
			for i < len(s) && isDigit(s[i]) {
				i++
			}
			if i == expStart {
				return 0
			}
		}
		return i
	}
	// no dot — must have exponent and integer part
	if hadInt && i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		expStart := i
		for i < len(s) && isDigit(s[i]) {
			i++
		}
		if i == expStart {
			return 0
		}
		return i
	}
	return 0
}

// matchInteger returns the length of an integer literal starting at t.pos, or 0.
//
//	-?(0([Xx][0-9A-Fa-f]+|[0-7]*)|[1-9][0-9]*)
func (t *tokenState) matchInteger() int {
	s := t.src[t.pos:]
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	if i >= len(s) {
		return 0
	}
	if s[i] == '0' {
		i++
		if i < len(s) && (s[i] == 'x' || s[i] == 'X') {
			i++
			hexStart := i
			for i < len(s) && isHexDigit(s[i]) {
				i++
			}
			if i == hexStart {
				return 0
			}
			return i
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '7' {
			i++
		}
		return i
	}
	if s[i] >= '1' && s[i] <= '9' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
		}
		return i
	}
	return 0
}

// matchIdentifier returns the length of an identifier starting at t.pos, or 0.
//
//	[_-]?[A-Za-z][0-9A-Z_a-z-]*
func (t *tokenState) matchIdentifier() int {
	s := t.src[t.pos:]
	i := 0
	if i < len(s) && (s[i] == '_' || s[i] == '-') {
		i++
	}
	if i >= len(s) || !isLetter(s[i]) {
		return 0
	}
	i++
	for i < len(s) && isIdentTail(s[i]) {
		i++
	}
	return i
}

func isDigit(c byte) bool    { return c >= '0' && c <= '9' }
func isHexDigit(c byte) bool { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func isLetter(c byte) bool   { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') }
func isIdentTail(c byte) bool {
	return isDigit(c) || isLetter(c) || c == '_' || c == '-'
}

// unescape strips a leading underscore used to escape a reserved word.
func unescape(id string) string {
	if strings.HasPrefix(id, "_") {
		return id[1:]
	}
	return id
}
