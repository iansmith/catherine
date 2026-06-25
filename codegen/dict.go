package codegen

import (
	"fmt"
	"strings"
	"unicode"
)

// DictField describes a single field in a WebIDL dictionary. IDLName is the
// raw WebIDL name (e.g. "my-field"); GoType is the pre-resolved Go type string
// (e.g. "string", "int32", "[]Event"); Optional controls whether the field is
// emitted as *GoType with an omitempty JSON tag.
type DictField struct {
	IDLName  string
	GoType   string
	Optional bool
}

// dictField is the sanitized internal representation used by DictDecl.
type dictField struct {
	goName   string // PascalCase Go identifier
	idlName  string // original WebIDL name, used as JSON tag key
	goType   string // pre-resolved Go type string from the caller
	optional bool
}

// DictDecl is a Decl that emits a Go struct type from a WebIDL dictionary.
type DictDecl struct {
	typeName     string
	parentGoName string // empty string means no inheritance / no embedding
	fields       []dictField
}

// NewDictDecl creates a DictDecl from a WebIDL dictionary name, an optional
// already-sanitized parent Go type name (empty string for no inheritance), and
// a slice of fields. Field IDL names are sanitized and collision-checked here.
// diag may be nil; a fresh Diagnostics collector is used in that case.
func NewDictDecl(idlName string, parentGoName string, fields []DictField, diag *Diagnostics) *DictDecl {
	if diag == nil {
		diag = NewDiagnostics()
	}

	if !hasAlnum(idlName) {
		diag.Add("error", fmt.Sprintf("dict name %q has no letter or digit content; cannot produce a valid Go type name", idlName))
	}

	typeName := IdentSanitize(idlName)

	seen := make(map[string]bool)
	var internal []dictField
	for _, f := range fields {
		if !hasAlnum(f.IDLName) {
			diag.Add("error", fmt.Sprintf("dict %q: field IDL name %q has no letter or digit content", idlName, f.IDLName))
			continue
		}
		goName := IdentSanitize(f.IDLName)
		if runes := []rune(goName); !unicode.IsLetter(runes[0]) {
			diag.Add("error", fmt.Sprintf("dict %q: field IDL name %q sanitizes to invalid Go identifier %q", idlName, f.IDLName, goName))
			continue
		}
		if seen[goName] {
			diag.Add("error", fmt.Sprintf("dict %q: field %q dropped — collides with a prior field (both sanitize to %s)", idlName, f.IDLName, goName))
			continue
		}
		seen[goName] = true
		internal = append(internal, dictField{
			goName:   goName,
			idlName:  f.IDLName,
			goType:   f.GoType,
			optional: f.Optional,
		})
	}

	return &DictDecl{
		typeName:     typeName,
		parentGoName: parentGoName,
		fields:       internal,
	}
}

func (d *DictDecl) declName() string { return d.typeName }

// declSource implements Decl. It emits:
//
//	type T struct {
//	    Parent           // unnamed embedding, only when parentGoName != ""
//	    Field  GoType  `json:"fieldName"`
//	    OptFld *GoType `json:"optName,omitempty"`
//	}
func (d *DictDecl) declSource() string {
	var sb strings.Builder

	sb.WriteString("type ")
	sb.WriteString(d.typeName)
	sb.WriteString(" struct {\n")

	if d.parentGoName != "" {
		sb.WriteString("\t")
		sb.WriteString(d.parentGoName)
		sb.WriteString("\n")
	}

	for _, f := range d.fields {
		typeStr := f.goType
		tagSuffix := ""
		if f.optional {
			typeStr = "*" + f.goType
			tagSuffix = ",omitempty"
		}
		sb.WriteString("\t")
		sb.WriteString(f.goName)
		sb.WriteString(" ")
		sb.WriteString(typeStr)
		sb.WriteString(" `json:\"")
		sb.WriteString(f.idlName)
		sb.WriteString(tagSuffix)
		sb.WriteString("\"`\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}
