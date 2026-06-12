package webidl

import "strings"

// PrintIDL reconstructs the original IDL source string from a definition list
// and the original source. Round-trips are byte-for-byte identical: all
// whitespace, comments, and formatting are preserved exactly.
//
// The reconstruction re-tokenizes src to recover token byte offsets, then fills
// each inter-token gap (whitespace, comments) directly from src. The defs
// parameter is unused in this implementation but is retained in the signature
// for future modified-AST printing: when defs carries modified definitions,
// this function will emit replacement tokens in place of the originals while
// preserving surrounding trivia.
//
// If src cannot be tokenized (e.g. an unterminated block comment), PrintIDL
// returns src unchanged. This path is unreachable when src was successfully
// parsed: Parse(src) calls Tokenize internally, so any lexical error would
// have already surfaced there.
func PrintIDL(src string, defs []Definition) string {
	tokens, err := Tokenize(src)
	if err != nil {
		// Guards callers that pass synthetic or partially-constructed source;
		// see the doc comment for why this is unreachable after a real Parse.
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	pos := 0
	for _, tok := range tokens {
		if tok.Kind == TokEOF {
			break
		}
		// Copy the trivia (whitespace / comments) preceding this token, then
		// the token itself. Tokens are in source order, so tok.Offset >= pos
		// and the slice is empty when there is no intervening trivia. For an
		// unmodified round-trip tok.Value is a verbatim byte-slice of src, so
		// this loop reproduces src exactly.
		b.WriteString(src[pos:tok.Offset])
		b.WriteString(tok.Value)
		pos = tok.Offset + len(tok.Value)
	}
	// Copy any trailing trivia after the last real token.
	b.WriteString(src[pos:])
	return b.String()
}
