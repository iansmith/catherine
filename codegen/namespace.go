package codegen

import (
	"fmt"
	"strings"
)

// nsMethod is one method on a namespace (from an operation or attribute).
type nsMethod struct {
	goName     string
	params     []ifaceParam
	returnType string // "" for void
}

// NamespaceDecl is a Decl that emits a Go singleton struct from a WebIDL namespace.
type NamespaceDecl struct {
	typeName string // exported Go name, e.g. "Console"
	implName string // unexported type name, e.g. "consoleType"
	methods  []nsMethod
}

// NewNamespaceDecl creates a NamespaceDecl. idlName is the WebIDL namespace name.
// methods are the processed operations and attributes. diag may be nil.
func NewNamespaceDecl(idlName string, methods []nsMethod, diag *Diagnostics) *NamespaceDecl {
	if diag == nil {
		diag = NewDiagnostics()
	}
	if !hasAlnum(idlName) {
		diag.Add("error", fmt.Sprintf("namespace name %q has no letter or digit content", idlName))
	}
	typeName := IdentSanitize(idlName)
	implName := strings.ToLower(typeName[:1]) + typeName[1:] + "Type"
	return &NamespaceDecl{
		typeName: typeName,
		implName: implName,
		methods:  methods,
	}
}

func (n *NamespaceDecl) declName() string { return n.typeName }

// declSource implements Decl. Emits the unexported struct type, the exported var,
// and stub methods.
func (n *NamespaceDecl) declSource() string {
	var sb strings.Builder

	sb.WriteString("type ")
	sb.WriteString(n.implName)
	sb.WriteString(" struct{}\n\n")

	sb.WriteString("var ")
	sb.WriteString(n.typeName)
	sb.WriteString(" = &")
	sb.WriteString(n.implName)
	sb.WriteString("{}\n")

	recv := strings.ToLower(n.implName[:1])

	for _, m := range n.methods {
		sb.WriteString("\nfunc (")
		sb.WriteString(recv)
		sb.WriteString(" *")
		sb.WriteString(n.implName)
		sb.WriteString(") ")
		sb.WriteString(m.goName)
		sb.WriteString("(")
		writeParams(&sb, m.params)
		sb.WriteString(")")
		if m.returnType != "" {
			sb.WriteString(" ")
			sb.WriteString(m.returnType)
			sb.WriteString(" { panic(\"not implemented\") }\n")
		} else {
			sb.WriteString(" {}\n")
		}
	}

	return sb.String()
}
