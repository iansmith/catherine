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

// Go method names for the indexed special operations. Shared so the layer-1
// interface (which declares them) and the binding (which dispatches into them)
// cannot drift — a rename here is compiler-invisible across the two backends
// otherwise, since goja is not a dependency. idxKeyGoType is the Go type of the
// integer index parameter.
const (
	idxGetterGoName  = "Index"
	idxSetterGoName  = "SetIndex"
	idxDeleterGoName = "Delete"
	idxKeyGoType     = "uint32"
)

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

// iterMethod is one JS-visible iteration method of an iterable/maplike/setlike
// declaration, in the form both backends need: iface.go reads goName + params +
// returnType to declare the layer-1 interface method; the binding reads jsName +
// goName + params + returnType to emit the accessor case. It is the single
// source of truth for the per-kind method set, the readonly gating, and the
// key/value type arity.
type iterMethod struct {
	jsName     string       // JS key (e.g. "values", "forEach", "set"); "" for async (binding skips)
	goName     string       // layer-1 Go method (e.g. "Values", "ForEach", "Set")
	params     []ifaceParam // layer-1 signature params
	returnType string       // layer-1 return type ("" for void)
}

// resolveIterMethods returns the ordered iteration methods an iterable/maplike/
// setlike exposes, with the key/value types already resolved and the readonly
// gating applied. Both addIterMethods (layer-1) and addIterable (binding)
// consume this so the two backends agree on the method set without each
// re-deriving the per-kind lists and arity.
func resolveIterMethods(it *webidl.IterableLike, tm typemap.Mapper, diag *Diagnostics, idlName string) []iterMethod {
	typeStrs := make([]string, 0, len(it.Types))
	for _, t := range it.Types {
		gt, err := tm.MapType(t)
		if err != nil {
			diag.Add("error", fmt.Sprintf("interface %q: cannot map iterable type: %v", idlName, err))
			typeStrs = append(typeStrs, "any")
			continue
		}
		typeStrs = append(typeStrs, gt.String())
	}

	seq := func(elem string) string { return "iter.Seq[" + elem + "]" }
	seq2 := func(k, v string) string { return "iter.Seq2[" + k + ", " + v + "]" }

	switch it.Kind {
	case webidl.IterIterable:
		valType, keyType := "any", "uint32"
		if len(typeStrs) == 1 {
			valType = typeStrs[0]
		} else if len(typeStrs) >= 2 {
			keyType, valType = typeStrs[0], typeStrs[1]
		}
		return []iterMethod{
			{jsName: "values", goName: "Values", returnType: seq(valType)},
			{jsName: "keys", goName: "Keys", returnType: seq(keyType)},
			{jsName: "entries", goName: "Entries", returnType: seq2(keyType, valType)},
			{jsName: "forEach", goName: "ForEach", params: []ifaceParam{{goName: "Fn", goType: "func(" + valType + ", " + keyType + ")"}}},
		}

	case webidl.IterAsyncIterable:
		valType := "any"
		if len(typeStrs) >= 1 {
			valType = typeStrs[len(typeStrs)-1]
		}
		out := []iterMethod{
			{goName: "AsyncValues", params: []ifaceParam{{goName: "Ctx", goType: "context.Context"}}, returnType: seq2(valType, "error")},
		}
		if len(typeStrs) >= 2 {
			keyType := typeStrs[0]
			out = append(out,
				iterMethod{goName: "AsyncKeys", params: []ifaceParam{{goName: "Ctx", goType: "context.Context"}}, returnType: seq2(keyType, "error")},
				iterMethod{goName: "AsyncEntries", params: []ifaceParam{{goName: "Ctx", goType: "context.Context"}}, returnType: seq2("Entry["+keyType+", "+valType+"]", "error")},
			)
		}
		return out

	case webidl.IterMaplike:
		if len(typeStrs) < 2 {
			diag.Add("error", fmt.Sprintf("interface %q: maplike requires 2 type arguments, got %d", idlName, len(typeStrs)))
			return nil
		}
		keyType, valType := typeStrs[0], typeStrs[1]
		out := []iterMethod{
			{jsName: "get", goName: "Get", params: []ifaceParam{{goName: "K", goType: keyType}}, returnType: valType},
			{jsName: "has", goName: "Has", params: []ifaceParam{{goName: "K", goType: keyType}}, returnType: "bool"},
			{jsName: "keys", goName: "Keys", returnType: seq(keyType)},
			{jsName: "values", goName: "Values", returnType: seq(valType)},
			{jsName: "entries", goName: "Entries", returnType: seq2(keyType, valType)},
			{jsName: "size", goName: "Size", returnType: "int"},
		}
		if !it.Readonly {
			out = append(out,
				iterMethod{jsName: "set", goName: "Set", params: []ifaceParam{{goName: "K", goType: keyType}, {goName: "V", goType: valType}}},
				iterMethod{jsName: "delete", goName: "Delete", params: []ifaceParam{{goName: "K", goType: keyType}}},
				iterMethod{jsName: "clear", goName: "Clear"},
			)
		}
		return out

	case webidl.IterSetlike:
		valType := "any"
		if len(typeStrs) >= 1 {
			valType = typeStrs[0]
		}
		out := []iterMethod{
			{jsName: "has", goName: "Has", params: []ifaceParam{{goName: "V", goType: valType}}, returnType: "bool"},
			{jsName: "keys", goName: "Keys", returnType: seq(valType)},
			{jsName: "values", goName: "Values", returnType: seq(valType)},
			{jsName: "entries", goName: "Entries", returnType: seq2(valType, valType)},
			{jsName: "size", goName: "Size", returnType: "int"},
		}
		if !it.Readonly {
			out = append(out,
				iterMethod{jsName: "add", goName: "Add", params: []ifaceParam{{goName: "V", goType: valType}}},
				iterMethod{jsName: "delete", goName: "Delete", params: []ifaceParam{{goName: "V", goType: valType}}},
				iterMethod{jsName: "clear", goName: "Clear"},
			)
		}
		return out
	}
	return nil
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
