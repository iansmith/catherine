package codegen

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// enumEntry holds one const's Go identifier and the original IDL string value.
type enumEntry struct {
	constName string
	idlValue  string
}

// EnumDecl is a Decl that emits a Go named-string type, a const block, and a
// Parse helper from a WebIDL enum definition.
type EnumDecl struct {
	typeName string
	entries  []enumEntry
}

// NewEnumDecl creates an EnumDecl from a WebIDL enum's name and string values.
// Sanitization collisions are reported to diag (first value wins).
func NewEnumDecl(idlName string, idlValues []string, diag *Diagnostics) *EnumDecl {
	typeName := enumIdent(idlName)

	seen := make(map[string]bool)
	var entries []enumEntry
	for _, v := range idlValues {
		suffix := enumValueSanitize(v)
		constName := typeName + suffix
		if seen[constName] {
			diag.Add("error", fmt.Sprintf("enum %s: const name collision for %q (maps to %s; first value wins)", idlName, v, constName))
			continue
		}
		seen[constName] = true
		entries = append(entries, enumEntry{constName: constName, idlValue: v})
	}

	return &EnumDecl{typeName: typeName, entries: entries}
}

// declSource implements Decl. It emits:
//
//	type T string
//
//	const (
//	    TFoo T = "foo"
//	    TBar T = "bar"
//	)
//
//	func ParseT(s string) (T, bool) { ... }
func (e *EnumDecl) declSource() string {
	var sb strings.Builder

	sb.WriteString("type ")
	sb.WriteString(e.typeName)
	sb.WriteString(" string\n")

	if len(e.entries) > 0 {
		sb.WriteString("\nconst (\n")
		for _, entry := range e.entries {
			sb.WriteString("\t")
			sb.WriteString(entry.constName)
			sb.WriteString(" ")
			sb.WriteString(e.typeName)
			sb.WriteString(" = ")
			sb.WriteString(strconv.Quote(entry.idlValue))
			sb.WriteString("\n")
		}
		sb.WriteString(")\n")
	}

	sb.WriteString("\nfunc Parse")
	sb.WriteString(e.typeName)
	sb.WriteString("(s string) (")
	sb.WriteString(e.typeName)
	sb.WriteString(", bool) {\n")
	if len(e.entries) > 0 {
		sb.WriteString("\tswitch s {\n\tcase ")
		for i, entry := range e.entries {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(strconv.Quote(entry.idlValue))
		}
		sb.WriteString(":\n\t\treturn ")
		sb.WriteString(e.typeName)
		sb.WriteString("(s), true\n\t}\n")
	}
	sb.WriteString("\treturn \"\", false\n}\n")

	return sb.String()
}

// enumIdent normalises a WebIDL string (an enum name or value, which may
// contain characters outside the set IdentSanitize handles) into a valid
// exported Go identifier. Every rune that is not [A-Za-z0-9-_] becomes an
// underscore, which IdentSanitize then treats as a segment separator.
func enumIdent(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return IdentSanitize(sb.String())
}

// enumValueSanitize converts a WebIDL enum string value to a Go identifier
// suffix. An empty value maps to "Empty"; all others go through enumIdent.
func enumValueSanitize(v string) string {
	if v == "" {
		return "Empty"
	}
	return enumIdent(v)
}
