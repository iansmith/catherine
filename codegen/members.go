package codegen

import (
	"fmt"
	"unicode"

	"github.com/iansmith/webidl/typemap"
	"github.com/iansmith/webidl/webidl"
)

// This file is the single source of truth for how an interface's members map to
// Go names and which members survive. BOTH backends consume it: the layer-1
// interface generator (iface.go) emits the methods/consts, and the goja binding
// generator (binding.go) dispatches into them. They must agree exactly — a name
// or drop decision that differs between the two means the binding calls a method
// the interface never generated. Keeping the derivation here makes that drift a
// single-edit concern instead of a latent, compiler-invisible break.

// attrGetterName is the Go method name for an attribute's reader: `nodeType` →
// `NodeTypeAttr`.
func attrGetterName(idlName string) string { return IdentSanitize(idlName) + "Attr" }

// attrSetterName is the Go method name for a writable attribute's writer:
// `className` → `SetClassNameAttr`.
func attrSetterName(idlName string) string { return "Set" + IdentSanitize(idlName) + "Attr" }

// opGoName is the Go method name for an operation: `appendChild` → `AppendChild`.
func opGoName(idlName string) string { return IdentSanitize(idlName) }

// constGoName is the package-level Go name for an interface constant:
// (`Node`, `ELEMENT_NODE`) → `NodeELEMENTNODE`.
func constGoName(typeName, idlName string) string { return typeName + IdentSanitize(idlName) }

// isVoidReturn reports whether a return-position type carries no value — a nil
// node or the `undefined`/`void` sentinels. (In argument position `undefined`
// still maps to `any`; this rule applies only to returns.)
func isVoidReturn(t *webidl.IDLType) bool {
	return t == nil || t.Base == "undefined" || t.Base == "void"
}

// validGoIdentBase reports whether s is usable as a Go identifier base
// (non-empty and letter-led).
func validGoIdentBase(s string) bool {
	r := []rune(s)
	return len(r) > 0 && unicode.IsLetter(r[0])
}

// resolvedConst is one interface constant that survives resolution, carrying the
// JS-visible name, the package-level Go const name, the mapped Go type, and the
// literal value.
type resolvedConst struct {
	jsName string
	goName string
	goType string
	value  *webidl.ConstValue
}

// resolveConstants returns the constants an interface exposes, in declaration
// order, after the Go-name first-wins dedup and the type-mappability gate. Both
// buildConstBlock (which declares the consts) and the binding generator (which
// references them) call this so they cannot disagree on which constants exist or
// what they are named.
func resolveConstants(typeName string, members []webidl.Member, tm typemap.Mapper, diag *Diagnostics, idlName string) []resolvedConst {
	seen := make(map[string]bool)
	var out []resolvedConst
	for _, mem := range members {
		c, ok := mem.(*webidl.Constant)
		if !ok {
			continue
		}
		goName := constGoName(typeName, c.Name)
		if seen[goName] {
			diag.Add("error", fmt.Sprintf("interface %q: constant %q dropped — collision (first wins)", idlName, c.Name))
			continue
		}
		seen[goName] = true
		gt, err := tm.MapType(c.IDLType)
		if err != nil {
			diag.Add("error", fmt.Sprintf("interface %q: cannot map type for const %q: %v", idlName, c.Name, err))
			continue
		}
		out = append(out, resolvedConst{jsName: c.Name, goName: goName, goType: gt.String(), value: c.Value})
	}
	return out
}
