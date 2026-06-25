package codegen

import "fmt"

// TypedefDecl is a Decl that emits a Go type declaration from a WebIDL typedef.
// If isAlias is true it emits `type Name = GoType`; otherwise `type Name GoType`.
type TypedefDecl struct {
	typeName string
	goType   string
	isAlias  bool
}

// NewTypedefDecl creates a TypedefDecl. idlName is the WebIDL typedef name (e.g. "DOMString");
// goType is the pre-resolved Go type string (e.g. "string"); isAlias controls alias vs distinct.
// diag may be nil; a fresh Diagnostics is used in that case.
func NewTypedefDecl(idlName, goType string, isAlias bool, diag *Diagnostics) *TypedefDecl {
	if diag == nil {
		diag = NewDiagnostics()
	}
	if !hasAlnum(idlName) {
		diag.Add("error", fmt.Sprintf("typedef name %q has no letter or digit content; cannot produce a valid Go type name", idlName))
	}
	if goType == "" {
		diag.Add("error", fmt.Sprintf("typedef %q: goType must not be empty", idlName))
	}
	return &TypedefDecl{
		typeName: IdentSanitize(idlName),
		goType:   goType,
		isAlias:  isAlias,
	}
}

func (t *TypedefDecl) declName() string { return t.typeName }

// declSource implements Decl. Emits either:
//
//	type Name = GoType   (isAlias=true)
//	type Name GoType     (isAlias=false)
func (t *TypedefDecl) declSource() string {
	if t.isAlias {
		return "type " + t.typeName + " = " + t.goType + "\n"
	}
	return "type " + t.typeName + " " + t.goType + "\n"
}
