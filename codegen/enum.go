package codegen

// EnumDecl is a Decl that emits a Go named-string type, a const block, and a
// Parse helper from a WebIDL enum definition.
type EnumDecl struct{}

// NewEnumDecl creates an EnumDecl from a WebIDL enum's name and string values.
// Sanitization issues (identifier collisions, empty values) are reported to diag.
func NewEnumDecl(idlName string, idlValues []string, diag *Diagnostics) *EnumDecl {
	panic("NewEnumDecl: not implemented")
}

func (e *EnumDecl) declSource() string {
	panic("EnumDecl.declSource: not implemented")
}
