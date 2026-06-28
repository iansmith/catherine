package codegen

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/iansmith/webidl/typemap"
	"github.com/iansmith/webidl/webidl"
)

// ---------------------------------------------------------------------------
// Internal representation
// ---------------------------------------------------------------------------

// ifaceMethod describes one method in a Go interface.
type ifaceMethod struct {
	goName     string
	params     []ifaceParam
	returnType string // "" for void
}

// ifaceParam is one parameter in a method or function signature.
type ifaceParam struct {
	goName   string
	goType   string
	variadic bool
}

// ---------------------------------------------------------------------------
// InterfaceDecl
// ---------------------------------------------------------------------------

// InterfaceDecl is a Decl that emits a Go interface type from a WebIDL interface.
type InterfaceDecl struct {
	typeName   string
	parentName string // "" means no parent embedding
	methods    []ifaceMethod
}

func (d *InterfaceDecl) declName() string { return d.typeName }

// declSource emits:
//
//	type Foo interface {
//	    ParentName        // only when parentName != ""
//	    Method(Param T) R
//	}
func (d *InterfaceDecl) declSource() string {
	var sb strings.Builder
	sb.WriteString("type ")
	sb.WriteString(d.typeName)
	sb.WriteString(" interface {\n")
	if d.parentName != "" {
		sb.WriteString("\t")
		sb.WriteString(d.parentName)
		sb.WriteString("\n")
	}
	for _, m := range d.methods {
		sb.WriteString("\t")
		sb.WriteString(m.goName)
		sb.WriteString("(")
		writeParams(&sb, m.params)
		sb.WriteString(")")
		if m.returnType != "" {
			sb.WriteString(" ")
			sb.WriteString(m.returnType)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

// ---------------------------------------------------------------------------
// ConstBlockDecl
// ---------------------------------------------------------------------------

type constEntry struct {
	goName string
	goType string
	value  string // Go literal
}

// ConstBlockDecl is a Decl that emits a package-level const block for constants
// that belong to a specific WebIDL interface.
type ConstBlockDecl struct {
	typeName  string // interface name — used as declName prefix for uniqueness
	constants []constEntry
}

func (d *ConstBlockDecl) declName() string { return d.typeName + "Consts" }

// declSource emits:
//
//	const (
//	    InterfaceNameConst GoType = value
//	)
func (d *ConstBlockDecl) declSource() string {
	var sb strings.Builder
	sb.WriteString("const (\n")
	for _, c := range d.constants {
		sb.WriteString("\t")
		sb.WriteString(c.goName)
		sb.WriteString(" ")
		sb.WriteString(c.goType)
		sb.WriteString(" = ")
		sb.WriteString(c.value)
		sb.WriteString("\n")
	}
	sb.WriteString(")\n")
	return sb.String()
}

// ---------------------------------------------------------------------------
// ConstructorDecl
// ---------------------------------------------------------------------------

// ConstructorDecl is a Decl that emits a factory function stub for a WebIDL
// interface constructor.
type ConstructorDecl struct {
	funcName  string // e.g. "NewEventTarget"
	ifaceName string // the return type (the Go interface name)
	params    []ifaceParam
}

func (d *ConstructorDecl) declName() string { return d.funcName }

// declSource emits:
//
//	func NewFoo(Param T) Foo {
//	    panic("NewFoo: not implemented")
//	}
func (d *ConstructorDecl) declSource() string {
	var sb strings.Builder
	sb.WriteString("func ")
	sb.WriteString(d.funcName)
	sb.WriteString("(")
	writeParams(&sb, d.params)
	sb.WriteString(") ")
	sb.WriteString(d.ifaceName)
	sb.WriteString(" {\n\tpanic(\"")
	sb.WriteString(d.funcName)
	sb.WriteString(": not implemented\")\n}\n")
	return sb.String()
}

// ---------------------------------------------------------------------------
// StaticFuncDecl
// ---------------------------------------------------------------------------

// StaticFuncDecl is a Decl that emits a package-level function stub for a
// WebIDL static operation or static attribute accessor.
type StaticFuncDecl struct {
	funcName   string
	params     []ifaceParam
	returnType string // "" for void
}

func (d *StaticFuncDecl) declName() string { return d.funcName }

// declSource emits:
//
//	func FooBar(Param T) R {
//	    panic("FooBar: not implemented")
//	}
func (d *StaticFuncDecl) declSource() string {
	var sb strings.Builder
	sb.WriteString("func ")
	sb.WriteString(d.funcName)
	sb.WriteString("(")
	writeParams(&sb, d.params)
	sb.WriteString(")")
	if d.returnType != "" {
		sb.WriteString(" ")
		sb.WriteString(d.returnType)
	}
	sb.WriteString(" {\n\tpanic(\"")
	sb.WriteString(d.funcName)
	sb.WriteString(": not implemented\")\n}\n")
	return sb.String()
}

// ---------------------------------------------------------------------------
// CallbackFuncDecl
// ---------------------------------------------------------------------------

// CallbackFuncDecl is a Decl that emits `type FooFunc func(...)` for a
// WebIDL callback function. When raisesException is true the emitted signature
// includes an error return (or an additional error in a tuple return).
type CallbackFuncDecl struct {
	typeName        string
	params          []ifaceParam
	returnType      string // "" = void
	raisesException bool
}

func (d *CallbackFuncDecl) declName() string { return d.typeName }

// declSource emits one of:
//
//	type FooFunc func(T1, T2)               void, no exception
//	type FooFunc func(T1, T2) error         void, raises exception
//	type FooFunc func(T1, T2) R             non-void, no exception
//	type FooFunc func(T1, T2) (R, error)    non-void, raises exception
func (d *CallbackFuncDecl) declSource() string {
	var sb strings.Builder
	sb.WriteString("type ")
	sb.WriteString(d.typeName)
	sb.WriteString(" func(")
	// func type literals use type-only params (names not required and would be
	// non-idiomatic in a type alias context)
	for i, p := range d.params {
		if i > 0 {
			sb.WriteString(", ")
		}
		if p.variadic {
			sb.WriteString("...")
		}
		sb.WriteString(p.goType)
	}
	sb.WriteString(")")
	switch {
	case d.returnType == "" && !d.raisesException:
		// void, no error — no return clause
	case d.returnType == "" && d.raisesException:
		sb.WriteString(" error")
	case d.returnType != "" && !d.raisesException:
		sb.WriteString(" ")
		sb.WriteString(d.returnType)
	default: // returnType != "" && raisesException
		sb.WriteString(" (")
		sb.WriteString(d.returnType)
		sb.WriteString(", error)")
	}
	sb.WriteString("\n")
	return sb.String()
}

// ---------------------------------------------------------------------------
// EntryTypeDecl
// ---------------------------------------------------------------------------

// EntryTypeDecl is a Decl that emits the generic Entry[K, V any] struct used
// as the element type of async_iterable<K, V> pair iterators. Add it to a
// File at most once — File.Render rejects duplicate declNames.
type EntryTypeDecl struct{}

func (d *EntryTypeDecl) declName() string { return "Entry" }

// declSource emits:
//
//	type Entry[K, V any] struct { Key K; Value V }
func (d *EntryTypeDecl) declSource() string {
	return "type Entry[K, V any] struct {\n\tKey   K\n\tValue V\n}\n"
}

// ---------------------------------------------------------------------------
// NewInterfaceDecls — entry point
// ---------------------------------------------------------------------------

// NewInterfaceDecls produces all Decl values needed for a single WebIDL
// interface definition. The returned order is always:
//  1. InterfaceDecl (or CallbackFuncDecl for single-method callbacks)
//  2. ConstBlockDecl (only when the interface has constants)
//  3. ConstructorDecl (only when the interface has a constructor)
//  4. StaticFuncDecl values (one per static operation or attribute accessor)
//  5. EntryTypeDecl (only when a pair async_iterable is present)
//
// Returns nil for IfaceMixin (already merged by IR; transparent to codegen).
// Returns nil for zero-method callback interfaces (error added to diag).
// diag must not be nil.
func NewInterfaceDecls(def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics) []Decl {
	if diag == nil {
		diag = NewDiagnostics()
	}
	if def == nil {
		return nil
	}
	iface, ok := def.Primary.(*webidl.Interface)
	if !ok {
		return nil
	}
	switch iface.Variant {
	case webidl.IfaceMixin:
		return nil
	case webidl.IfaceCallback:
		return buildCallbackDecls(iface.Name, def.Members, tm, diag)
	default:
		return buildRegularDecls(iface, def, tm, diag)
	}
}

// ---------------------------------------------------------------------------
// Callback path
// ---------------------------------------------------------------------------

func buildCallbackDecls(idlName string, members []webidl.Member, tm typemap.Mapper, diag *Diagnostics) []Decl {
	typeName := IdentSanitize(idlName)

	var ops []*webidl.Operation
	for _, m := range members {
		if op, ok := m.(*webidl.Operation); ok {
			ops = append(ops, op)
		}
	}

	switch len(ops) {
	case 0:
		diag.Add("error", fmt.Sprintf("callback interface %q has no operations; skipping", idlName))
		return nil
	case 1:
		op := ops[0]
		params := buildParams(op.Arguments, tm, diag, idlName)
		retType := buildReturnType(op.ReturnType, tm, diag, idlName, op.Name)
		raisesException := false
		for _, attr := range op.ExtAttrs {
			if attr.Name == "RaisesException" {
				raisesException = true
				break
			}
		}
		return []Decl{&CallbackFuncDecl{
			typeName:        typeName + "Func",
			params:          params,
			returnType:      retType,
			raisesException: raisesException,
		}}
	default:
		methods := buildOpsAsMethods(ops, tm, diag, idlName)
		return []Decl{&InterfaceDecl{typeName: typeName, methods: methods}}
	}
}

// ---------------------------------------------------------------------------
// Regular interface path
// ---------------------------------------------------------------------------

func buildRegularDecls(iface *webidl.Interface, def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics) []Decl {
	idlName := iface.Name
	typeName := IdentSanitize(idlName)

	if !hasAlnum(idlName) {
		diag.Add("error", fmt.Sprintf("interface name %q has no letter or digit content; cannot produce a valid Go type name", idlName))
		return nil
	}

	idecl := &InterfaceDecl{typeName: typeName}
	if iface.Inheritance != "" {
		idecl.parentName = IdentSanitize(iface.Inheritance)
	}

	seenMethods := make(map[string]bool)
	var needsEntry bool

	// Overloaded operations (same name, >1 signature) declare one method per
	// overload (CATH-65 D6) instead of the first-wins drop; the binding dispatches
	// into exactly these. Detect them up front so the walk skips the duplicates.
	overloads := groupOverloads(def.Members)
	emittedOverload := make(map[string]bool)

	for _, mem := range def.Members {
		switch m := mem.(type) {
		case *webidl.Attribute:
			if m.Special == "static" {
				continue
			}
			addAttrMethods(idecl, m, tm, diag, idlName, seenMethods)
		case *webidl.Operation:
			if m.Special == "static" {
				continue
			}
			if m.Special == "" && m.Name != "" {
				if grp, multi := overloads[m.Name]; multi {
					if !emittedOverload[m.Name] {
						emittedOverload[m.Name] = true
						addOverloadMethods(idecl, m.Name, grp, tm, diag, idlName, seenMethods)
					}
					continue
				}
			}
			addOpMethod(idecl, m, tm, diag, idlName, seenMethods)
		case *webidl.IterableLike:
			if m.Kind == webidl.IterAsyncIterable && len(m.Types) >= 2 {
				needsEntry = true
			}
			addIterMethods(idecl, m, tm, diag, idlName, seenMethods)
		}
	}

	var decls []Decl
	decls = append(decls, idecl)

	if cb := buildConstBlock(typeName, def.Members, tm, diag, idlName); cb != nil {
		decls = append(decls, cb)
	}
	if cd := buildConstructorDecl(typeName, def.Members, tm, diag, idlName); cd != nil {
		decls = append(decls, cd)
	}
	decls = append(decls, buildStaticDecls(typeName, def.Members, tm, diag, idlName)...)
	if needsEntry {
		decls = append(decls, &EntryTypeDecl{})
	}
	return decls
}

// ---------------------------------------------------------------------------
// Attribute methods
// ---------------------------------------------------------------------------

func addAttrMethods(idecl *InterfaceDecl, attr *webidl.Attribute, tm typemap.Mapper, diag *Diagnostics, idlName string, seen map[string]bool) {
	// [Reflect] attrs are reflected end-to-end by the binding over the attribute
	// store (CATH-65 D1), so layer-1 declares NO method for them. Trimmed via the
	// shared reflectedAttr gate — the same predicate binding.go emits on — so the
	// two backends agree on exactly which attrs have a layer-1 method.
	if _, _, ok := reflectedAttr(attr, tm); ok {
		return
	}
	goBaseName := IdentSanitize(attr.Name)
	if !validGoIdentBase(goBaseName) {
		diag.Add("error", fmt.Sprintf("interface %q: attribute %q sanitizes to invalid Go identifier %q; skipping", idlName, attr.Name, goBaseName))
		return
	}
	getterName := attrGetterName(attr.Name)
	gt, err := tm.MapType(attr.IDLType)
	if err != nil {
		diag.Add("error", fmt.Sprintf("interface %q: cannot map type for attribute %q: %v", idlName, attr.Name, err))
		return
	}
	if gt.Unresolved {
		diag.Add("warning", fmt.Sprintf("interface %q: attribute %q maps to unresolved type %q", idlName, attr.Name, gt.String()))
	}
	typeStr := gt.String()

	if seen[getterName] {
		diag.Add("error", fmt.Sprintf("interface %q: attribute getter %q dropped — collision (first wins)", idlName, attr.Name))
		return
	}
	seen[getterName] = true
	idecl.methods = append(idecl.methods, ifaceMethod{goName: getterName, returnType: typeStr})

	// Stringifier attribute also provides String() string
	if attr.Special == "stringifier" && !seen["String"] {
		seen["String"] = true
		idecl.methods = append(idecl.methods, ifaceMethod{goName: "String", returnType: "string"})
	}

	if attr.Readonly {
		return
	}
	setterName := attrSetterName(attr.Name)
	if seen[setterName] {
		diag.Add("error", fmt.Sprintf("interface %q: attribute setter %q dropped — collision (first wins)", idlName, attr.Name))
		return
	}
	seen[setterName] = true
	idecl.methods = append(idecl.methods, ifaceMethod{
		goName: setterName,
		params: []ifaceParam{{goName: "V", goType: typeStr}},
	})
}

// ---------------------------------------------------------------------------
// Operation methods
// ---------------------------------------------------------------------------

func addOpMethod(idecl *InterfaceDecl, op *webidl.Operation, tm typemap.Mapper, diag *Diagnostics, idlName string, seen map[string]bool) {
	// Stringifier: named or unnamed, with or without return type
	if op.Special == "stringifier" {
		if !seen["String"] {
			seen["String"] = true
			idecl.methods = append(idecl.methods, ifaceMethod{goName: "String", returnType: "string"})
		}
		return
	}

	// Special indexed operations
	switch op.Special {
	case "getter":
		retType := buildReturnType(op.ReturnType, tm, diag, idlName, "getter")
		addSpecialMethod(idecl, idxGetterGoName, []ifaceParam{{goName: "I", goType: idxKeyGoType}}, retType, seen, diag, idlName)
		return
	case "setter":
		valType := "any"
		if len(op.Arguments) >= 2 {
			gt, err := tm.MapType(op.Arguments[1].IDLType)
			if err == nil {
				valType = gt.String()
			}
		}
		addSpecialMethod(idecl, idxSetterGoName, []ifaceParam{{goName: "I", goType: idxKeyGoType}, {goName: "V", goType: valType}}, "", seen, diag, idlName)
		return
	case "deleter":
		addSpecialMethod(idecl, idxDeleterGoName, []ifaceParam{{goName: "I", goType: idxKeyGoType}}, "", seen, diag, idlName)
		return
	}

	// Regular named operation
	if op.Name == "" {
		return // anonymous with no known special — skip
	}
	goName := opGoName(op.Name)
	if !validGoIdentBase(goName) {
		diag.Add("error", fmt.Sprintf("interface %q: operation %q sanitizes to invalid Go identifier %q; skipping", idlName, op.Name, goName))
		return
	}
	if seen[goName] {
		diag.Add("error", fmt.Sprintf("interface %q: operation %q dropped — name collision (first wins)", idlName, op.Name))
		return
	}
	seen[goName] = true
	params := buildParams(op.Arguments, tm, diag, idlName)
	retType := buildReturnType(op.ReturnType, tm, diag, idlName, op.Name)
	idecl.methods = append(idecl.methods, ifaceMethod{goName: goName, params: params, returnType: retType})
}

// addOverloadMethods declares one layer-1 method per overload of an operation
// name, using the shared resolveOverloads (members.go) so the binding dispatches
// into exactly these methods (CATH-65 D6).
func addOverloadMethods(idecl *InterfaceDecl, name string, ops []*webidl.Operation, tm typemap.Mapper, diag *Diagnostics, idlName string, seen map[string]bool) {
	for _, s := range resolveOverloads(name, ops, tm, diag, idlName) {
		if seen[s.goName] {
			diag.Add("error", fmt.Sprintf("interface %q: overloaded operation %q → method %q dropped — name collision (first wins)", idlName, name, s.goName))
			continue
		}
		seen[s.goName] = true
		idecl.methods = append(idecl.methods, ifaceMethod{goName: s.goName, params: s.params, returnType: s.returnType})
	}
}

func addSpecialMethod(idecl *InterfaceDecl, goName string, params []ifaceParam, retType string, seen map[string]bool, diag *Diagnostics, idlName string) {
	if seen[goName] {
		diag.Add("error", fmt.Sprintf("interface %q: special operation %q already defined — duplicate skipped", idlName, goName))
		return
	}
	seen[goName] = true
	idecl.methods = append(idecl.methods, ifaceMethod{goName: goName, params: params, returnType: retType})
}

// ---------------------------------------------------------------------------
// IterableLike methods
// ---------------------------------------------------------------------------

func addIterMethods(idecl *InterfaceDecl, it *webidl.IterableLike, tm typemap.Mapper, diag *Diagnostics, idlName string, seen map[string]bool) {
	// resolveIterMethods (members.go) is the single source of truth for the
	// per-kind method set, readonly gating, and key/value arity — shared with the
	// binding backend so the two cannot drift. Here we just declare each method,
	// keeping the layer-1 first-wins collision rule.
	for _, m := range resolveIterMethods(it, tm, diag, idlName) {
		if seen[m.goName] {
			diag.Add("warning", fmt.Sprintf("interface %q: iterable method %q conflicts with existing member — skipped", idlName, m.goName))
			continue
		}
		seen[m.goName] = true
		idecl.methods = append(idecl.methods, ifaceMethod{goName: m.goName, params: m.params, returnType: m.returnType})
	}
}

// ---------------------------------------------------------------------------
// Const, constructor, and static helpers
// ---------------------------------------------------------------------------

func buildConstBlock(typeName string, members []webidl.Member, tm typemap.Mapper, diag *Diagnostics, idlName string) *ConstBlockDecl {
	rcs := resolveConstants(typeName, members, tm, diag, idlName)
	if len(rcs) == 0 {
		return nil
	}
	entries := make([]constEntry, len(rcs))
	for i, rc := range rcs {
		entries[i] = constEntry{
			goName: rc.goName,
			goType: rc.goType,
			value:  constValueLit(rc.value),
		}
	}
	return &ConstBlockDecl{typeName: typeName, constants: entries}
}

func buildConstructorDecl(typeName string, members []webidl.Member, tm typemap.Mapper, diag *Diagnostics, idlName string) *ConstructorDecl {
	var first *webidl.Constructor
	var overloads int
	for _, mem := range members {
		c, ok := mem.(*webidl.Constructor)
		if !ok {
			continue
		}
		if first == nil {
			first = c
		} else {
			overloads++
		}
	}
	if first == nil {
		return nil
	}
	if overloads > 0 {
		diag.Add("error", fmt.Sprintf("interface %q: %d constructor overload(s) dropped (first wins)", idlName, overloads))
	}
	params := buildParams(first.Arguments, tm, diag, idlName)
	return &ConstructorDecl{
		funcName:  "New" + typeName,
		ifaceName: typeName,
		params:    params,
	}
}

func buildStaticDecls(typeName string, members []webidl.Member, tm typemap.Mapper, diag *Diagnostics, idlName string) []Decl {
	seen := make(map[string]bool)
	var decls []Decl
	for _, mem := range members {
		switch m := mem.(type) {
		case *webidl.Operation:
			if m.Special != "static" || m.Name == "" {
				continue
			}
			funcName := typeName + IdentSanitize(m.Name)
			if seen[funcName] {
				diag.Add("error", fmt.Sprintf("interface %q: static operation %q dropped — collision (first wins)", idlName, m.Name))
				continue
			}
			seen[funcName] = true
			params := buildParams(m.Arguments, tm, diag, idlName)
			retType := buildReturnType(m.ReturnType, tm, diag, idlName, m.Name)
			decls = append(decls, &StaticFuncDecl{funcName: funcName, params: params, returnType: retType})

		case *webidl.Attribute:
			if m.Special != "static" {
				continue
			}
			gt, err := tm.MapType(m.IDLType)
			typeStr := "any"
			if err != nil {
				diag.Add("error", fmt.Sprintf("interface %q: cannot map type for static attribute %q: %v", idlName, m.Name, err))
			} else {
				typeStr = gt.String()
			}
			getterName := typeName + "Get" + IdentSanitize(m.Name)
			if !seen[getterName] {
				seen[getterName] = true
				decls = append(decls, &StaticFuncDecl{funcName: getterName, returnType: typeStr})
			}
			if m.Readonly {
				continue
			}
			setterName := typeName + "Set" + IdentSanitize(m.Name)
			if !seen[setterName] {
				seen[setterName] = true
				decls = append(decls, &StaticFuncDecl{
					funcName: setterName,
					params:   []ifaceParam{{goName: "V", goType: typeStr}},
				})
			}
		}
	}
	return decls
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// buildOpsAsMethods converts a slice of *webidl.Operation to []ifaceMethod.
// Used for multi-method callback interfaces.
func buildOpsAsMethods(ops []*webidl.Operation, tm typemap.Mapper, diag *Diagnostics, idlName string) []ifaceMethod {
	seen := make(map[string]bool)
	idecl := &InterfaceDecl{}
	for _, op := range ops {
		addOpMethod(idecl, op, tm, diag, idlName, seen)
	}
	return idecl.methods
}

// buildParams converts a slice of *webidl.Argument to []ifaceParam.
func buildParams(args []*webidl.Argument, tm typemap.Mapper, diag *Diagnostics, idlName string) []ifaceParam {
	var params []ifaceParam
	for _, arg := range args {
		argGoName := IdentSanitize(arg.Name)
		if argGoName == "" {
			argGoName = "Arg"
		}
		gt, err := tm.MapType(arg.IDLType)
		typeStr := "any"
		if err != nil {
			diag.Add("error", fmt.Sprintf("interface %q: cannot map argument type for %q: %v", idlName, arg.Name, err))
		} else {
			typeStr = gt.String()
		}
		params = append(params, ifaceParam{goName: argGoName, goType: typeStr, variadic: arg.Variadic})
	}
	return params
}

// buildReturnType maps an IDLType to a Go return-type string. Returns "" for
// nil receivers (anonymous stringifier body-less form) and for undefined/void.
func buildReturnType(t *webidl.IDLType, tm typemap.Mapper, diag *Diagnostics, idlName, opName string) string {
	if isVoidReturn(t) {
		return ""
	}
	gt, err := tm.MapType(t)
	if err != nil {
		diag.Add("error", fmt.Sprintf("interface %q: cannot map return type for op %q: %v", idlName, opName, err))
		return "any"
	}
	return gt.String()
}

// constValueLit renders a *webidl.ConstValue as a Go literal string.
func constValueLit(cv *webidl.ConstValue) string {
	if cv == nil {
		return "0"
	}
	switch cv.Kind {
	case webidl.CVNumber:
		return cv.Number
	case webidl.CVBoolean:
		if cv.Bool {
			return "true"
		}
		return "false"
	case webidl.CVNull:
		return "nil"
	case webidl.CVInfinity:
		if cv.Negative {
			return "math.Inf(-1)"
		}
		return "math.Inf(1)"
	case webidl.CVNaN:
		return "math.NaN()"
	default:
		return "0"
	}
}

// writeParams writes a comma-separated parameter list into sb.
func writeParams(sb *strings.Builder, params []ifaceParam) {
	for i, p := range params {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(p.goName)
		sb.WriteString(" ")
		if p.variadic {
			sb.WriteString("...")
		}
		sb.WriteString(p.goType)
	}
}

// hasAlnum reports whether s contains at least one letter or digit. A name
// without one sanitizes to the fallback identifier "X", which is valid Go but
// almost certainly a caller bug.
func hasAlnum(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}
