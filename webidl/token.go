package webidl

// TokenKind classifies a lexical token.
//
// The kinds mirror webidl2.js's tokeniser: literals (decimal, integer, string,
// identifier), keywords and punctuation (inline), and an eof sentinel. Unlike
// webidl2.js we do not preserve trivia (whitespace/comments) on each token;
// the abstract AST does not need them.
type TokenKind int

const (
	TokEOF TokenKind = iota
	TokInline
	TokIdentifier
	TokDecimal
	TokInteger
	TokString
	TokOther
)

func (k TokenKind) String() string {
	switch k {
	case TokEOF:
		return "eof"
	case TokInline:
		return "inline"
	case TokIdentifier:
		return "identifier"
	case TokDecimal:
		return "decimal"
	case TokInteger:
		return "integer"
	case TokString:
		return "string"
	case TokOther:
		return "other"
	}
	return "unknown"
}

// Token is a single lexical element.
type Token struct {
	Kind   TokenKind
	Value  string
	Line   int // 1-based line number where the token begins
	Index  int // token index in the stream (0-based)
	Offset int // byte offset of Value in the source string (set by Tokenize)
}
