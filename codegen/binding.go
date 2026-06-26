package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

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
		seen:     make(map[string]bool),
	}
	if iface.Inheritance != "" {
		b.parentName = IdentSanitize(iface.Inheritance)
	}
	for _, mem := range def.Members {
		b.add(mem)
	}
	return []Decl{b.toDecl()}
}

// bindingBuilder accumulates the rendered fragments of one binding type as it
// walks an interface's members. A single ordered walk feeds every DynamicObject
// method, and the shared seen set enforces first-wins so no JS key produces a
// duplicate (which would be an illegal duplicate `case` label).
type bindingBuilder struct {
	typeName   string
	parentName string // "" if the interface has no parent
	idlName    string
	tm         typemap.Mapper
	diag       *Diagnostics
	seen       map[string]bool

	getCases []string // rendered `case "x": ...` blocks for Get
	setCases []string // rendered `case "x": ...` blocks for Set
	keyNames []string // JS-visible enumerable names, in declaration order

	stringifier  bool
	indexGetter  bool
	indexSetter  bool
	indexDeleter bool
	indexValType string // V type for SetIndex coercion
}

// claim reserves a JS key (first wins). Returns false if already taken.
func (b *bindingBuilder) claim(jsName string) bool {
	if b.seen[jsName] {
		b.diag.Add("error", fmt.Sprintf("interface %q: binding member %q dropped — key collision (first wins)", b.idlName, jsName))
		return false
	}
	b.seen[jsName] = true
	return true
}

func (b *bindingBuilder) goType(t *webidl.IDLType) string {
	gt, err := b.tm.MapType(t)
	if err != nil {
		b.diag.Add("error", fmt.Sprintf("interface %q: binding cannot map type: %v", b.idlName, err))
		return "any"
	}
	return gt.String()
}

func bindIsVoid(t *webidl.IDLType) bool {
	return t == nil || t.Base == "undefined" || t.Base == "void"
}

// validIdent reports whether sanitized is a usable Go identifier base.
func validIdent(sanitized string) bool {
	r := []rune(sanitized)
	return len(r) > 0 && unicode.IsLetter(r[0])
}

func (b *bindingBuilder) add(mem webidl.Member) {
	switch m := mem.(type) {
	case *webidl.Attribute:
		b.addAttribute(m)
	case *webidl.Operation:
		b.addOperation(m)
	case *webidl.Constant:
		b.addConstant(m)
	case *webidl.IterableLike:
		b.addIterable(m)
	}
}

func (b *bindingBuilder) addAttribute(a *webidl.Attribute) {
	if a.Special == "static" {
		return
	}
	if a.Special == "stringifier" {
		b.stringifier = true
	}
	goBase := IdentSanitize(a.Name)
	if !validIdent(goBase) {
		b.diag.Add("error", fmt.Sprintf("interface %q: attribute %q sanitizes to invalid Go identifier %q; skipping", b.idlName, a.Name, goBase))
		return
	}
	if !b.claim(a.Name) {
		return
	}
	b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:\n\t\treturn b.ctx.vm.ToValue(b.impl.%sAttr())", a.Name, goBase))
	b.keyNames = append(b.keyNames, a.Name)
	if a.Readonly {
		return
	}
	typeStr := b.goType(a.IDLType)
	b.setCases = append(b.setCases, fmt.Sprintf("\tcase %q:\n\t\tb.impl.Set%sAttr(coerce[%s](b.ctx, val))\n\t\treturn true", a.Name, goBase, typeStr))
}

func (b *bindingBuilder) addOperation(op *webidl.Operation) {
	switch op.Special {
	case "static":
		return
	case "stringifier":
		b.stringifier = true
		return
	case "getter":
		b.indexGetter = true
		return
	case "setter":
		b.indexSetter = true
		b.indexValType = "any"
		if len(op.Arguments) >= 2 {
			b.indexValType = b.goType(op.Arguments[1].IDLType)
		}
		return
	case "deleter":
		b.indexDeleter = true
		return
	}
	if op.Name == "" {
		return // anonymous, non-special — nothing to dispatch
	}
	goName := IdentSanitize(op.Name)
	if !validIdent(goName) {
		b.diag.Add("error", fmt.Sprintf("interface %q: operation %q sanitizes to invalid Go identifier %q; skipping", b.idlName, op.Name, goName))
		return
	}
	args := make([]string, 0, len(op.Arguments))
	for i, a := range op.Arguments {
		args = append(args, fmt.Sprintf("coerce[%s](b.ctx, call.Argument(%d))", b.goType(a.IDLType), i))
	}
	call := fmt.Sprintf("b.impl.%s(%s)", goName, strings.Join(args, ", "))
	if bindIsVoid(op.ReturnType) {
		b.addCallable(op.Name, call, true)
	} else {
		b.addCallable(op.Name, fmt.Sprintf("b.ctx.vm.ToValue(%s)", call), false)
	}
}

func (b *bindingBuilder) addConstant(c *webidl.Constant) {
	if !b.claim(c.Name) {
		return
	}
	goConst := b.typeName + IdentSanitize(c.Name)
	b.getCases = append(b.getCases, fmt.Sprintf("\tcase %q:\n\t\treturn b.ctx.vm.ToValue(%s)", c.Name, goConst))
	b.keyNames = append(b.keyNames, c.Name)
}

// addIterable routes the JS-visible iteration methods of an iterable/maplike/
// setlike declaration into the layer-1 methods iface.go emits for it.
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
		b.addCallable("forEach", "b.impl.ForEach(b.ctx.callbackFn(call.Argument(0)))", true)
	case webidl.IterMaplike:
		b.addCallable("get", fmt.Sprintf("b.ctx.vm.ToValue(b.impl.Get(coerce[%s](b.ctx, call.Argument(0))))", keyType), false)
		b.addCallable("has", fmt.Sprintf("b.ctx.vm.ToValue(b.impl.Has(coerce[%s](b.ctx, call.Argument(0))))", keyType), false)
		b.addSeqMethod("keys", "Keys")
		b.addSeqMethod("values", "Values")
		b.addSeqMethod("entries", "Entries")
		b.addCallable("size", "b.ctx.vm.ToValue(b.impl.Size())", false)
		if !it.Readonly {
			b.addCallable("set", fmt.Sprintf("b.impl.Set(coerce[%s](b.ctx, call.Argument(0)), coerce[%s](b.ctx, call.Argument(1)))", keyType, valType), true)
			b.addCallable("delete", fmt.Sprintf("b.impl.Delete(coerce[%s](b.ctx, call.Argument(0)))", keyType), true)
			b.addCallable("clear", "b.impl.Clear()", true)
		}
	case webidl.IterSetlike:
		b.addCallable("has", fmt.Sprintf("b.ctx.vm.ToValue(b.impl.Has(coerce[%s](b.ctx, call.Argument(0))))", valType), false)
		b.addSeqMethod("keys", "Keys")
		b.addSeqMethod("values", "Values")
		b.addSeqMethod("entries", "Entries")
		b.addCallable("size", "b.ctx.vm.ToValue(b.impl.Size())", false)
		if !it.Readonly {
			b.addCallable("add", fmt.Sprintf("b.impl.Add(coerce[%s](b.ctx, call.Argument(0)))", valType), true)
			b.addCallable("delete", fmt.Sprintf("b.impl.Delete(coerce[%s](b.ctx, call.Argument(0)))", valType), true)
			b.addCallable("clear", "b.impl.Clear()", true)
		}
		// IterAsyncIterable: JS async iteration is deferred (CATH-66+).
	}
}

// addSeqMethod adds an iteration method whose layer-1 form returns an iter.Seq,
// wrapped into a JS iterator by the runtime shim.
func (b *bindingBuilder) addSeqMethod(jsName, goMethod string) {
	b.addCallable(jsName, fmt.Sprintf("b.ctx.wrapSeq(b.impl.%s())", goMethod), false)
}

// addCallable adds a Get case that returns a goja-callable closure. body is
// either an expression returning a goja.Value (void=false) or a statement
// (void=true), in which case the closure returns goja.Undefined().
func (b *bindingBuilder) addCallable(jsName, body string, void bool) {
	if !b.claim(jsName) {
		return
	}
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

func (b *bindingBuilder) toDecl() *bindingDecl {
	if b.stringifier {
		b.addCallable("toString", "b.ctx.vm.ToValue(b.impl.String())", false)
	}
	return &bindingDecl{
		typeName:   b.typeName,
		parentName: b.parentName,
		getCases:   b.getCases,
		setCases:   b.setCases,
		keyNames:   b.keyNames,
		indexGet:   b.indexGetter,
		indexSet:   b.indexSetter,
		indexDel:   b.indexDeleter,
		indexVal:   b.indexValType,
	}
}

// bindingDecl is one generated DynamicObject accessor type.
type bindingDecl struct {
	typeName   string
	parentName string
	getCases   []string
	setCases   []string
	keyNames   []string
	indexGet   bool
	indexSet   bool
	indexDel   bool
	indexVal   string
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
		sb.WriteString("\tif i, ok := asArrayIndex(key); ok {\n\t\treturn b.ctx.vm.ToValue(b.impl.Index(i))\n\t}\n")
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

	tm := typemap.Mapper{}
	diag := NewDiagnostics()
	f := NewFile(opts.PackageName)
	tr := NewImportTracker()
	tr.Add("github.com/dop251/goja")
	f.SetImports(tr)

	var all []Decl
	for _, def := range ir.All() {
		all = append(all, NewBindingDecls(def, tm, diag)...)
	}
	for _, d := range DedupeDecls(all) {
		f.AddDecl(d)
	}
	if !diag.IsClean() {
		return fmt.Errorf("codegen.GenerateBindings: type-mapping errors:\n%s", diag.Format())
	}

	src, err := f.Render()
	if err != nil {
		return fmt.Errorf("codegen.GenerateBindings: render: %w", err)
	}
	if fi, err := os.Stat(opts.OutputDir); err != nil {
		return fmt.Errorf("codegen.GenerateBindings: OutputDir %q: %w", opts.OutputDir, err)
	} else if !fi.IsDir() {
		return fmt.Errorf("codegen.GenerateBindings: OutputDir %q: not a directory", opts.OutputDir)
	}
	outPath := filepath.Join(opts.OutputDir, "bindings.go")
	if err := os.WriteFile(outPath, src, 0o644); err != nil {
		return fmt.Errorf("codegen.GenerateBindings: write %q: %w", outPath, err)
	}
	return nil
}
