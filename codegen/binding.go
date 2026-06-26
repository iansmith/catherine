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
// Because of these references the output is gofmt-valid but does not fully
// compile standalone until the CATH-66 shim lands. goja itself is intentionally
// NOT a dependency of this module — the generator emits source text; the
// consumer (louis14) provides goja.

// NewBindingDecls emits the DynamicObject accessor decls for the interface in
// def. Returns nil for a nil def, a non-interface primary, or a mixin/callback
// interface (only regular interfaces get a binding accessor).
func NewBindingDecls(def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics) []Decl {
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

	b := &bindingBuilder{
		typeName: IdentSanitize(iface.Name),
		idlName:  iface.Name,
		tm:       tm,
		diag:     diag,
		goSeen:   make(map[string]bool),
		jsSeen:   make(map[string]bool),
	}
	if iface.Inheritance != "" {
		b.parentName = IdentSanitize(iface.Inheritance)
	}
	for _, mem := range def.Members {
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
	if a.Special == "stringifier" {
		b.stringifier = true
	}
	if !validGoIdentBase(IdentSanitize(a.Name)) {
		b.diag.Add("error", fmt.Sprintf("interface %q: attribute %q sanitizes to invalid Go identifier; skipping", b.idlName, a.Name))
		return
	}
	getter := attrGetterName(a.Name)
	if !b.claimMethod(a.Name, getter) {
		return
	}
	b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:\n\t\treturn b.ctx.vm.ToValue(b.impl.%s())", a.Name, getter))
	b.keyNames = append(b.keyNames, a.Name)
	if a.Readonly {
		return
	}
	typeStr := b.goType(a.IDLType)
	b.setCases = append(b.setCases, fmt.Sprintf("\tcase %q:\n\t\tb.impl.%s(coerce[%s](b.ctx, val))\n\t\treturn true", a.Name, attrSetterName(a.Name), typeStr))
}

func (b *bindingBuilder) addOperation(op *webidl.Operation) {
	switch op.Special {
	case "static":
		return
	case "stringifier":
		b.stringifier = true
		return
	case "getter":
		if !b.claimGoName("Index") {
			return
		}
		b.indexGetter = true
		b.indexGetterRet = !isVoidReturn(op.ReturnType)
		return
	case "setter":
		if !b.claimGoName("SetIndex") {
			return
		}
		b.indexSetter = true
		b.indexValType = "any"
		if len(op.Arguments) >= 2 {
			b.indexValType = b.goType(op.Arguments[1].IDLType)
		}
		return
	case "deleter":
		if !b.claimGoName("Delete") {
			return
		}
		b.indexDeleter = true
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
	args := make([]string, 0, len(op.Arguments))
	for i, a := range op.Arguments {
		args = append(args, fmt.Sprintf("coerce[%s](b.ctx, call.Argument(%d))", b.goType(a.IDLType), i))
	}
	call := fmt.Sprintf("b.impl.%s(%s)", goName, strings.Join(args, ", "))
	if isVoidReturn(op.ReturnType) {
		b.emitCallable(op.Name, call, true)
	} else {
		b.emitCallable(op.Name, fmt.Sprintf("b.ctx.vm.ToValue(%s)", call), false)
	}
}

// addIterable routes the JS-visible iteration methods of an iterable/maplike/
// setlike declaration into the layer-1 methods iface.go emits for it. Each is an
// INJECTED method (claimInjected): if its Go name was already claimed by a
// declared member, it is dropped with a warning — exactly as iface.go does.
func (b *bindingBuilder) addIterable(it *webidl.IterableLike) {
	types := make([]string, 0, len(it.Types))
	for _, t := range it.Types {
		types = append(types, b.goType(t))
	}
	keyType, valType := "uint32", "any"
	if len(types) == 1 {
		valType = types[0]
	} else if len(types) >= 2 {
		keyType, valType = types[0], types[1]
	}

	switch it.Kind {
	case webidl.IterIterable:
		b.addSeqMethod("values", "Values")
		b.addSeqMethod("keys", "Keys")
		b.addSeqMethod("entries", "Entries")
		b.addInjected("forEach", "ForEach", "b.impl.ForEach(b.ctx.callbackFn(call.Argument(0)))", true)
	case webidl.IterMaplike:
		b.addInjected("get", "Get", fmt.Sprintf("b.ctx.vm.ToValue(b.impl.Get(coerce[%s](b.ctx, call.Argument(0))))", keyType), false)
		b.addInjected("has", "Has", fmt.Sprintf("b.ctx.vm.ToValue(b.impl.Has(coerce[%s](b.ctx, call.Argument(0))))", keyType), false)
		b.addSeqMethod("keys", "Keys")
		b.addSeqMethod("values", "Values")
		b.addSeqMethod("entries", "Entries")
		b.addInjected("size", "Size", "b.ctx.vm.ToValue(b.impl.Size())", false)
		if !it.Readonly {
			b.addInjected("set", "Set", fmt.Sprintf("b.impl.Set(coerce[%s](b.ctx, call.Argument(0)), coerce[%s](b.ctx, call.Argument(1)))", keyType, valType), true)
			b.addInjected("delete", "Delete", fmt.Sprintf("b.impl.Delete(coerce[%s](b.ctx, call.Argument(0)))", keyType), true)
			b.addInjected("clear", "Clear", "b.impl.Clear()", true)
		}
	case webidl.IterSetlike:
		b.addInjected("has", "Has", fmt.Sprintf("b.ctx.vm.ToValue(b.impl.Has(coerce[%s](b.ctx, call.Argument(0))))", valType), false)
		b.addSeqMethod("keys", "Keys")
		b.addSeqMethod("values", "Values")
		b.addSeqMethod("entries", "Entries")
		b.addInjected("size", "Size", "b.ctx.vm.ToValue(b.impl.Size())", false)
		if !it.Readonly {
			b.addInjected("add", "Add", fmt.Sprintf("b.impl.Add(coerce[%s](b.ctx, call.Argument(0)))", valType), true)
			b.addInjected("delete", "Delete", fmt.Sprintf("b.impl.Delete(coerce[%s](b.ctx, call.Argument(0)))", valType), true)
			b.addInjected("clear", "Clear", "b.impl.Clear()", true)
		}
		// IterAsyncIterable: JS async iteration is deferred (CATH-66+).
	}
}

// addSeqMethod adds an injected iteration method whose layer-1 form returns an
// iter.Seq, wrapped into a JS iterator by the runtime shim.
func (b *bindingBuilder) addSeqMethod(jsName, goMethod string) {
	b.addInjected(jsName, goMethod, fmt.Sprintf("b.ctx.wrapSeq(b.impl.%s())", goMethod), false)
}

// addInjected claims an injected method by (jsName, goName) and, if it survives
// the collision check, renders its Get case.
func (b *bindingBuilder) addInjected(jsName, goName, body string, void bool) {
	if !b.claimInjected(jsName, goName) {
		return
	}
	b.emitCallable(jsName, body, void)
}

// emitCallable renders a Get case returning a goja-callable closure. body is
// either an expression returning a goja.Value (void=false) or a statement
// (void=true), in which case the closure returns goja.Undefined(). It does NOT
// claim — the caller has already reserved the name.
func (b *bindingBuilder) emitCallable(jsName, body string, void bool) {
	var inner string
	if void {
		inner = fmt.Sprintf("\t\t%s\n\t\treturn goja.Undefined()", body)
	} else {
		inner = fmt.Sprintf("\t\treturn %s", body)
	}
	cl := fmt.Sprintf("func(call goja.FunctionCall) goja.Value {\n%s\n\t}", inner)
	b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:\n\t\treturn b.ctx.vm.ToValue(%s)", jsName, cl))
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
		typeName:     b.typeName,
		parentName:   b.parentName,
		getCases:     b.getCases,
		setCases:     b.setCases,
		keyNames:     b.keyNames,
		indexGet:     b.indexGetter,
		indexGetRet:  b.indexGetterRet,
		indexSet:     b.indexSetter,
		indexDel:     b.indexDeleter,
		indexVal:     b.indexValType,
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
			sb.WriteString("\tif i, ok := asArrayIndex(key); ok {\n\t\treturn b.ctx.vm.ToValue(b.impl.Index(i))\n\t}\n")
		} else {
			// Void/undefined indexed getter: Index has no return — call it, then
			// fall through to the no-value result rather than wrapping it.
			sb.WriteString("\tif i, ok := asArrayIndex(key); ok {\n\t\tb.impl.Index(i)\n\t\treturn goja.Undefined()\n\t}\n")
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
		fmt.Fprintf(sb, "\tif i, ok := asArrayIndex(key); ok {\n\t\tb.impl.SetIndex(i, coerce[%s](b.ctx, val))\n\t\treturn true\n\t}\n", d.indexVal)
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
		sb.WriteString("\tif i, ok := asArrayIndex(key); ok {\n\t\tb.impl.Delete(i)\n\t\treturn true\n\t}\n")
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

	var all []Decl
	for _, def := range ir.All() {
		all = append(all, NewBindingDecls(def, tm, diag)...)
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
