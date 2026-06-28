package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iansmith/webidl/typemap"
	"github.com/iansmith/webidl/webidl"
)

// This file is the goja JS-binding backend (CATH-64, under CATH-62). For each
// regular interface it emits a Go type implementing goja's DynamicObject
// interface (Get/Set/Has/Delete/Keys) whose cases coerce JS values and dispatch
// into the already-generated layer-1 Go interface (see iface.go). The binding
// is the "second backend": it is emitted to its own output (GenerateBindings →
// bindings.go), not mixed into the layer-1 generated.go.
//
// Shape (interface I with parent P):
//
//	type IBinding struct {
//		ctx    *bindCtx   // runtime context (vm, identity cache) — defined by CATH-66
//		impl   I          // the engine object satisfying the layer-1 interface
//		parent *PBinding  // present only when I inherits; inherited keys delegate here
//	}
//
// Inheritance is embed-and-delegate: IBinding handles its own members, and
// Get/Set/Has/Delete/Keys fall through to b.parent for everything else.
//
// # Runtime shim contract (implemented by CATH-66)
//
// The generated bodies reference a small runtime surface that this package does
// NOT define; CATH-66 supplies it. Emitting these names here fixes the contract:
//
//	type bindCtx struct { vm *goja.Runtime; /* identity cache, etc. */ }
//	func coerce[T any](ctx *bindCtx, v goja.Value) T   // JS value → Go arg
//	func asArrayIndex(key string) (uint32, bool)        // "0" → 0,true ; "x" → _,false
//	func (ctx *bindCtx) wrapSeq(any) goja.Value          // iter.Seq → JS iterator
//	func (ctx *bindCtx) callbackFn(v goja.Value) func(...) // JS fn → Go callback
//
// CATH-65 adds the following to the contract:
//
//	// [Reflect] — read/write a content attribute on the impl's attribute store.
//	// impl is passed as `any` (the layer-1 method is trimmed, so no typed
//	// accessor exists); CATH-66 type-asserts it to the attribute-store interface.
//	// Integer getters default to 0 when the attribute is absent/unparseable.
//	func (ctx *bindCtx) reflectGetString(impl any, name string) string
//	func (ctx *bindCtx) reflectSetString(impl any, name, v string)
//	func (ctx *bindCtx) reflectGetBool(impl any, name string) bool       // presence-based
//	func (ctx *bindCtx) reflectSetBool(impl any, name string, v bool)
//	func (ctx *bindCtx) reflectGetInt32(impl any, name string) int32
//	func (ctx *bindCtx) reflectSetInt32(impl any, name string, v int32)
//	func (ctx *bindCtx) reflectGetUint32(impl any, name string) uint32
//	func (ctx *bindCtx) reflectSetUint32(impl any, name string, v uint32)
//
//	// [SameObject] — memoize an attribute's object so repeated reads are ===.
//	func (ctx *bindCtx) sameObject(owner any, key string, compute func() goja.Value) goja.Value
//
//	// Overload dispatch — coarse runtime kind of a JS argument.
//	type Kind int
//	const ( KindUndefined Kind = iota; KindNull; KindBoolean; KindNumber; KindString; KindObject )
//	func (ctx *bindCtx) argKind(v goja.Value) Kind
//
// GenerateBindings additionally emits an exposure registry:
//
//	type ExposedBinding struct {
//		Name    string
//		Globals []string
//		New     func(ctx *bindCtx, impl any) goja.Value // wraps a layer-1 impl
//	}
//	var ExposedBindings []ExposedBinding
//
// Because of these references the output is gofmt-valid but does not fully
// compile standalone until the CATH-66 shim lands. goja itself is intentionally
// NOT a dependency of this module — the generator emits source text; the
// consumer (louis14) provides goja.

// NewBindingDecls emits the DynamicObject accessor decls for the interface in
// def. Returns nil for a nil def, a non-interface primary, or a mixin/callback
// interface (only regular interfaces get a binding accessor).
func NewBindingDecls(def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics) []Decl {
	// Standalone callers target the default global; GenerateBindings overrides it
	// from Options.ExposureGlobal. exposed=nil → no parent-exposure filtering (a
	// single interface carries no IR to check its parent against).
	return newBindingDeclsFor(def, tm, diag, "Window", nil)
}

// newBindingDeclsFor is NewBindingDecls parameterized by the target [Exposed]
// global and the set of exposed Go type names (for parent-delegation filtering;
// nil disables it). An interface not exposed to global gets no binding (CATH-65 D4).
func newBindingDeclsFor(def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics, global string, exposed map[string]bool) []Decl {
	if def == nil {
		return nil
	}
	iface, ok := def.Primary.(*webidl.Interface)
	if !ok || iface.Variant != webidl.IfaceRegular {
		return nil
	}
	if !hasAlnum(iface.Name) {
		diag.Add("error", fmt.Sprintf("interface name %q has no letter or digit content; cannot produce a binding type", iface.Name))
		return nil
	}
	if !exposedTo(ParseExtAttrs(iface.ExtAttrs, diag).ExposedScopes, global) {
		return nil
	}

	b := &bindingBuilder{
		typeName: IdentSanitize(iface.Name),
		idlName:  iface.Name,
		tm:       tm,
		diag:     diag,
		goSeen:   make(map[string]bool),
		jsSeen:   make(map[string]bool),
	}
	if iface.Inheritance != "" {
		parent := IdentSanitize(iface.Inheritance)
		// Only delegate to the parent's binding when that binding is actually
		// emitted. With [Exposed] filtering a child can be exposed while its parent
		// is not (reachable only under spec-invalid IDL — GenerateBindings does not
		// re-validate the exposure-subset rule); referencing an unemitted
		// *ParentBinding would produce non-compiling output. exposed == nil
		// (standalone NewBindingDecls) keeps the unconditional behavior.
		if exposed == nil || exposed[parent] {
			b.parentName = parent
		} else {
			diag.Add("warning", fmt.Sprintf("interface %q: parent %q is not exposed to %q; emitting %q without parent delegation", iface.Name, iface.Inheritance, global, b.typeName))
		}
	}

	// Overloaded operations (same name, >1 signature) are emitted once as a
	// single arg-shape dispatcher rather than dropped first-wins; detect them up
	// front so the member walk skips the duplicates.
	overloads := groupOverloads(def.Members)
	emittedOverload := make(map[string]bool)
	for _, mem := range def.Members {
		if op, ok := mem.(*webidl.Operation); ok && op.Special == "" && op.Name != "" {
			if grp, multi := overloads[op.Name]; multi {
				if !emittedOverload[op.Name] {
					emittedOverload[op.Name] = true
					b.addOverloadedOperation(op.Name, grp)
				}
				continue
			}
		}
		b.add(mem)
	}
	b.addConstants(def.Members)
	return []Decl{b.toDecl()}
}

// bindingBuilder accumulates the rendered fragments of one binding type as it
// walks an interface's members. It enforces the SAME drop decisions the layer-1
// generator (iface.go) makes — using the shared name derivations in members.go —
// so the binding never dispatches into a method the interface did not generate.
//
// Two dedup namespaces are tracked:
//   - goSeen: Go method names actually emitted by the layer-1 interface (mirrors
//     iface.go's per-method seen set). A member whose Go name is already claimed
//     is dropped, exactly as iface.go drops it.
//   - jsSeen: the JS keys used as `case` labels across Get/Set/Has. This is the
//     binding's own constraint — every member kind shares one switch, so a key
//     may appear at most once or the emitted Go has a duplicate `case` label.
type bindingBuilder struct {
	typeName   string
	parentName string // "" if the interface has no parent
	idlName    string
	tm         typemap.Mapper
	diag       *Diagnostics
	goSeen     map[string]bool // Go method names (mirrors iface.go)
	jsSeen     map[string]bool // JS switch keys (binding-specific)

	getCases []string // rendered `case "x": ...` blocks for Get
	setCases []string // rendered `case "x": ...` blocks for Set
	keyNames []string // JS-visible enumerable names, in declaration order

	stringifier    bool
	indexGetter    bool
	indexGetterRet bool // true when the indexed getter returns a value (not void)
	indexSetter    bool
	indexDeleter   bool
	indexValType   string // V type for SetIndex coercion
}

// claimMethod reserves a declared member's (attribute/operation) Go name + JS
// key. A collision is malformed IDL (duplicate member name / Go-name clash) — an
// error, first-wins — matching iface.go's behaviour for attributes/operations.
func (b *bindingBuilder) claimMethod(jsName, goName string) bool {
	if b.goSeen[goName] || b.jsSeen[jsName] {
		b.diag.Add("error", fmt.Sprintf("interface %q: binding member %q dropped — collision (first wins)", b.idlName, jsName))
		return false
	}
	b.goSeen[goName] = true
	b.jsSeen[jsName] = true
	return true
}

// claimInjected reserves a method the binding INJECTS (iteration methods,
// toString) that legitimately collides with a declared member on real
// interfaces (e.g. a maplike `get` vs a declared `get()` operation). iface.go
// drops the injected method with a warning in this case; the binding does the
// same — non-fatal, so the rest of the interface still generates.
func (b *bindingBuilder) claimInjected(jsName, goName string) bool {
	if b.goSeen[goName] || b.jsSeen[jsName] {
		b.diag.Add("warning", fmt.Sprintf("interface %q: binding %q skipped — collides with an existing member", b.idlName, jsName))
		return false
	}
	b.goSeen[goName] = true
	b.jsSeen[jsName] = true
	return true
}

// claimGoName reserves a special operation's Go method name (Index/SetIndex/
// Delete). These have no JS switch key — they are numeric-index access — so only
// the Go-name namespace is claimed. A collision with an already-claimed member
// (e.g. a named op `delete` → Delete) drops the special op, first-wins, exactly
// as iface.go's addSpecialMethod does. Without this the two backends could
// disagree and the binding would dispatch into a method the interface lacks.
func (b *bindingBuilder) claimGoName(goName string) bool {
	if b.goSeen[goName] {
		b.diag.Add("error", fmt.Sprintf("interface %q: binding indexed operation dropped — Go name %q already claimed (first wins)", b.idlName, goName))
		return false
	}
	b.goSeen[goName] = true
	return true
}

// claimConstKey reserves a constant's JS key. The constant's Go name is already
// deduped by resolveConstants (its own namespace); this only guards the switch
// key against a method of the same name. Non-fatal (skip with a warning).
func (b *bindingBuilder) claimConstKey(jsName string) bool {
	if b.jsSeen[jsName] {
		b.diag.Add("warning", fmt.Sprintf("interface %q: binding constant %q skipped — collides with an existing member", b.idlName, jsName))
		return false
	}
	b.jsSeen[jsName] = true
	return true
}

func (b *bindingBuilder) goType(t *webidl.IDLType) string {
	gt, err := b.tm.MapType(t)
	if err != nil {
		b.diag.Add("error", fmt.Sprintf("interface %q: binding cannot map type: %v", b.idlName, err))
		return "any"
	}
	if gt.Unresolved {
		b.diag.Add("warning", fmt.Sprintf("interface %q: binding maps a member to unresolved type %q", b.idlName, gt.String()))
	}
	return gt.String()
}

func (b *bindingBuilder) add(mem webidl.Member) {
	switch m := mem.(type) {
	case *webidl.Attribute:
		b.addAttribute(m)
	case *webidl.Operation:
		b.addOperation(m)
	case *webidl.IterableLike:
		b.addIterable(m)
	}
	// *webidl.Constant is handled in a separate pass via resolveConstants so the
	// binding and the layer-1 const block agree on which constants survive.
}

func (b *bindingBuilder) addAttribute(a *webidl.Attribute) {
	if a.Special == "static" {
		return
	}

	// [Reflect] (CATH-65 D1/D3): read/write the content attribute directly via
	// the CATH-66 reflect shim, bypassing layer-1. iface.go trims the layer-1
	// method for the same attrs (gated on the shared reflectedAttr predicate), so
	// there is no Go method name to claim — only the JS switch key.
	//
	// This MUST run before the stringifier flag below: iface.go reflect-trims the
	// attr before its own stringifier branch, so a reflected `stringifier
	// attribute` has no layer-1 String(). Were we to set b.stringifier here, the
	// binding would emit a toString dispatching into a b.impl.String() the
	// interface never declares — a cross-backend divergence.
	if domName, kind, ok := reflectedAttr(a, b.tm); ok {
		b.addReflectedAttr(a, domName, kind)
		return
	}

	if a.Special == "stringifier" {
		b.stringifier = true
	}
	if !validGoIdentBase(IdentSanitize(a.Name)) {
		b.diag.Add("error", fmt.Sprintf("interface %q: attribute %q sanitizes to invalid Go identifier; skipping", b.idlName, a.Name))
		return
	}

	set := ParseExtAttrs(a.ExtAttrs, b.diag)
	if set.ReflectPresent {
		b.diag.Add("warning", fmt.Sprintf("interface %q: [Reflect] on attribute %q of non-reflectable type — keeping the layer-1 accessor", b.idlName, a.Name))
	}

	getter := attrGetterName(a.Name)
	if !b.claimMethod(a.Name, getter) {
		return
	}

	// Getter — wrapped in the identity cache for [SameObject] (CATH-65 D7), which
	// is only meaningful on a readonly object-typed attribute; otherwise the
	// directive is dropped with a diagnostic.
	getExpr := fmt.Sprintf("b.ctx.vm.ToValue(b.impl.%s())", getter)
	if set.SameObject {
		if a.Readonly && isObjectType(a.IDLType, b.tm) {
			getExpr = fmt.Sprintf("b.ctx.sameObject(b.impl, %q, func() goja.Value { return b.ctx.vm.ToValue(b.impl.%s()) })", a.Name, getter)
		} else {
			b.diag.Add("warning", fmt.Sprintf("interface %q: [SameObject] on attribute %q ignored — requires a readonly object-typed attribute", b.idlName, a.Name))
		}
	}
	b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:%s\n\t\treturn %s", a.Name, noopMarker(set), getExpr))
	b.keyNames = append(b.keyNames, a.Name)

	if a.Readonly {
		// [PutForwards=x] (CATH-65 D8): assignment forwards to .x on the returned
		// object. The target attribute's type is not resolvable here without a
		// cross-interface lookup, so coerce to string — the common URL/stringifier
		// case; non-string PutForwards targets are future work.
		if set.PutForwards != "" {
			b.setCases = append(b.setCases, fmt.Sprintf("\tcase %q:\n\t\tb.impl.%s().%s(coerce[string](b.ctx, val))\n\t\treturn true", a.Name, getter, attrSetterName(set.PutForwards)))
		}
		// [Replaceable] (CATH-65 D8): deferred — own-data-property-on-assign needs
		// the CATH-66 runtime (a replaceProperty shim). Diagnostic marker only.
		if set.Replaceable {
			b.diag.Add("warning", fmt.Sprintf("interface %q: [Replaceable] on attribute %q not implemented — deferred (see CATH-65)", b.idlName, a.Name))
		}
		return
	}
	typeStr := b.goType(a.IDLType)
	b.setCases = append(b.setCases, fmt.Sprintf("\tcase %q:\n\t\tb.impl.%s(coerce[%s](b.ctx, val))\n\t\treturn true", a.Name, attrSetterName(a.Name), typeStr))
}

// addReflectedAttr emits a Get (and, for writable attrs, Set) case that routes
// through the CATH-66 reflect shim instead of any layer-1 method.
func (b *bindingBuilder) addReflectedAttr(a *webidl.Attribute, domName string, kind reflectKind) {
	if b.jsSeen[a.Name] {
		b.diag.Add("error", fmt.Sprintf("interface %q: reflected attribute %q dropped — collision (first wins)", b.idlName, a.Name))
		return
	}
	b.jsSeen[a.Name] = true
	b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:\n\t\treturn b.ctx.vm.ToValue(b.ctx.reflectGet%s(b.impl, %q))", a.Name, kind.shimSuffix(), domName))
	b.keyNames = append(b.keyNames, a.Name)
	if !a.Readonly {
		b.setCases = append(b.setCases, fmt.Sprintf("\tcase %q:\n\t\tb.ctx.reflectSet%s(b.impl, %q, coerce[%s](b.ctx, val))\n\t\treturn true", a.Name, kind.shimSuffix(), domName, kind.goType()))
	}
}

// noopMarker returns a trailing line-comment naming the recognized-but-no-op
// extended attributes present (CEReactions/NewObject/Unscopable), or "" if none.
// They have no binding behavior in CATH-65 (D9) — the comment keeps them
// greppable in the generated source.
func noopMarker(set ExtAttrSet) string {
	var names []string
	if set.CEReactions {
		names = append(names, "[CEReactions]")
	}
	if set.NewObject {
		names = append(names, "[NewObject]")
	}
	if set.Unscopable {
		names = append(names, "[Unscopable]")
	}
	if len(names) == 0 {
		return ""
	}
	return " // " + strings.Join(names, " ") + " recognized, not implemented (CATH-65)"
}

func (b *bindingBuilder) addOperation(op *webidl.Operation) {
	if op.Special != "" {
		b.addSpecialOp(op)
		return
	}
	if op.Name == "" {
		return // anonymous, non-special — nothing to dispatch
	}
	goName := opGoName(op.Name)
	if !validGoIdentBase(goName) {
		b.diag.Add("error", fmt.Sprintf("interface %q: operation %q sanitizes to invalid Go identifier; skipping", b.idlName, op.Name))
		return
	}
	if !b.claimMethod(op.Name, goName) {
		return
	}
	marker := noopMarker(ParseExtAttrs(op.ExtAttrs, b.diag))
	args := make([]string, 0, len(op.Arguments))
	for i, a := range op.Arguments {
		args = append(args, fmt.Sprintf("coerce[%s](b.ctx, call.Argument(%d))", b.goType(a.IDLType), i))
	}
	call := fmt.Sprintf("b.impl.%s(%s)", goName, strings.Join(args, ", "))
	if isVoidReturn(op.ReturnType) {
		b.emitCallable(op.Name, call, true, marker)
	} else {
		b.emitCallable(op.Name, fmt.Sprintf("b.ctx.vm.ToValue(%s)", call), false, marker)
	}
}

// addOverloadedOperation emits a single arg-shape dispatcher for an operation
// name that has multiple signatures (CATH-65 D6). resolveOverloads (members.go)
// is the shared source of truth for the per-overload Go method names and the
// dispatch keys, so the layer-1 methods iface.go declares and the calls emitted
// here cannot drift.
func (b *bindingBuilder) addOverloadedOperation(name string, ops []*webidl.Operation) {
	if !validGoIdentBase(opGoName(name)) {
		b.diag.Add("error", fmt.Sprintf("interface %q: operation %q sanitizes to invalid Go identifier; skipping", b.idlName, name))
		return
	}
	if b.jsSeen[name] {
		b.diag.Add("error", fmt.Sprintf("interface %q: overloaded operation %q dropped — collision (first wins)", b.idlName, name))
		return
	}
	b.jsSeen[name] = true

	sigs := resolveOverloads(name, ops, b.tm, b.diag, b.idlName)
	var arities []int
	byArity := map[int][]overloadSig{}
	for _, s := range sigs {
		b.goSeen[s.goName] = true // mirror the layer-1 methods iface.go will declare
		if _, ok := byArity[s.arity]; !ok {
			arities = append(arities, s.arity)
		}
		byArity[s.arity] = append(byArity[s.arity], s)
	}

	var body strings.Builder
	body.WriteString("switch len(call.Arguments) {\n")
	for _, a := range arities {
		fmt.Fprintf(&body, "case %d:\n", a)
		group := byArity[a]
		if len(group) == 1 {
			body.WriteString(overloadBranch(group[0]))
			continue
		}
		fmt.Fprintf(&body, "switch b.ctx.argKind(call.Argument(%d)) {\n", group[0].distinguishPos)
		for _, s := range group {
			fmt.Fprintf(&body, "case %s:\n", s.class.kindConst())
			body.WriteString(overloadBranch(s))
		}
		// Any argument kind not matched above — notably KindNull / KindUndefined —
		// routes to the last overload so a valid call still dispatches instead of
		// silently returning undefined. Precise WebIDL null/undefined coercion
		// (prefer the nullable overload) is deferred to the CATH-66 runtime.
		body.WriteString("default:\n")
		body.WriteString(overloadBranch(group[len(group)-1]))
		body.WriteString("}\n")
	}
	body.WriteString("}\nreturn goja.Undefined()")

	closure := fmt.Sprintf("func(call goja.FunctionCall) goja.Value {\n%s\n}", body.String())
	marker := noopMarker(ParseExtAttrs(ops[0].ExtAttrs, b.diag))
	b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:%s\n\t\treturn b.ctx.vm.ToValue(%s)", name, marker, closure))
	b.keyNames = append(b.keyNames, name)
}

// overloadBranch renders the coercion + dispatch (+ return) for one overload.
func overloadBranch(s overloadSig) string {
	args := make([]string, len(s.params))
	for i, p := range s.params {
		args[i] = fmt.Sprintf("coerce[%s](b.ctx, call.Argument(%d))", p.goType, i)
	}
	call := fmt.Sprintf("b.impl.%s(%s)", s.goName, strings.Join(args, ", "))
	if s.returnType == "" {
		return fmt.Sprintf("%s\nreturn goja.Undefined()\n", call)
	}
	return fmt.Sprintf("return b.ctx.vm.ToValue(%s)\n", call)
}

// addSpecialOp records a special operation (static/stringifier/getter/setter/
// deleter). The indexed getter/setter/deleter claim their Go names through the
// shared dedup so they mirror iface.go's drop decisions; static ops are skipped
// (handled by the static decls) and stringifiers surface a toString key.
func (b *bindingBuilder) addSpecialOp(op *webidl.Operation) {
	switch op.Special {
	case "stringifier":
		b.stringifier = true
	case "getter":
		if b.claimGoName(idxGetterGoName) {
			b.indexGetter = true
			b.indexGetterRet = !isVoidReturn(op.ReturnType)
		}
	case "setter":
		if b.claimGoName(idxSetterGoName) {
			b.indexSetter = true
			b.indexValType = "any"
			if len(op.Arguments) >= 2 {
				b.indexValType = b.goType(op.Arguments[1].IDLType)
			}
		}
	case "deleter":
		if b.claimGoName(idxDeleterGoName) {
			b.indexDeleter = true
		}
	}
	// "static": instance binding skips static members.
}

// addIterable routes the JS-visible iteration methods of an iterable/maplike/
// setlike declaration into the layer-1 methods iface.go emits for it. Each is an
// INJECTED method (claimInjected): if its Go name was already claimed by a
// declared member, it is dropped with a warning — exactly as iface.go does.
func (b *bindingBuilder) addIterable(it *webidl.IterableLike) {
	// Async iteration (Symbol.asyncIterator) is deferred (CATH-66+); the layer-1
	// interface still gets AsyncValues/etc., but the binding has no JS surface
	// for it yet.
	if it.Kind == webidl.IterAsyncIterable {
		return
	}
	// resolveIterMethods (members.go) is the shared source of truth for the
	// method set / readonly gating / arity; here we only render each into a Get
	// case and dispatch into the matching layer-1 method.
	for _, m := range resolveIterMethods(it, b.tm, b.diag, b.idlName) {
		b.addInjected(m.jsName, m.goName, iterCallBody(m), iterMethodVoid(m))
	}
}

// iterCallBody renders the goja call body for an iteration method, dispatching
// into the layer-1 method m.goName. The wrap shape comes from m.render (set in
// resolveIterMethods), not from sniffing the rendered Go type — so the binding
// stays decoupled from iface.go's exact type spelling.
func iterCallBody(m iterMethod) string {
	if m.render == renderForEach {
		return "b.impl.ForEach(b.ctx.callbackFn(call.Argument(0)))"
	}
	args := make([]string, len(m.params))
	for i, p := range m.params {
		args[i] = fmt.Sprintf("coerce[%s](b.ctx, call.Argument(%d))", p.goType, i)
	}
	call := fmt.Sprintf("b.impl.%s(%s)", m.goName, strings.Join(args, ", "))
	switch m.render {
	case renderSeq:
		return fmt.Sprintf("b.ctx.wrapSeq(%s)", call)
	case renderVoid:
		return call
	default: // renderScalar
		return fmt.Sprintf("b.ctx.vm.ToValue(%s)", call)
	}
}

// iterMethodVoid reports whether the rendered closure body is a statement (void)
// rather than an expression returning a goja.Value.
func iterMethodVoid(m iterMethod) bool {
	return m.render == renderForEach || m.render == renderVoid
}

// addInjected claims an injected method by (jsName, goName) and, if it survives
// the collision check, renders its Get case.
func (b *bindingBuilder) addInjected(jsName, goName, body string, void bool) {
	if !b.claimInjected(jsName, goName) {
		return
	}
	b.emitCallable(jsName, body, void, "")
}

// emitCallable renders a Get case returning a goja-callable closure. body is
// either an expression returning a goja.Value (void=false) or a statement
// (void=true), in which case the closure returns goja.Undefined(). marker is an
// optional trailing line-comment for the case (recognized no-op ext-attrs). It
// does NOT claim — the caller has already reserved the name.
func (b *bindingBuilder) emitCallable(jsName, body string, void bool, marker string) {
	var inner string
	if void {
		inner = fmt.Sprintf("\t\t%s\n\t\treturn goja.Undefined()", body)
	} else {
		inner = fmt.Sprintf("\t\treturn %s", body)
	}
	cl := fmt.Sprintf("func(call goja.FunctionCall) goja.Value {\n%s\n\t}", inner)
	b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:%s\n\t\treturn b.ctx.vm.ToValue(%s)", jsName, marker, cl))
	b.keyNames = append(b.keyNames, jsName)
}

// addConstants emits a Get case per constant the interface exposes, using the
// shared resolver so the binding references exactly the consts the layer-1 const
// block declares (same Go-name dedup + type-mappability gate).
func (b *bindingBuilder) addConstants(members []webidl.Member) {
	for _, rc := range resolveConstants(b.typeName, members, b.tm, b.diag, b.idlName) {
		if !b.claimConstKey(rc.jsName) {
			continue
		}
		b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:\n\t\treturn b.ctx.vm.ToValue(%s)", rc.jsName, rc.goName))
		b.keyNames = append(b.keyNames, rc.jsName)
	}
}

func (b *bindingBuilder) toDecl() *bindingDecl {
	if b.stringifier {
		b.addInjected("toString", "String", "b.ctx.vm.ToValue(b.impl.String())", false)
	}
	return &bindingDecl{
		typeName:    b.typeName,
		parentName:  b.parentName,
		getCases:    b.getCases,
		setCases:    b.setCases,
		keyNames:    b.keyNames,
		indexGet:    b.indexGetter,
		indexGetRet: b.indexGetterRet,
		indexSet:    b.indexSetter,
		indexDel:    b.indexDeleter,
		indexVal:    b.indexValType,
	}
}

// bindingDecl is one generated DynamicObject accessor type.
type bindingDecl struct {
	typeName    string
	parentName  string
	getCases    []string
	setCases    []string
	keyNames    []string
	indexGet    bool
	indexGetRet bool // indexed getter returns a value (wrap in ToValue) vs void
	indexSet    bool
	indexDel    bool
	indexVal    string
}

func (d *bindingDecl) declName() string { return d.typeName + "Binding" }

func (d *bindingDecl) recv() string { return "b *" + d.typeName + "Binding" }

func (d *bindingDecl) declSource() string {
	var sb strings.Builder

	// struct
	fmt.Fprintf(&sb, "type %sBinding struct {\n", d.typeName)
	sb.WriteString("\tctx *bindCtx\n")
	fmt.Fprintf(&sb, "\timpl %s\n", d.typeName)
	if d.parentName != "" {
		fmt.Fprintf(&sb, "\tparent *%sBinding\n", d.parentName)
	}
	sb.WriteString("}\n\n")

	d.writeGet(&sb)
	d.writeSet(&sb)
	d.writeHas(&sb)
	d.writeDelete(&sb)
	d.writeKeys(&sb)

	return sb.String()
}

func (d *bindingDecl) writeGet(sb *strings.Builder) {
	fmt.Fprintf(sb, "func (%s) Get(key string) goja.Value {\n", d.recv())
	if len(d.getCases) > 0 {
		sb.WriteString("\tswitch key {\n")
		sb.WriteString(strings.Join(d.getCases, "\n"))
		sb.WriteString("\n\t}\n")
	}
	if d.indexGet {
		if d.indexGetRet {
			fmt.Fprintf(sb, "\tif i, ok := asArrayIndex(key); ok {\n\t\treturn b.ctx.vm.ToValue(b.impl.%s(i))\n\t}\n", idxGetterGoName)
		} else {
			// Void/undefined indexed getter: Index has no return — call it, then
			// fall through to the no-value result rather than wrapping it.
			fmt.Fprintf(sb, "\tif i, ok := asArrayIndex(key); ok {\n\t\tb.impl.%s(i)\n\t\treturn goja.Undefined()\n\t}\n", idxGetterGoName)
		}
	}
	if d.parentName != "" {
		sb.WriteString("\treturn b.parent.Get(key)\n}\n\n")
	} else {
		sb.WriteString("\treturn goja.Undefined()\n}\n\n")
	}
}

func (d *bindingDecl) writeSet(sb *strings.Builder) {
	fmt.Fprintf(sb, "func (%s) Set(key string, val goja.Value) bool {\n", d.recv())
	if len(d.setCases) > 0 {
		sb.WriteString("\tswitch key {\n")
		sb.WriteString(strings.Join(d.setCases, "\n"))
		sb.WriteString("\n\t}\n")
	}
	if d.indexSet {
		fmt.Fprintf(sb, "\tif i, ok := asArrayIndex(key); ok {\n\t\tb.impl.%s(i, coerce[%s](b.ctx, val))\n\t\treturn true\n\t}\n", idxSetterGoName, d.indexVal)
	}
	if d.parentName != "" {
		sb.WriteString("\treturn b.parent.Set(key, val)\n}\n\n")
	} else {
		sb.WriteString("\treturn false\n}\n\n")
	}
}

func (d *bindingDecl) writeHas(sb *strings.Builder) {
	fmt.Fprintf(sb, "func (%s) Has(key string) bool {\n", d.recv())
	// Membership via equality (not case labels) so a key is never emitted twice.
	conds := make([]string, 0, len(d.keyNames))
	for _, n := range d.keyNames {
		conds = append(conds, fmt.Sprintf("key == %q", n))
	}
	if d.indexGet {
		conds = append(conds, "func() bool { _, ok := asArrayIndex(key); return ok }()")
	}
	if d.parentName != "" {
		conds = append(conds, "b.parent.Has(key)")
	}
	if len(conds) == 0 {
		sb.WriteString("\treturn false\n}\n\n")
		return
	}
	fmt.Fprintf(sb, "\treturn %s\n}\n\n", strings.Join(conds, " ||\n\t\t"))
}

func (d *bindingDecl) writeDelete(sb *strings.Builder) {
	fmt.Fprintf(sb, "func (%s) Delete(key string) bool {\n", d.recv())
	if d.indexDel {
		fmt.Fprintf(sb, "\tif i, ok := asArrayIndex(key); ok {\n\t\tb.impl.%s(i)\n\t\treturn true\n\t}\n", idxDeleterGoName)
	}
	if d.parentName != "" {
		sb.WriteString("\treturn b.parent.Delete(key)\n}\n\n")
	} else {
		sb.WriteString("\treturn false\n}\n\n")
	}
}

func (d *bindingDecl) writeKeys(sb *strings.Builder) {
	fmt.Fprintf(sb, "func (%s) Keys() []string {\n", d.recv())
	lits := make([]string, 0, len(d.keyNames))
	for _, n := range d.keyNames {
		lits = append(lits, fmt.Sprintf("%q", n))
	}
	own := "[]string{" + strings.Join(lits, ", ") + "}"
	if d.parentName != "" {
		fmt.Fprintf(sb, "\treturn append(%s, b.parent.Keys()...)\n}\n", own)
	} else {
		fmt.Fprintf(sb, "\treturn %s\n}\n", own)
	}
}

// exposedTypeNames returns the sanitized Go type names of the regular interfaces
// exposed to global, so a child binding never delegates to an unexposed (and thus
// unemitted) parent's binding type. Uses a throwaway Diagnostics — the real emit
// pass records any warnings.
func exposedTypeNames(ir *webidl.IR, global string) map[string]bool {
	out := map[string]bool{}
	for _, def := range ir.All() {
		iface, ok := def.Primary.(*webidl.Interface)
		if !ok || iface.Variant != webidl.IfaceRegular || !hasAlnum(iface.Name) {
			continue
		}
		if exposedTo(ParseExtAttrs(iface.ExtAttrs, NewDiagnostics()).ExposedScopes, global) {
			out[IdentSanitize(iface.Name)] = true
		}
	}
	return out
}

// GenerateBindings runs the binding backend over ir: for each regular interface
// it emits a DynamicObject accessor and writes them all to bindings.go in
// opts.OutputDir. It is independent of Generate (which emits the layer-1
// generated.go); the two backends do not share an output file.
func GenerateBindings(ir *webidl.IR, opts Options) error {
	if ir == nil {
		return fmt.Errorf("codegen.GenerateBindings: ir is nil")
	}
	if opts.PackageName == "" {
		return fmt.Errorf("codegen.GenerateBindings: Options.PackageName is required")
	}
	// Validate the output dir up front, before doing any rendering work.
	if fi, err := os.Stat(opts.OutputDir); err != nil {
		return fmt.Errorf("codegen.GenerateBindings: OutputDir %q: %w", opts.OutputDir, err)
	} else if !fi.IsDir() {
		return fmt.Errorf("codegen.GenerateBindings: OutputDir %q: not a directory", opts.OutputDir)
	}

	tm := typemap.Mapper{}
	diag := NewDiagnostics()
	global := opts.exposureGlobalOrDefault()
	exposed := exposedTypeNames(ir, global)

	var all []Decl
	var manifest []manifestEntry
	for _, def := range ir.All() {
		decls := newBindingDeclsFor(def, tm, diag, global, exposed)
		if len(decls) == 0 {
			continue
		}
		all = append(all, decls...)
		// Non-empty decls ⇒ def.Primary is a regular, exposed interface.
		iface := def.Primary.(*webidl.Interface)
		scopes := ParseExtAttrs(iface.ExtAttrs, diag).ExposedScopes
		manifest = append(manifest, manifestEntry{
			idlName:  iface.Name,
			typeName: IdentSanitize(iface.Name),
			globals:  manifestGlobals(scopes),
		})
	}
	if !diag.IsClean() {
		return fmt.Errorf("codegen.GenerateBindings: type-mapping errors:\n%s", diag.Format())
	}

	f := NewFile(opts.PackageName)
	// Import goja only when there is at least one binding to emit; otherwise the
	// file would carry an unused import and fail to compile.
	if len(all) > 0 {
		tr := NewImportTracker()
		tr.Add("github.com/dop251/goja")
		f.SetImports(tr)
	}
	// No DedupeDecls: binding decls are one-per-interface with no shared
	// singletons, so a repeated declName means two interfaces collided under
	// IdentSanitize — let File.Render surface that as a hard error rather than
	// silently dropping a binding.
	for _, d := range all {
		f.AddDecl(d)
	}
	// The [Exposed] registry (CATH-65 D4/D5): emitted only when there is at least
	// one exposed binding, so an empty IR carries no goja/bindCtx reference.
	if len(manifest) > 0 {
		f.AddDecl(&manifestDecl{entries: manifest})
	}

	src, err := f.Render()
	if err != nil {
		return fmt.Errorf("codegen.GenerateBindings: render: %w", err)
	}
	outPath := filepath.Join(opts.OutputDir, "bindings.go")
	if err := os.WriteFile(outPath, src, 0o644); err != nil {
		return fmt.Errorf("codegen.GenerateBindings: write %q: %w", outPath, err)
	}
	return nil
}

// manifestEntry is one exposed interface in the [Exposed] registry.
type manifestEntry struct {
	idlName  string   // JS global name (the IDL interface name)
	typeName string   // sanitized Go type name (<typeName>Binding)
	globals  []string // normalized exposure scopes (["*"] for absent/star)
}

// manifestDecl emits the exposure registry CATH-66's Register ranges over: the
// ExposedBinding type plus the ExposedBindings slice. The New factory wraps a
// layer-1 impl into its DynamicObject binding (CATH-65 D5).
type manifestDecl struct {
	entries []manifestEntry
}

func (d *manifestDecl) declName() string { return "ExposedBindings" }

func (d *manifestDecl) declSource() string {
	var sb strings.Builder
	sb.WriteString("type ExposedBinding struct {\n")
	sb.WriteString("\tName    string\n")
	sb.WriteString("\tGlobals []string\n")
	sb.WriteString("\tNew     func(ctx *bindCtx, impl any) goja.Value\n")
	sb.WriteString("}\n\n")
	sb.WriteString("var ExposedBindings = []ExposedBinding{\n")
	for _, e := range d.entries {
		globs := make([]string, len(e.globals))
		for i, g := range e.globals {
			globs[i] = fmt.Sprintf("%q", g)
		}
		fmt.Fprintf(&sb, "\t{Name: %q, Globals: []string{%s}, New: func(ctx *bindCtx, impl any) goja.Value { return ctx.vm.NewDynamicObject(&%sBinding{ctx: ctx, impl: impl.(%s)}) }},\n",
			e.idlName, strings.Join(globs, ", "), e.typeName, e.typeName)
	}
	sb.WriteString("}\n")
	return sb.String()
}
