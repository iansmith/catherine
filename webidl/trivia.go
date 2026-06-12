package webidl

// PrintIDL reconstructs the original IDL source string from a definition list
// and the original source. Round-trips are byte-for-byte identical: all
// whitespace, comments, and formatting are preserved exactly.
//
// The defs must have been produced by Parse(src) with Token.Offset values set
// by Tokenize. The reconstruction works by filling inter-token gaps — the
// trivia regions that Tokenize currently discards — from src using each
// token's byte offset.
//
// TODO: implement using Token.Offset once Tokenize populates it.
func PrintIDL(src string, defs []Definition) string {
	return ""
}
