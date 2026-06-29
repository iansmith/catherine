package codegen

import (
	"fmt"
	"sort"
	"strings"
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

// iterRenderKind tells the binding how to render an iteration method's goja
// body. It is set where the method is declared (resolveIterMethods) so the
// binding switches on intent rather than re-sniffing the rendered Go type
// string — decoupling it from iface.go's exact type spelling.
type iterRenderKind int

const (
	renderScalar  iterRenderKind = iota // wrap the result in ToValue
	renderSeq                           // wrap the iter.Seq result via the shim's wrapSeq
	renderVoid                          // statement (mutator); closure returns Undefined
	renderForEach                       // callback-adapted forEach
)

// iterMethod is one JS-visible iteration method of an iterable/maplike/setlike
// declaration, in the form both backends need: iface.go reads goName + params +
// returnType to declare the layer-1 interface method; the binding reads jsName +
// goName + params + render to emit the accessor case. It is the single source of
// truth for the per-kind method set, the readonly gating, and the key/value type
// arity.
type iterMethod struct {
	jsName     string         // JS key (e.g. "values", "forEach", "set"); "" for async (binding skips)
	goName     string         // layer-1 Go method (e.g. "Values", "ForEach", "Set")
	params     []ifaceParam   // layer-1 signature params
	returnType string         // layer-1 return type ("" for void)
	render     iterRenderKind // how the binding wraps the dispatch
	cbArgs     []ifaceParam   // typed closure args for the adapter (renderForEach only; nil for other renders)
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

	switch it.Kind {
	case webidl.IterIterable:
		valType, keyType := "any", "uint32"
		if len(typeStrs) == 1 {
			valType = typeStrs[0]
		} else if len(typeStrs) >= 2 {
			keyType, valType = typeStrs[0], typeStrs[1]
		}
		return []iterMethod{
			{jsName: "values", goName: "Values", returnType: iterSeq(valType), render: renderSeq},
			{jsName: "keys", goName: "Keys", returnType: iterSeq(keyType), render: renderSeq},
			{jsName: "entries", goName: "Entries", returnType: iterSeq2(keyType, valType), render: renderSeq},
			{jsName: "forEach", goName: "ForEach", params: []ifaceParam{{goName: "Fn", goType: "func(" + valType + ", " + keyType + ")"}}, render: renderForEach, cbArgs: []ifaceParam{{goName: "_v", goType: valType}, {goName: "_k", goType: keyType}}},
		}

	case webidl.IterAsyncIterable:
		return asyncIterMethods(typeStrs)

	case webidl.IterMaplike:
		if len(typeStrs) < 2 {
			diag.Add("error", fmt.Sprintf("interface %q: maplike requires 2 type arguments, got %d", idlName, len(typeStrs)))
			return nil
		}
		keyType, valType := typeStrs[0], typeStrs[1]
		out := []iterMethod{
			{jsName: "get", goName: "Get", params: []ifaceParam{{goName: "K", goType: keyType}}, returnType: valType, render: renderScalar},
			{jsName: "has", goName: "Has", params: []ifaceParam{{goName: "K", goType: keyType}}, returnType: "bool", render: renderScalar},
			{jsName: "keys", goName: "Keys", returnType: iterSeq(keyType), render: renderSeq},
			{jsName: "values", goName: "Values", returnType: iterSeq(valType), render: renderSeq},
			{jsName: "entries", goName: "Entries", returnType: iterSeq2(keyType, valType), render: renderSeq},
			{jsName: "size", goName: "Size", returnType: "int", render: renderScalar},
		}
		if !it.Readonly {
			out = append(out,
				iterMethod{jsName: "set", goName: "Set", params: []ifaceParam{{goName: "K", goType: keyType}, {goName: "V", goType: valType}}, render: renderVoid},
				iterMethod{jsName: "delete", goName: "Delete", params: []ifaceParam{{goName: "K", goType: keyType}}, render: renderVoid},
				iterMethod{jsName: "clear", goName: "Clear", render: renderVoid},
			)
		}
		return out

	case webidl.IterSetlike:
		valType := "any"
		if len(typeStrs) >= 1 {
			valType = typeStrs[0]
		}
		out := []iterMethod{
			{jsName: "has", goName: "Has", params: []ifaceParam{{goName: "V", goType: valType}}, returnType: "bool", render: renderScalar},
			{jsName: "keys", goName: "Keys", returnType: iterSeq(valType), render: renderSeq},
			{jsName: "values", goName: "Values", returnType: iterSeq(valType), render: renderSeq},
			{jsName: "entries", goName: "Entries", returnType: iterSeq2(valType, valType), render: renderSeq},
			{jsName: "size", goName: "Size", returnType: "int", render: renderScalar},
		}
		if !it.Readonly {
			out = append(out,
				iterMethod{jsName: "add", goName: "Add", params: []ifaceParam{{goName: "V", goType: valType}}, render: renderVoid},
				iterMethod{jsName: "delete", goName: "Delete", params: []ifaceParam{{goName: "V", goType: valType}}, render: renderVoid},
				iterMethod{jsName: "clear", goName: "Clear", render: renderVoid},
			)
		}
		return out
	}
	return nil
}

func iterSeq(elem string) string  { return "iter.Seq[" + elem + "]" }
func iterSeq2(k, v string) string { return "iter.Seq2[" + k + ", " + v + "]" }

// asyncIterMethods returns the async-iterable methods. These are layer-1 only —
// the binding defers JS async iteration (CATH-66+) and skips this kind — so no
// render kind / jsName is set.
func asyncIterMethods(typeStrs []string) []iterMethod {
	valType := "any"
	if len(typeStrs) >= 1 {
		valType = typeStrs[len(typeStrs)-1]
	}
	ctx := []ifaceParam{{goName: "Ctx", goType: "context.Context"}}
	out := []iterMethod{
		{goName: "AsyncValues", params: ctx, returnType: iterSeq2(valType, "error")},
	}
	if len(typeStrs) >= 2 {
		keyType := typeStrs[0]
		out = append(out,
			iterMethod{goName: "AsyncKeys", params: ctx, returnType: iterSeq2(keyType, "error")},
			iterMethod{goName: "AsyncEntries", params: ctx, returnType: iterSeq2("Entry["+keyType+", "+valType+"]", "error")},
		)
	}
	return out
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

// ===========================================================================
// CATH-65: extended-attribute helpers shared by both backends
//
// reflectedAttr, classifyArg, and resolveOverloads all live here for the same
// reason as the helpers above: iface.go and binding.go must make IDENTICAL
// decisions (which attrs are reflected → trimmed; how overloads are named and
// dispatched) or the binding calls a layer-1 method that does not exist. Keeping
// them here makes that agreement a single-edit invariant.
// ===========================================================================

// reflectKind identifies the reflected-attribute algorithm, keyed off the
// attribute's mapped Go type. The four kinds are the reflectable set for this
// ticket (CATH-65 D2); any other type is reflectNone and keeps its layer-1
// method (no hole).
type reflectKind int

const (
	reflectNone reflectKind = iota
	reflectString
	reflectBool
	reflectInt32
	reflectUint32
)

// shimSuffix is the typed CATH-66 reflect-shim method suffix:
// b.ctx.reflectGet<suffix> / reflectSet<suffix>.
func (k reflectKind) shimSuffix() string {
	switch k {
	case reflectString:
		return "String"
	case reflectBool:
		return "Bool"
	case reflectInt32:
		return "Int32"
	case reflectUint32:
		return "Uint32"
	}
	return ""
}

// goType is the Go type the reflect setter coerces the JS value to.
func (k reflectKind) goType() string {
	switch k {
	case reflectString:
		return "string"
	case reflectBool:
		return "bool"
	case reflectInt32:
		return "int32"
	case reflectUint32:
		return "uint32"
	}
	return "any"
}

// reflectedAttr decides whether attribute a is reflected end-to-end (CATH-65
// D1/D2/D3). It returns the content-attribute (DOM) name, the algorithm kind,
// and ok=true ONLY when [Reflect] is present AND the attribute's type is one we
// can reflect fully. A [Reflect] on any other type returns ok=false so the
// caller keeps the layer-1 accessor as a fallback. The DOM name is the
// [Reflect=foo] value, else the IDL identifier ASCII-lowercased.
//
// This is the single gate consumed by BOTH iface.go (to trim the layer-1 method)
// and binding.go (to emit the reflected accessor), so they cannot disagree.
func reflectedAttr(a *webidl.Attribute, tm typemap.Mapper) (domName string, kind reflectKind, ok bool) {
	set := ParseExtAttrs(a.ExtAttrs, nil)
	if !set.ReflectPresent {
		return "", reflectNone, false
	}
	domName = set.ReflectAttr
	if domName == "" {
		domName = strings.ToLower(a.Name)
	}
	switch goTypeOf(a.IDLType, tm) {
	case "string":
		return domName, reflectString, true
	case "bool":
		return domName, reflectBool, true
	case "int32":
		return domName, reflectInt32, true
	case "uint32":
		return domName, reflectUint32, true
	}
	return "", reflectNone, false
}

// argClass is the coarse runtime kind used to discriminate same-arity overloads.
// It maps to the CATH-66 shim's Kind enum (see the binding.go shim contract).
// The validator (§3.2.11) guarantees overloads are distinguishable, and
// distinguishability collapses every object-like IDL type into a single bucket,
// so these coarse classes suffice to route the cases the generator emits.
type argClass int

const (
	classObject  argClass = iota // interfaces, dicts, sequences, records, callbacks, object, any
	classString                  // DOMString/USVString/ByteString
	classNumber                  // integer and float types
	classBoolean                 // boolean
)

// kindConst is the goja-shim Kind constant this class routes on (the case label
// the binding emits in an argKind switch). CATH-66 defines the Kind enum.
func (c argClass) kindConst() string {
	switch c {
	case classString:
		return "rt.KindString"
	case classNumber:
		return "rt.KindNumber"
	case classBoolean:
		return "rt.KindBoolean"
	}
	return "rt.KindObject"
}

var numericGoTypes = map[string]bool{
	"int8": true, "uint8": true, "int16": true, "uint16": true,
	"int32": true, "uint32": true, "int64": true, "uint64": true,
	"int": true, "uint": true, "float32": true, "float64": true,
}

// goTypeOf maps an IDL type to its Go type string, degrading unmappable types to
// "any" (the binding's degrade-and-continue convention).
func goTypeOf(t *webidl.IDLType, tm typemap.Mapper) string {
	gt, err := tm.MapType(t)
	if err != nil {
		return "any"
	}
	return gt.String()
}

// classifyArg maps an argument's IDL type to a coarse runtime class.
func classifyArg(t *webidl.IDLType, tm typemap.Mapper) argClass {
	return classGoType(goTypeOf(t, tm))
}

// classGoType maps a Go type string to a coarse runtime class. Anything that is
// not a string, boolean, or number (including unresolved interface types, which
// map to "any") is treated as object — coerced via Unwrap, returned via WrapAny.
func classGoType(g string) argClass {
	switch {
	case g == "string":
		return classString
	case g == "bool":
		return classBoolean
	case numericGoTypes[g]:
		return classNumber
	default:
		return classObject
	}
}

// isObjectType reports whether an attribute's type is object-like (not a string,
// boolean, or number). Used to gate [SameObject], which is only meaningful for
// object-typed attributes.
func isObjectType(t *webidl.IDLType, tm typemap.Mapper) bool {
	return classifyArg(t, tm) == classObject
}

// overloadTypeTag is the disambiguating token for a same-arity overload's method
// name, derived from the distinguishing argument. Primitive classes use a fixed
// readable token; object types use the IDL type name (which survives even when
// the type is unresolved, unlike the mapped Go type which degrades to "any").
func overloadTypeTag(t *webidl.IDLType, cls argClass, tm typemap.Mapper) string {
	switch cls {
	case classString:
		return "String"
	case classBoolean:
		return "Bool"
	case classNumber:
		return typeTag(goTypeOf(t, tm))
	default:
		return typeTag(idlTypeName(t))
	}
}

// idlTypeName returns a readable name for an IDL type for use in a Go identifier.
func idlTypeName(t *webidl.IDLType) string {
	switch {
	case t == nil:
		return "X"
	case t.Base != "":
		return t.Base
	case t.Union:
		return "Union"
	case t.Generic != "":
		return t.Generic
	}
	return "X"
}

// typeTag is the readable Go-ident token that disambiguates same-arity overload
// method names: Add(DOMString)→Add1String, Add(Node)→Add1Node. Derived from the
// distinguishing argument's Go type, stripped to letters/digits and title-cased.
func typeTag(goType string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, goType)
	if cleaned == "" {
		return "X"
	}
	r := []rune(cleaned)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

// exposedTo reports whether an interface with the given [Exposed] scopes is
// exposed to global. Absent [Exposed] (nil) is lenient — exposed everywhere;
// ["*"] is always exposed; otherwise membership decides (CATH-65 D4).
func exposedTo(scopes []string, global string) bool {
	if scopes == nil {
		return true
	}
	for _, s := range scopes {
		if s == "*" || s == global {
			return true
		}
	}
	return false
}

// manifestGlobals normalizes an interface's [Exposed] scopes for the registry
// manifest: absent (nil) becomes ["*"] (lenient = everywhere).
func manifestGlobals(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{"*"}
	}
	return scopes
}

// overloadSig is one resolved overload of a same-named operation, in the form
// both backends need: iface.go declares goName(params) returnType; binding.go
// builds the runtime dispatch from arity and (for same-arity overloads)
// distinguishPos/class.
type overloadSig struct {
	goName         string
	params         []ifaceParam
	returnType     string // "" for void
	arity          int    // len(Arguments) — the len(call.Arguments) this matches
	distinguishPos int    // arg position to switch on for same-arity, else -1
	class          argClass
}

// groupOverloads returns the regular, named, non-static operations grouped by
// name, but only for names with MORE THAN ONE such operation (true overloads).
// Single-signature ops are not returned — callers handle them on the normal
// path. Shared so both backends detect the same overload sets.
func groupOverloads(members []webidl.Member) map[string][]*webidl.Operation {
	byName := map[string][]*webidl.Operation{}
	for _, mem := range members {
		op, ok := mem.(*webidl.Operation)
		if !ok || op.Special != "" || op.Name == "" {
			continue
		}
		byName[op.Name] = append(byName[op.Name], op)
	}
	out := map[string][]*webidl.Operation{}
	for name, ops := range byName {
		if len(ops) > 1 {
			out[name] = ops
		}
	}
	return out
}

// overloadHasVariadicOrOptional reports whether any operation in an overload set
// declares an optional or variadic argument — cases the arity-bucketed dispatch
// only approximates.
func overloadHasVariadicOrOptional(ops []*webidl.Operation) bool {
	for _, op := range ops {
		for _, arg := range op.Arguments {
			if arg.Optional || arg.Variadic {
				return true
			}
		}
	}
	return false
}

// resolveOverloads turns the overload set for one operation name into the
// per-overload signatures both backends emit. Overloads are bucketed by arity
// (ascending for deterministic output): each arity with a single signature gets
// an arity-suffixed name (DrawImage3); an arity shared by several signatures
// that are distinguishable by a coarse runtime class at some position gets a
// type-tagged name (Add1String/Add1Node) and the dispatch position. An arity
// whose signatures are NOT distinguishable by coarse class (e.g. two interface
// types) degrades to first-wins with a diagnostic — the object-vs-object case is
// deferred (CATH-65 D6).
func resolveOverloads(name string, ops []*webidl.Operation, tm typemap.Mapper, diag *Diagnostics, idlName string) []overloadSig {
	base := opGoName(name)

	// Dispatch is by declared argument count plus a coarse runtime kind at one
	// position. optional/variadic arguments make the effective arity a range,
	// which this arity-bucketed scheme does not model — surface it rather than
	// silently mis-dispatching. (Full §3.2.11 effective-overload-set resolution,
	// including a per-pair distinguishing position, is future work alongside the
	// object-vs-object deferral — see F4 in the CATH-65 review.)
	if overloadHasVariadicOrOptional(ops) {
		diag.Add("warning", fmt.Sprintf("interface %q: overloaded operation %q has optional/variadic argument(s); dispatch is by declared argument count only — calls with an intermediate or extra argument count may not match", idlName, name))
	}

	byArity := map[int][]*webidl.Operation{}
	for _, op := range ops {
		a := len(op.Arguments)
		byArity[a] = append(byArity[a], op)
	}
	arities := make([]int, 0, len(byArity))
	for a := range byArity {
		arities = append(arities, a)
	}
	sort.Ints(arities)

	var sigs []overloadSig
	for _, a := range arities {
		group := byArity[a]
		if len(group) == 1 {
			sigs = append(sigs, overloadSigFor(group[0], fmt.Sprintf("%s%d", base, a), a, -1, classObject, tm, diag, idlName, name))
			continue
		}
		pos := distinguishingPos(group, tm)
		if pos < 0 {
			diag.Add("warning", fmt.Sprintf("interface %q: overloads of %q at %d argument(s) are not distinguishable by a single runtime argument kind — keeping the first, dropping %d (e.g. two object types, or two types sharing a coarse kind such as DOMString/USVString)", idlName, name, a, len(group)-1))
			sigs = append(sigs, overloadSigFor(group[0], fmt.Sprintf("%s%d", base, a), a, -1, classObject, tm, diag, idlName, name))
			continue
		}
		for _, op := range group {
			cls := classifyArg(op.Arguments[pos].IDLType, tm)
			tag := overloadTypeTag(op.Arguments[pos].IDLType, cls, tm)
			sigs = append(sigs, overloadSigFor(op, fmt.Sprintf("%s%d%s", base, a, tag), a, pos, cls, tm, diag, idlName, name))
		}
	}
	return sigs
}

func overloadSigFor(op *webidl.Operation, goName string, arity, pos int, cls argClass, tm typemap.Mapper, diag *Diagnostics, idlName, opName string) overloadSig {
	return overloadSig{
		goName:         goName,
		params:         buildParams(op.Arguments, tm, diag, idlName),
		returnType:     buildReturnType(op.ReturnType, tm, diag, idlName, opName),
		arity:          arity,
		distinguishPos: pos,
		class:          cls,
	}
}

// distinguishingPos returns the first argument position at which every operation
// in the group has a pairwise-distinct coarse runtime class, or -1 if none does
// (the object-vs-object / indistinguishable case).
func distinguishingPos(ops []*webidl.Operation, tm typemap.Mapper) int {
	minArity := len(ops[0].Arguments)
	for _, op := range ops {
		if len(op.Arguments) < minArity {
			minArity = len(op.Arguments)
		}
	}
	for pos := 0; pos < minArity; pos++ {
		seen := map[argClass]bool{}
		distinct := true
		for _, op := range ops {
			c := classifyArg(op.Arguments[pos].IDLType, tm)
			if seen[c] {
				distinct = false
				break
			}
			seen[c] = true
		}
		if distinct {
			return pos
		}
	}
	return -1
}
