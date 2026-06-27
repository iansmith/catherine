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
	if diag.IsClean() {
		t.Errorf("non-reflectable [Reflect] should record a diagnostic")
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
	if diag.IsClean() {
		t.Errorf("[SameObject] on a primitive attr should record a diagnostic")
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
