package codegen

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// IdentSanitize converts an IDL name into a valid, exported Go identifier.
// It splits on hyphens and underscores (PascalCase), prepends "X" for
// leading digits, and ensures the first rune is uppercase.
// PascalCase output can never equal a lowercase Go keyword or predeclared
// identifier, so no additional reserved-word check is needed.
func IdentSanitize(name string) string {
	segments := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})

	if len(segments) == 0 {
		return "X"
	}

	var sb strings.Builder
	for _, seg := range segments {
		runes := []rune(seg)
		runes[0] = unicode.ToUpper(runes[0])
		sb.WriteString(string(runes))
	}

	result := sb.String()

	firstRune, _ := utf8.DecodeRuneInString(result)
	if unicode.IsDigit(firstRune) {
		result = "X" + result
	}

	runes := []rune(result)
	if unicode.IsLower(runes[0]) {
		runes[0] = unicode.ToUpper(runes[0])
		result = string(runes)
	}

	return result
}
