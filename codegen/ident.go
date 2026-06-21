package codegen

import (
	"strings"
	"unicode"
)

// IdentSanitize converts a WebIDL identifier name into a valid, exported Go
// identifier. name is expected to contain only ASCII letters, digits, hyphens,
// and underscores — the character set the WebIDL tokenizer produces; other
// runes pass through unfiltered and may produce invalid identifiers.
// PascalCase output can never equal a lowercase Go keyword or predeclared
// identifier, so no reserved-word check is needed.
func IdentSanitize(name string) string {
	segments := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})

	if len(segments) == 0 {
		return "X"
	}

	// Capitalize the first rune of each segment to produce PascalCase. The first
	// segment's first rune therefore always becomes uppercase, so the result can
	// never start with a lowercase letter.
	var sb strings.Builder
	for _, seg := range segments {
		runes := []rune(seg)
		runes[0] = unicode.ToUpper(runes[0])
		sb.WriteString(string(runes))
	}
	result := sb.String()

	// Go identifiers cannot start with a digit; prefix one that does.
	if firstRune := []rune(result)[0]; unicode.IsDigit(firstRune) {
		result = "X" + result
	}

	return result
}
