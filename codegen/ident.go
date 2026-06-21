package codegen

import (
	"strings"
	"unicode"
)

// goKeywords is the set of all 25 Go reserved words.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// goPredeclared is the set of Go predeclared identifiers that must not be shadowed.
var goPredeclared = map[string]bool{
	"true": true, "false": true, "nil": true, "error": true, "string": true,
	"int": true, "uint": true, "byte": true, "rune": true, "bool": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"int8": true, "int16": true, "int32": true, "int64": true,
	"uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"append": true, "make": true, "new": true, "len": true, "cap": true,
	"copy": true, "close": true, "delete": true, "panic": true, "recover": true,
	"print": true, "println": true, "iota": true,
}

// IdentSanitize converts an IDL name into a valid, exported Go identifier.
// It handles Go reserved words, predeclared identifiers, leading digits,
// hyphens, underscores, and lowercase-leading names.
func IdentSanitize(name string) string {
	// Split on hyphens and underscores, capitalizing the first letter of each segment.
	segments := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})

	// If there are no non-empty segments, return fallback.
	if len(segments) == 0 {
		return "X"
	}

	var sb strings.Builder
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		runes := []rune(seg)
		runes[0] = unicode.ToUpper(runes[0])
		sb.WriteString(string(runes))
	}

	result := sb.String()

	// If result is empty after joining (shouldn't happen, but guard), return fallback.
	if result == "" {
		return "X"
	}

	// If the first rune is a digit, prepend "X".
	firstRune := []rune(result)[0]
	if unicode.IsDigit(firstRune) {
		result = "X" + result
	}

	// Ensure the first rune is uppercase (handles the case where the first segment
	// started with a non-letter, non-digit character after ToUpper was a no-op).
	runes := []rune(result)
	if unicode.IsLower(runes[0]) {
		runes[0] = unicode.ToUpper(runes[0])
		result = string(runes)
	}

	// Append "Val" if the result is a Go keyword or predeclared identifier.
	if goKeywords[result] || goPredeclared[result] {
		result += "Val"
	}

	return result
}
