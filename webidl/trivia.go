package webidl

import (
	"slices"
	"strings"
)

// PrintIDL reconstructs the original IDL source string from a definition list
// and the original source. When defs contains unmodified definitions the result
// is byte-for-byte identical to src. When a mutable field (Name, Inheritance,
// etc.) on a definition differs from the token value found at that position in
// src, the new field value is emitted in place of the original token while all
// surrounding trivia (whitespace, comments) is preserved verbatim.
//
// The reconstruction re-tokenizes src to recover token byte offsets, then fills
// each inter-token gap (whitespace, comments) directly from src. If src cannot
// be tokenized, PrintIDL returns src unchanged.
func PrintIDL(src string, defs []Definition) string {
	tokens, err := Tokenize(src)
	if err != nil {
		return src
	}
	var subs map[int]string
	if len(defs) > 0 {
		subs = buildSubstitutions(tokens, tokensByOffset(tokens), defs)
	}
	var b strings.Builder
	b.Grow(len(src))
	pos := 0
	for _, tok := range tokens {
		if tok.Kind == TokEOF {
			break
		}
		b.WriteString(src[pos:tok.Offset])
		if sub, ok := subs[tok.Offset]; ok {
			b.WriteString(sub)
		} else {
			b.WriteString(tok.Value)
		}
		pos = tok.Offset + len(tok.Value)
	}
	b.WriteString(src[pos:])
	return b.String()
}

// tokensByOffset returns a map from each token's byte offset to its index in
// the tokens slice. Enables O(1) lookup from a definition's Span.Offset to the
// corresponding token.
func tokensByOffset(tokens []Token) map[int]int {
	m := make(map[int]int, len(tokens))
	for i, tok := range tokens {
		m[tok.Offset] = i
	}
	return m
}

// buildSubstitutions returns a map from source byte offsets to replacement
// strings. For each definition in defs, it locates the token(s) corresponding
// to mutable fields and records a substitution when the current field value
// differs from the token value in src.
func buildSubstitutions(tokens []Token, byOffset map[int]int, defs []Definition) map[int]string {
	subs := make(map[int]string)
	for _, def := range defs {
		off := spanOffsetOf(def)
		if off < 0 {
			continue
		}
		i, ok := byOffset[off]
		if !ok {
			continue
		}
		switch d := def.(type) {
		case *Interface:
			addInterfaceSubs(tokens, i, d, subs)
		case *Dictionary:
			addDictionarySubs(tokens, i, d, subs)
		case *Enum:
			addEnumSubs(tokens, i, d, subs)
		case *Typedef:
			addTypedefSubs(tokens, i, d, subs)
		case *Includes:
			addIncludesSubs(tokens, i, d, subs)
		case *Namespace:
			addNamespaceSubs(tokens, i, d, subs)
		case *CallbackFunction:
			addCallbackFunctionSubs(tokens, i, d, subs)
		}
	}
	return subs
}

// spanOffsetOf returns the byte offset of the first token of def, or -1 if the
// concrete type is not recognised.
func spanOffsetOf(def Definition) int {
	switch d := def.(type) {
	case *Interface:
		return d.Span.Offset
	case *Dictionary:
		return d.Span.Offset
	case *Enum:
		return d.Span.Offset
	case *Typedef:
		return d.Span.Offset
	case *Includes:
		return d.Span.Offset
	case *Namespace:
		return d.Span.Offset
	case *CallbackFunction:
		return d.Span.Offset
	}
	return -1
}

// skipExtAttrs returns the index of the first token after a balanced [...]
// extended-attribute block. If tokens[i] is not '[', i is returned unchanged.
func skipExtAttrs(tokens []Token, i int) int {
	if i >= len(tokens) || tokens[i].Value != "[" {
		return i
	}
	depth := 0
	for ; i < len(tokens); i++ {
		switch tokens[i].Value {
		case "[":
			depth++
		case "]":
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return i
}

// skipWord advances past tokens[i] when its value matches any of words (an
// optional prefix keyword such as "partial" or "mixin"). Otherwise i is
// returned unchanged. Out-of-range i is returned as-is.
func skipWord(tokens []Token, i int, words ...string) int {
	if i < 0 || i >= len(tokens) {
		return i
	}
	if slices.Contains(words, tokens[i].Value) {
		return i + 1
	}
	return i
}

// requireWord advances past tokens[i] when its value equals word (a mandatory
// keyword such as "interface" or "enum"), returning the next index. If the
// keyword is absent or i is out of range it returns -1, which propagates
// through later cursor operations as a no-op.
func requireWord(tokens []Token, i int, word string) int {
	if i < 0 || i >= len(tokens) || tokens[i].Value != word {
		return -1
	}
	return i + 1
}

// recordInheritance records a substitution for an inheritance base name when
// the definition has one. nameIdx is the index of the definition's name token;
// the ":" separator and base name follow it. A negative nameIdx (name not
// located) or an empty inheritance string is left untouched.
func recordInheritance(tokens []Token, nameIdx int, inheritance string, subs map[int]string) {
	if inheritance == "" || nameIdx < 0 {
		return
	}
	colon := nameIdx + 1
	if colon >= len(tokens) || tokens[colon].Value != ":" {
		return
	}
	recordSub(tokens, colon+1, inheritance, subs)
}

// recordSub adds offset → newVal to subs when newVal differs from tokens[i].Value.
// A no-op replacement (same value) produces no entry.
func recordSub(tokens []Token, i int, newVal string, subs map[int]string) {
	if i < 0 || i >= len(tokens) || tokens[i].Kind == TokEOF {
		return
	}
	if unescape(tokens[i].Value) != newVal {
		subs[tokens[i].Offset] = newVal
	}
}

// addInterfaceSubs locates the Name (and optionally Inheritance) token(s) for
// an interface definition and records substitutions when their values differ.
//
// Keyword layout (all optional prefixes handled):
//
//	[ExtAttrs] (partial|callback)? interface mixin? Name (: Base)? { ... };
func addInterfaceSubs(tokens []Token, i int, d *Interface, subs map[int]string) {
	i = skipExtAttrs(tokens, i)
	i = skipWord(tokens, i, "partial", "callback")
	i = requireWord(tokens, i, "interface")
	i = skipWord(tokens, i, "mixin")
	recordSub(tokens, i, d.Name, subs)
	recordInheritance(tokens, i, d.Inheritance, subs)
}

// addDictionarySubs locates the Name (and optionally Inheritance) token(s) for
// a dictionary definition.
//
//	[ExtAttrs] partial? dictionary Name (: Base)? { ... };
func addDictionarySubs(tokens []Token, i int, d *Dictionary, subs map[int]string) {
	i = skipExtAttrs(tokens, i)
	i = skipWord(tokens, i, "partial")
	i = requireWord(tokens, i, "dictionary")
	recordSub(tokens, i, d.Name, subs)
	recordInheritance(tokens, i, d.Inheritance, subs)
}

// addEnumSubs locates the Name token for an enum definition.
//
//	[ExtAttrs] enum Name { ... };
func addEnumSubs(tokens []Token, i int, d *Enum, subs map[int]string) {
	i = skipExtAttrs(tokens, i)
	i = requireWord(tokens, i, "enum")
	recordSub(tokens, i, d.Name, subs)
}

// addNamespaceSubs locates the Name token for a namespace definition.
//
//	[ExtAttrs] partial? namespace Name { ... };
func addNamespaceSubs(tokens []Token, i int, d *Namespace, subs map[int]string) {
	i = skipExtAttrs(tokens, i)
	i = skipWord(tokens, i, "partial")
	i = requireWord(tokens, i, "namespace")
	recordSub(tokens, i, d.Name, subs)
}

// addCallbackFunctionSubs locates the Name token for a callback function.
//
//	[ExtAttrs] callback Name = ReturnType ( Args... );
//
// Note: callback interface is handled via addInterfaceSubs (type *Interface,
// Variant == IfaceCallback); this function only handles CallbackFunction.
func addCallbackFunctionSubs(tokens []Token, i int, d *CallbackFunction, subs map[int]string) {
	i = skipExtAttrs(tokens, i)
	i = requireWord(tokens, i, "callback")
	recordSub(tokens, i, d.Name, subs)
}

// addTypedefSubs locates the Name token for a typedef. The name is the
// TokIdentifier immediately before the closing ";", because the type expression
// may occupy a variable number of tokens (including user-defined type names).
//
//	[ExtAttrs] typedef TypeExpr Name;
func addTypedefSubs(tokens []Token, i int, d *Typedef, subs map[int]string) {
	i = skipExtAttrs(tokens, i)
	if i >= len(tokens) || tokens[i].Value != "typedef" {
		return
	}
	i++ // skip "typedef"
	// Scan forward to find the last TokIdentifier before ";". All built-in type
	// keywords (long, unsigned, sequence, DOMString, …) are TokInline; only
	// user-defined names are TokIdentifier. The typedef name is always the last
	// TokIdentifier in the definition.
	lastIdent := -1
	for j := i; j < len(tokens) && tokens[j].Kind != TokEOF; j++ {
		if tokens[j].Value == ";" {
			break
		}
		if tokens[j].Kind == TokIdentifier {
			lastIdent = j
		}
	}
	if lastIdent >= 0 {
		recordSub(tokens, lastIdent, d.Name, subs)
	}
}

// addIncludesSubs locates the Target and Includes (mixin) name tokens.
//
//	[ExtAttrs] Target includes Mixin;
func addIncludesSubs(tokens []Token, i int, d *Includes, subs map[int]string) {
	i = skipExtAttrs(tokens, i)
	recordSub(tokens, i, d.Target, subs) // Target identifier
	i = requireWord(tokens, i+1, "includes")
	recordSub(tokens, i, d.Includes, subs) // mixin identifier
}
