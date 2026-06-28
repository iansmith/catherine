package codegen_test

// ===========================================================================
// CATH-65: extended-attribute semantics in the binding backend (red tests)
//
// These describe the expected emitted output once the binding/iface generators
// consume ExtAttrs. They fail on current code (which ignores ExtAttrs) and turn
// green as CATH-65 is implemented. Design decisions D1–D10 are recorded in
// ~/.claude/ticket-active/CATH-65/findings.md.
//
// All assertions are on generated SOURCE (gofmt-valid, references the CATH-66
// shim) — never on compiled behavior, matching the CATH-64 contract.
// ===========================================================================

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
	"github.com/iansmith/webidl/webidl"
)

// --- local ext-attr constructors -------------------------------------------

func xa(name string) *webidl.ExtAttr { return &webidl.ExtAttr{Name: name} }

func xav(name, ident string) *webidl.ExtAttr {
	return &webidl.ExtAttr{Name: name, RHS: &webidl.ExtAttrRHS{Type: "identifier", Value: ident}}
}

func xaStar(name string) *webidl.ExtAttr {
	return &webidl.ExtAttr{Name: name, RHS: &webidl.ExtAttrRHS{Type: "*"}}
}

// ifaceExtAttrs sets the interface-level ext-attrs on a regularMergedDef.
func ifaceExtAttrs(def *webidl.MergedDef, ea ...*webidl.ExtAttr) *webidl.MergedDef {
	def.Primary.(*webidl.Interface).ExtAttrs = ea
	return def
}

func bindingSrc(t *testing.T, def *webidl.MergedDef, diag *codegen.Diagnostics) string {
	t.Helper()
	return sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, diag)), gojaPkg)
}

func ifaceSrc(t *testing.T, def *webidl.MergedDef, diag *codegen.Diagnostics) string {
	t.Helper()
	return sourceOf(t, firstDecl(t, codegen.NewInterfaceDecls(def, tm, diag)))
}

// ===========================================================================
// [Exposed] — interface filtering (AC #1)   (D4)
// ===========================================================================

// A non-Window-exposed interface gets NO binding under the default Window global.
func TestExposed_WorkerOnly_NoBinding(t *testing.T) {
	t.Parallel()
	def := ifaceExtAttrs(
		regularMergedDef("Wkr", "", attr("y", true, idlAttrType("long"))),
		xav("Exposed", "Worker"))
	if decls := codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics()); len(decls) != 0 {
		t.Errorf("[Exposed=Worker] under default Window must yield 0 binding decls, got %d", len(decls))
	}
}

func TestExposed_Window_EmitsBinding(t *testing.T) {
	t.Parallel()
	def := ifaceExtAttrs(
		regularMergedDef("Win", "", attr("x", true, idlAttrType("long"))),
		xav("Exposed", "Window"))
	if decls := codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics()); len(decls) == 0 {
		t.Error("[Exposed=Window] must still emit a binding")
	}
}

// Absent [Exposed] is lenient: a binding is still emitted (existing goldens rely
// on this — they carry no [Exposed]).
func TestExposed_Absent_Lenient_EmitsBinding(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Plain", "", attr("x", true, idlAttrType("long")))
	if decls := codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics()); len(decls) == 0 {
		t.Error("absent [Exposed] must remain lenient and emit a binding")
	}
}

func TestExposed_Star_EmitsBinding(t *testing.T) {
	t.Parallel()
	def := ifaceExtAttrs(
		regularMergedDef("Everywhere", "", attr("x", true, idlAttrType("long"))),
		xaStar("Exposed"))
	if decls := codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics()); len(decls) == 0 {
		t.Error("[Exposed=*] must emit a binding")
	}
}

// Pipeline level: the registry manifest lists only exposed interfaces, and the
// excluded interface's binding struct is absent from bindings.go entirely.
func TestExposed_Manifest_ListsExposedOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ir := mustIR(t, "[Exposed=Window] interface Win { readonly attribute long x; }; "+
		"[Exposed=Worker] interface Wkr { readonly attribute long y; };")
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}
	src := readGenerated(t, dir, "bindings.go")
	if !strings.Contains(src, "ExposedBindings") {
		t.Errorf("bindings.go must emit the ExposedBindings registry manifest\n%s", src)
	}
	if !strings.Contains(src, "type WinBinding struct") {
		t.Errorf("Window-exposed interface must be bound\n%s", src)
	}
	if strings.Contains(src, "type WkrBinding struct") {
		t.Errorf("Worker-only interface must be excluded from a Window build\n%s", src)
	}
	if !strings.Contains(src, `"Win"`) {
		t.Errorf("manifest must name the exposed interface \"Win\"\n%s", src)
	}
	if strings.Contains(src, `"Wkr"`) {
		t.Errorf("manifest must not name the excluded interface \"Wkr\"\n%s", src)
	}
}

// ===========================================================================
// [Reflect] — attribute-map reflection + layer-1 trim (AC #2)   (D1, D2, D3)
// ===========================================================================

func reflectAttr(name, typ string, ea ...*webidl.ExtAttr) *webidl.Attribute {
	a := attr(name, false, idlAttrType(typ))
	a.ExtAttrs = ea
	return a
}

// DOMString [Reflect] → binding reflects over the attribute store, NOT layer-1.
func TestReflect_String_BindingReflectsAttrMap(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "", reflectAttr("id", "DOMString", xa("Reflect")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, `b.ctx.reflectGetString(b.impl, "id")`) {
		t.Errorf("reflected getter must go through reflectGetString\n%s", src)
	}
	if !strings.Contains(src, `b.ctx.reflectSetString(b.impl, "id"`) {
		t.Errorf("reflected setter must go through reflectSetString\n%s", src)
	}
	if strings.Contains(src, "b.impl.IdAttr()") {
		t.Errorf("a reflected attr must NOT dispatch into the layer-1 getter\n%s", src)
	}
}

// The reflected attr is trimmed from the layer-1 interface (no IdAttr/SetIdAttr).
func TestReflect_String_Layer1Trimmed(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "", reflectAttr("id", "DOMString", xa("Reflect")))
	src := ifaceSrc(t, def, codegen.NewDiagnostics())
	if strings.Contains(src, "IdAttr()") || strings.Contains(src, "SetIdAttr(") {
		t.Errorf("a fully-reflected attr must be trimmed from the layer-1 interface\n%s", src)
	}
}

func TestReflect_Boolean(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "", reflectAttr("hidden", "boolean", xa("Reflect")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, `b.ctx.reflectGetBool(b.impl, "hidden")`) {
		t.Errorf("boolean [Reflect] must use reflectGetBool (presence-based)\n%s", src)
	}
	if !strings.Contains(src, `b.ctx.reflectSetBool(b.impl, "hidden"`) {
		t.Errorf("boolean [Reflect] must use reflectSetBool\n%s", src)
	}
}

func TestReflect_UnsignedLong(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "", reflectAttr("width", "unsigned long", xa("Reflect")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, `b.ctx.reflectGetUint32(b.impl, "width")`) {
		t.Errorf("unsigned long [Reflect] must use reflectGetUint32\n%s", src)
	}
	if !strings.Contains(src, `b.ctx.reflectSetUint32(b.impl, "width"`) {
		t.Errorf("unsigned long [Reflect] must use reflectSetUint32\n%s", src)
	}
}

// [Reflect=class] uses the renamed content-attribute name, not the IDL name.
func TestReflect_RenamedContentAttr(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "", reflectAttr("className", "DOMString", xav("Reflect", "class")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, `b.ctx.reflectGetString(b.impl, "class")`) {
		t.Errorf("[Reflect=class] must reflect content attribute \"class\"\n%s", src)
	}
}

// A [Reflect] on a non-reflectable type (an interface type) is NOT reflected:
// the layer-1 method is KEPT and a diagnostic is emitted (D2 — no hole).
func TestReflect_NonReflectableType_KeepsLayer1(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	def := regularMergedDef("Document", "", reflectAttr("body", "HTMLElement", xa("Reflect")))
	bsrc := bindingSrc(t, def, diag)
	if strings.Contains(bsrc, "reflectGet") {
		t.Errorf("a non-reflectable type must not emit a reflect accessor\n%s", bsrc)
	}
	if !strings.Contains(bsrc, "b.impl.BodyAttr()") {
		t.Errorf("non-reflectable [Reflect] must fall back to the layer-1 getter\n%s", bsrc)
	}
	if !strings.Contains(diag.Format(), "Reflect") {
		t.Errorf("non-reflectable [Reflect] should record a diagnostic, got: %s", diag.Format())
	}
	isrc := ifaceSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(isrc, "BodyAttr()") {
		t.Errorf("non-reflectable [Reflect] must keep the layer-1 method\n%s", isrc)
	}
}

// ===========================================================================
// [SameObject] — identity caching (AC #3)   (D7)
// ===========================================================================

func TestSameObject_CachesReadonlyObjectAttr(t *testing.T) {
	t.Parallel()
	a := attr("frames", true, idlAttrType("Window"))
	a.ExtAttrs = []*webidl.ExtAttr{xa("SameObject")}
	def := regularMergedDef("Window", "", a)
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, `b.ctx.sameObject(b.impl, "frames"`) {
		t.Errorf("[SameObject] readonly object attr must wrap the getter in sameObject\n%s", src)
	}
}

// [SameObject] on a primitive type is meaningless: no cache, and a diagnostic.
func TestSameObject_OnPrimitive_Warns_NoCache(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	a := attr("title", true, idlAttrType("DOMString"))
	a.ExtAttrs = []*webidl.ExtAttr{xa("SameObject")}
	def := regularMergedDef("Element", "", a)
	src := bindingSrc(t, def, diag)
	if strings.Contains(src, "sameObject") {
		t.Errorf("[SameObject] on a primitive attr must not emit a cache wrap\n%s", src)
	}
	if !strings.Contains(diag.Format(), "SameObject") {
		t.Errorf("[SameObject] on a primitive attr should record a diagnostic, got: %s", diag.Format())
	}
}

// ===========================================================================
// overloads — arg-count + coarse-type dispatch (AC #4)   (D6)
// ===========================================================================

// Arg-count dispatch: drawImage(3 args) vs drawImage(5 args).
func TestOverload_ArgCountDispatch_Binding(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Ctx", "",
		op("drawImage", idlType("undefined"), arg("image", "any"), arg("dx", "double"), arg("dy", "double")),
		op("drawImage", idlType("undefined"), arg("image", "any"), arg("dx", "double"), arg("dy", "double"), arg("dw", "double"), arg("dh", "double")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "switch len(call.Arguments)") {
		t.Errorf("overloaded op must dispatch on len(call.Arguments)\n%s", src)
	}
	if !strings.Contains(src, "b.impl.DrawImage3(") || !strings.Contains(src, "b.impl.DrawImage5(") {
		t.Errorf("each arity overload must call its own arity-suffixed layer-1 method\n%s", src)
	}
}

// Layer-1 emits one method per overload (no first-wins drop).
func TestOverload_ArgCount_Layer1PerOverloadMethods(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Ctx", "",
		op("drawImage", idlType("undefined"), arg("image", "any"), arg("dx", "double"), arg("dy", "double")),
		op("drawImage", idlType("undefined"), arg("image", "any"), arg("dx", "double"), arg("dy", "double"), arg("dw", "double"), arg("dh", "double")))
	src := ifaceSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "DrawImage3(") || !strings.Contains(src, "DrawImage5(") {
		t.Errorf("layer-1 must emit one arity-suffixed method per overload\n%s", src)
	}
}

// Same-arity overloads discriminated by coarse argument kind.
func TestOverload_TypeDiscrimination_SameArity(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Bag", "",
		op("add", idlType("undefined"), arg("item", "DOMString")),
		op("add", idlType("undefined"), arg("item", "Node")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "b.ctx.argKind(call.Argument(0))") {
		t.Errorf("same-arity overloads must discriminate via argKind at the distinguishing position\n%s", src)
	}
	if !strings.Contains(src, "b.impl.Add1String(") || !strings.Contains(src, "b.impl.Add1Node(") {
		t.Errorf("same-arity overloads must route to per-type layer-1 methods\n%s", src)
	}
}

// ===========================================================================
// [PutForwards] — forwarding setter   (D8)
// ===========================================================================

func TestPutForwards_EmitsForwardingSetter(t *testing.T) {
	t.Parallel()
	a := attr("location", true, idlAttrType("Location"))
	a.ExtAttrs = []*webidl.ExtAttr{xav("PutForwards", "href")}
	def := regularMergedDef("Document", "", a)
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "b.impl.LocationAttr().SetHrefAttr(") {
		t.Errorf("[PutForwards=href] must emit a setter forwarding to .SetHrefAttr\n%s", src)
	}
}

// ===========================================================================
// recognize-and-document no-op trio   (D9)
// ===========================================================================

// [CEReactions]/[NewObject] are recognized and leave a searchable marker in the
// generated source; they introduce no behavior and no error.
func TestNoOpExtAttrs_Documented(t *testing.T) {
	t.Parallel()
	o := op("appendChild", idlType("any"), arg("node", "any"))
	o.ExtAttrs = []*webidl.ExtAttr{xa("CEReactions")}
	def := regularMergedDef("Node", "", o)
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "CEReactions") {
		t.Errorf("[CEReactions] should be recognized with a searchable marker comment\n%s", src)
	}
}

// ===========================================================================
// CATH-65: adversary-gap tests (Step 0f)
// ===========================================================================

// Signed `long` is the 4th reflectable type → reflectGetInt32/reflectSetInt32.
// Doubles as the name-lowercasing case: tabIndex → content attribute "tabindex".
func TestReflect_SignedLong_MixedCaseLowercased(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "", reflectAttr("tabIndex", "long", xa("Reflect")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, `b.ctx.reflectGetInt32(b.impl, "tabindex")`) {
		t.Errorf("signed long [Reflect] must use reflectGetInt32 with the lowercased name\n%s", src)
	}
	if !strings.Contains(src, `b.ctx.reflectSetInt32(b.impl, "tabindex"`) {
		t.Errorf("signed long [Reflect] must use reflectSetInt32\n%s", src)
	}
	// The JS property key stays "tabIndex"; only the reflect shim call uses the
	// ASCII-lowercased content-attribute name.
	if strings.Contains(src, `reflectGetInt32(b.impl, "tabIndex")`) {
		t.Errorf("reflect call must use the lowercased content name, not the raw IDL name\n%s", src)
	}
}

// A readonly [Reflect] attr emits the getter only — no reflectSet*, still trimmed.
func TestReflect_Readonly_GetterOnlyNoSetter(t *testing.T) {
	t.Parallel()
	a := attr("id", true, idlAttrType("DOMString"))
	a.ExtAttrs = []*webidl.ExtAttr{xa("Reflect")}
	def := regularMergedDef("Element", "", a)
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, `b.ctx.reflectGetString(b.impl, "id")`) {
		t.Errorf("readonly [Reflect] must still emit the reflected getter\n%s", src)
	}
	if strings.Contains(src, "reflectSetString") {
		t.Errorf("readonly [Reflect] must NOT emit a setter\n%s", src)
	}
}

// Reflected attr on a child interface: trim + binding reflection + parent delegation coexist.
func TestReflect_WithParent_TrimAndDelegate(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "Node", reflectAttr("id", "DOMString", xa("Reflect")))
	bsrc := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(bsrc, `b.ctx.reflectGetString(b.impl, "id")`) {
		t.Errorf("child binding must still reflect\n%s", bsrc)
	}
	if !strings.Contains(bsrc, "parent *NodeBinding") || !strings.Contains(bsrc, "b.parent.Get(key)") {
		t.Errorf("reflect must not break parent embed/delegation\n%s", bsrc)
	}
	isrc := ifaceSrc(t, def, codegen.NewDiagnostics())
	if strings.Contains(isrc, "IdAttr()") {
		t.Errorf("reflected attr on a child must still be trimmed from layer-1\n%s", isrc)
	}
}

// The manifest carries the full {Name, Globals, New} payload, not a bare name list.
func TestExposed_Manifest_FullShape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ir := mustIR(t, "[Exposed=Window] interface Win { readonly attribute long x; };")
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}
	src := readGenerated(t, dir, "bindings.go")
	for _, want := range []string{"type ExposedBinding struct", "Globals", "func(ctx *bindCtx, impl any) goja.Value", "New:"} {
		if !strings.Contains(src, want) {
			t.Errorf("manifest must carry the full {Name, Globals, New} shape — missing %q\n%s", want, src)
		}
	}
}

// [SameObject] on a writable object attr is a spec error → no cache + diagnostic.
func TestSameObject_OnWritable_Warns_NoCache(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	a := attr("frames", false, idlAttrType("Window"))
	a.ExtAttrs = []*webidl.ExtAttr{xa("SameObject")}
	def := regularMergedDef("Window", "", a)
	src := bindingSrc(t, def, diag)
	if strings.Contains(src, "sameObject") {
		t.Errorf("[SameObject] on a writable attr must not emit a cache wrap\n%s", src)
	}
	if !strings.Contains(diag.Format(), "SameObject") {
		t.Errorf("[SameObject] on a writable attr should record a diagnostic, got: %s", diag.Format())
	}
}

// [Reflect] + [SameObject] on one attr: reflection wins (it's primitive); no Frankenstein wrap.
func TestReflect_SameObject_Combo(t *testing.T) {
	t.Parallel()
	a := attr("id", false, idlAttrType("DOMString"))
	a.ExtAttrs = []*webidl.ExtAttr{xa("Reflect"), xa("SameObject")}
	def := regularMergedDef("Element", "", a)
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, `b.ctx.reflectGetString(b.impl, "id")`) {
		t.Errorf("reflect must win over SameObject on a primitive reflected attr\n%s", src)
	}
	if strings.Contains(src, "sameObject") {
		t.Errorf("must not wrap a reflected primitive getter in sameObject\n%s", src)
	}
}

// A 0-arg overload arm: stop() vs stop(long).
func TestOverload_ZeroArgArity(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Media", "",
		op("stop", idlType("undefined")),
		op("stop", idlType("undefined"), arg("at", "long")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "b.impl.Stop0(") || !strings.Contains(src, "b.impl.Stop1(") {
		t.Errorf("0-arg arity overload must route to Stop0/Stop1\n%s", src)
	}
}

// A non-void overload's dispatched result must be wrapped in ToValue.
func TestOverload_NonVoidReturn_Wrapped(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("List", "",
		op("item", idlType("DOMString"), arg("i", "long")),
		op("item", idlType("DOMString"), arg("i", "long"), arg("j", "long")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "b.ctx.vm.ToValue(b.impl.Item1(") {
		t.Errorf("non-void overload branch must wrap its result in ToValue\n%s", src)
	}
}

// [Exposed=(Window,Worker)] list form including Window → still emitted (invariant guard
// against over-filtering when the list-form RHS path lands).
func TestExposed_ListForm_IncludesWindow_Emits(t *testing.T) {
	t.Parallel()
	def := ifaceExtAttrs(
		regularMergedDef("Both", "", attr("x", true, idlAttrType("long"))),
		&webidl.ExtAttr{Name: "Exposed", RHS: &webidl.ExtAttrRHS{
			Type:   "identifier-list",
			IsList: true,
			Items: []*webidl.ExtAttrItem{
				{Type: "identifier", Value: "Window"},
				{Type: "identifier", Value: "Worker"},
			},
		}})
	if decls := codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics()); len(decls) == 0 {
		t.Error("[Exposed=(Window,Worker)] includes Window → must still emit a binding")
	}
}

// [NewObject] and [Unscopable] are recognized and leave searchable markers.
func TestNoOp_NewObject_And_Unscopable(t *testing.T) {
	t.Parallel()
	a := attr("createNode", true, idlAttrType("Node"))
	a.ExtAttrs = []*webidl.ExtAttr{xa("NewObject")}
	o := op("query", idlType("any"), arg("sel", "DOMString"))
	o.ExtAttrs = []*webidl.ExtAttr{xa("Unscopable")}
	def := regularMergedDef("Document", "", a, o)
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "NewObject") {
		t.Errorf("[NewObject] should leave a searchable marker\n%s", src)
	}
	if !strings.Contains(src, "Unscopable") {
		t.Errorf("[Unscopable] should be recognized with a searchable marker\n%s", src)
	}
}

// A [PutForwards] attr is readonly — it must NOT also emit a direct setter.
func TestPutForwards_NoDirectSetter(t *testing.T) {
	t.Parallel()
	a := attr("location", true, idlAttrType("Location"))
	a.ExtAttrs = []*webidl.ExtAttr{xav("PutForwards", "href")}
	def := regularMergedDef("Document", "", a)
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "b.impl.LocationAttr().SetHrefAttr(") {
		t.Errorf("[PutForwards=href] must forward to .SetHrefAttr\n%s", src)
	}
	if strings.Contains(src, "b.impl.SetLocationAttr(") {
		t.Errorf("[PutForwards] attr is readonly — must not emit a direct setter\n%s", src)
	}
}

// Deferral guard: [Replaceable] must NOT emit replace-on-set in this PR (D8 defers it).
// Flip this when [Replaceable] is implemented.
func TestReplaceable_Deferred_NotEmitted(t *testing.T) {
	t.Parallel()
	a := attr("onhandler", false, idlAttrType("any"))
	a.ExtAttrs = []*webidl.ExtAttr{xa("Replaceable")}
	def := regularMergedDef("Window", "", a)
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if strings.Contains(src, "replaceProperty") {
		t.Errorf("[Replaceable] is deferred — must not emit replace-on-set yet\n%s", src)
	}
}

// ===========================================================================
// CATH-65: whole-file golden — pins exposure exclusion, the registry manifest,
// reflection (string/bool/uint32 + renamed), [SameObject], and overload dispatch
// (arg-count + type) in one bindings.go. Satisfies the AC's golden requirement.
// ===========================================================================

const cath65GoldenIDL = `
interface Node {};

[Exposed=Window]
interface Element : Node {
  [Reflect] attribute DOMString id;
  [Reflect] attribute boolean hidden;
  [Reflect=class] attribute DOMString className;
  [Reflect] attribute unsigned long tabIndex;
  [SameObject] readonly attribute Node firstChild;
  undefined append(Node node);
  undefined append(DOMString text);
  undefined resize(unsigned long w, unsigned long h);
  undefined resize(unsigned long w, unsigned long h, unsigned long d);
};

[Exposed=Worker]
interface WorkerOnly {
  readonly attribute long count;
};
`

// Regression (review F1): a [Reflect] stringifier attribute must be reflected and
// fully trimmed from BOTH backends — the binding must NOT register a toString that
// dispatches into a b.impl.String() the trimmed layer-1 interface never declares.
func TestReflect_Stringifier_NoCrossBackendStringMethod(t *testing.T) {
	t.Parallel()
	a := attr("href", false, idlAttrType("DOMString"))
	a.Special = "stringifier"
	a.ExtAttrs = []*webidl.ExtAttr{xa("Reflect")}
	def := regularMergedDef("Link", "", a)

	bsrc := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(bsrc, `b.ctx.reflectGetString(b.impl, "href")`) {
		t.Errorf("reflected stringifier attr must still reflect\n%s", bsrc)
	}
	if strings.Contains(bsrc, "b.impl.String()") || strings.Contains(bsrc, `case "toString"`) {
		t.Errorf("a reflected attr must not register a stringifier toString (layer-1 has no String())\n%s", bsrc)
	}
	isrc := ifaceSrc(t, def, codegen.NewDiagnostics())
	if strings.Contains(isrc, "String()") || strings.Contains(isrc, "HrefAttr") {
		t.Errorf("reflected stringifier attr must be fully trimmed from layer-1\n%s", isrc)
	}
}

// Regression (review F3): a same-arity type-discriminated overload must emit a
// default arm so a null/undefined argument (KindNull/KindUndefined — no case)
// still dispatches instead of silently returning undefined.
func TestOverload_TypeDiscrimination_DefaultArm(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Bag", "",
		op("add", idlType("undefined"), arg("item", "DOMString")),
		op("add", idlType("undefined"), arg("item", "Node")))
	src := bindingSrc(t, def, codegen.NewDiagnostics())
	if !strings.Contains(src, "default:") {
		t.Errorf("type-discriminated overload needs a default arm for unmatched arg kinds\n%s", src)
	}
	// The default routes to the last overload (Add1Node), so it appears in both the
	// KindObject case and the default arm.
	if strings.Count(src, "b.impl.Add1Node(") < 2 {
		t.Errorf("default arm should route to the last overload (Add1Node)\n%s", src)
	}
}

// Regression (review F2): a child exposed to the target global whose parent is NOT
// exposed must emit no parent delegation — otherwise it references a *ParentBinding
// type that GenerateBindings never emits (non-compiling output).
func TestExposed_UnexposedParent_NoDelegation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ir := mustIR(t, "[Exposed=Worker] interface Node {}; "+
		"[Exposed=Window] interface Element : Node { readonly attribute long x; };")
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}
	src := readGenerated(t, dir, "bindings.go")
	if strings.Contains(src, "NodeBinding") {
		t.Errorf("child must not reference the unexposed parent's NodeBinding\n%s", src)
	}
	if strings.Contains(src, "parent *") || strings.Contains(src, "b.parent.") {
		t.Errorf("child of an unexposed parent must emit no parent delegation\n%s", src)
	}
}

// Regression (review F4): an overload set with an optional/variadic argument warns
// that dispatch is by declared argument count only (not silently approximate).
func TestOverload_OptionalArg_Warns(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	optArg := arg("opt", "DOMString")
	optArg.Optional = true
	def := regularMergedDef("Thing", "",
		op("f", idlType("undefined"), arg("a", "long")),
		op("f", idlType("undefined"), arg("a", "long"), optArg))
	_ = bindingSrc(t, def, diag)
	if !strings.Contains(diag.Format(), "optional/variadic") {
		t.Errorf("overload with an optional arg should warn about arity-only dispatch, got: %s", diag.Format())
	}
}

func TestCATH65_Golden_FullBindings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ir := mustIR(t, cath65GoldenIDL)
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}
	assertGolden(t, "cath65_full_binding.golden", readGenerated(t, dir, "bindings.go"))
}
