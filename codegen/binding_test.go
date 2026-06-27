package codegen_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
	"github.com/iansmith/webidl/webidl"
)

// updateGolden regenerates the checked-in golden files from current generator
// output. It is a developer convenience — CI must NEVER pass it, or the golden
// stops being an independent oracle and silently rubber-stamps whatever the
// generator emits.
var updateGolden = flag.Bool("update-golden", false, "regenerate codegen golden files (never set in CI)")

// ===========================================================================
// CATH-64: goja DynamicObject binding accessor generator (red tests)
//
// These describe the expected emitted output of NewBindingDecls. They fail on
// the Phase-0 stub and turn green once the generator is implemented.
//
// Design (locked):
//   - One generated type `<Iface>Binding` per regular interface, implementing
//     goja.DynamicObject (Get/Set/Has/Delete/Keys).
//   - Struct: { ctx *bindCtx; impl <Iface>; parent *<Parent>Binding (if any) }.
//   - Inheritance via embed-and-delegate: own members handled locally, the rest
//     fall through to b.parent.
//   - readonly attr → Get case only; writable attr → Get + Set; operation →
//     Get returns a func(goja.FunctionCall) goja.Value calling impl.<Op>.
//   - Method names dispatched into match layer-1: <Name>Attr / Set<Name>Attr /
//     <Op> (IdentSanitize), so the binding calls the generated interface.
//
// The generated source references goja and the CATH-66 runtime shim, so it is
// gofmt-valid (Render succeeds) but not fully compilable standalone yet.
// ===========================================================================

const gojaPkg = "github.com/dop251/goja"

// ---------------------------------------------------------------------------
// guards
// ---------------------------------------------------------------------------

func TestNewBindingDecls_NilMergedDef(t *testing.T) {
	t.Parallel()
	if decls := codegen.NewBindingDecls(nil, tm, codegen.NewDiagnostics()); len(decls) != 0 {
		t.Errorf("NewBindingDecls(nil) = %d decls, want 0", len(decls))
	}
}

func TestNewBindingDecls_MixinSkipped(t *testing.T) {
	t.Parallel()
	if decls := codegen.NewBindingDecls(mixinMergedDef("Foo"), tm, codegen.NewDiagnostics()); len(decls) != 0 {
		t.Errorf("NewBindingDecls(mixin) = %d decls, want 0 (mixins get no binding)", len(decls))
	}
}

// ---------------------------------------------------------------------------
// struct shape + DynamicObject method set
// ---------------------------------------------------------------------------

func TestBinding_StructShape(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "", attr("nodeType", true, idlAttrType("unsigned short")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	for _, want := range []string{
		"type NodeBinding struct",
		"impl Node",
		"ctx ",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("binding struct missing %q\n---\n%s", want, src)
		}
	}
}

func TestBinding_DynamicObjectMethodSet(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "", attr("nodeType", true, idlAttrType("unsigned short")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	// The exact goja.DynamicObject interface.
	for _, want := range []string{
		"func (b *NodeBinding) Get(key string) goja.Value",
		"func (b *NodeBinding) Set(key string, val goja.Value) bool",
		"func (b *NodeBinding) Has(key string) bool",
		"func (b *NodeBinding) Delete(key string) bool",
		"func (b *NodeBinding) Keys() []string",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("binding missing DynamicObject method %q\n---\n%s", want, src)
		}
	}
}

// ---------------------------------------------------------------------------
// attribute cases
// ---------------------------------------------------------------------------

func TestBinding_ReadonlyAttribute_GetterOnly(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "", attr("nodeType", true, idlAttrType("unsigned short")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, `case "nodeType"`) {
		t.Errorf("Get is missing readonly attr case `case \"nodeType\"`\n---\n%s", src)
	}
	if !strings.Contains(src, "b.impl.NodeTypeAttr()") {
		t.Errorf("Get case must dispatch into layer-1 getter b.impl.NodeTypeAttr()\n---\n%s", src)
	}
	// A readonly attribute must NOT produce a setter dispatch.
	if strings.Contains(src, "SetNodeTypeAttr") {
		t.Errorf("readonly attr produced a setter dispatch SetNodeTypeAttr — should be getter-only\n---\n%s", src)
	}
}

func TestBinding_WritableAttribute_GetterAndSetter(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "", attr("className", false, idlAttrType("DOMString")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, "b.impl.ClassNameAttr()") {
		t.Errorf("Get must dispatch writable attr getter b.impl.ClassNameAttr()\n---\n%s", src)
	}
	if !strings.Contains(src, "b.impl.SetClassNameAttr(") {
		t.Errorf("Set must dispatch writable attr setter b.impl.SetClassNameAttr(...)\n---\n%s", src)
	}
}

// ---------------------------------------------------------------------------
// operation case
// ---------------------------------------------------------------------------

func TestBinding_Operation_ClosureCase(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "",
		op("appendChild", idlType("any"), arg("node", "any")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, `case "appendChild"`) {
		t.Errorf("Get missing operation case `case \"appendChild\"`\n---\n%s", src)
	}
	if !strings.Contains(src, "func(call goja.FunctionCall) goja.Value") {
		t.Errorf("operation must emit a goja callable closure\n---\n%s", src)
	}
	if !strings.Contains(src, "b.impl.AppendChild") {
		t.Errorf("operation closure must call layer-1 method b.impl.AppendChild\n---\n%s", src)
	}
}

// ---------------------------------------------------------------------------
// Keys()
// ---------------------------------------------------------------------------

func TestBinding_KeysListsOwnMembers(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "",
		attr("nodeType", true, idlAttrType("unsigned short")),
		op("appendChild", idlType("any"), arg("node", "any")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// Keys must enumerate the JS-visible member names.
	for _, want := range []string{`"nodeType"`, `"appendChild"`} {
		if !strings.Contains(src, want) {
			t.Errorf("Keys()/source missing member name %s\n---\n%s", want, src)
		}
	}
}

// ---------------------------------------------------------------------------
// inheritance: embed parent binding + delegate
// ---------------------------------------------------------------------------

func TestBinding_Inheritance_EmbedAndDelegate(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "Node", attr("className", false, idlAttrType("DOMString")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, "parent *NodeBinding") {
		t.Errorf("inheriting binding must hold a parent *NodeBinding field\n---\n%s", src)
	}
	if !strings.Contains(src, "b.parent.Get(key)") {
		t.Errorf("Get must delegate unknown keys to b.parent.Get(key)\n---\n%s", src)
	}
	if !strings.Contains(src, "b.parent.Keys()") {
		t.Errorf("Keys must include inherited names via b.parent.Keys()\n---\n%s", src)
	}
}

func TestBinding_NoParent_DoesNotDelegate(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "", attr("nodeType", true, idlAttrType("unsigned short")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if strings.Contains(src, "b.parent") {
		t.Errorf("root interface (no parent) must not reference b.parent\n---\n%s", src)
	}
	if !strings.Contains(src, "goja.Undefined()") {
		t.Errorf("root Get must return goja.Undefined() for unknown keys\n---\n%s", src)
	}
}

// ---------------------------------------------------------------------------
// static members are skipped (mirrors layer-1)
// ---------------------------------------------------------------------------

// Anchored with a real instance member so the "must not contain" checks aren't
// vacuously true on an empty/stub accessor.
func TestBinding_StaticMembersSkipped(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "",
		attr("nodeType", true, idlAttrType("unsigned short")), // instance anchor
		staticAttr("version", true, idlAttrType("unsigned short")),
		staticOp("create", idlType("any")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// Positive anchor: the instance member MUST be generated.
	if !strings.Contains(src, `case "nodeType"`) {
		t.Fatalf("instance attr anchor `case \"nodeType\"` missing — accessor not generated\n---\n%s", src)
	}
	if strings.Contains(src, `case "version"`) || strings.Contains(src, "VersionAttr") {
		t.Errorf("static attribute must not appear in the instance accessor\n---\n%s", src)
	}
	if strings.Contains(src, `case "create"`) || strings.Contains(src, "b.impl.Create") {
		t.Errorf("static operation must not appear in the instance accessor\n---\n%s", src)
	}
}

// ===========================================================================
// CATH-64: adversary gap tests
// ===========================================================================

// --- additional guards ------------------------------------------------------

func TestNewBindingDecls_CallbackSkipped(t *testing.T) {
	t.Parallel()
	def := callbackMergedDef("EventListener", op("handleEvent", idlType("undefined"), arg("event", "any")))
	if decls := codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics()); len(decls) != 0 {
		t.Errorf("NewBindingDecls(callback interface) = %d decls, want 0", len(decls))
	}
}

func TestNewBindingDecls_NonInterfacePrimary(t *testing.T) {
	t.Parallel()
	def := &webidl.MergedDef{Primary: &webidl.Dictionary{Name: "Options"}}
	if decls := codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics()); len(decls) != 0 {
		t.Errorf("NewBindingDecls(dictionary primary) = %d decls, want 0", len(decls))
	}
}

// --- empty interface boundary ----------------------------------------------

func TestBinding_EmptyInterface_StillEmitsMethodSet(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("EventTarget", "")
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	for _, want := range []string{
		"type EventTargetBinding struct",
		"func (b *EventTargetBinding) Get(key string) goja.Value",
		"func (b *EventTargetBinding) Keys() []string",
		"goja.Undefined()", // empty switch falls straight through
	} {
		if !strings.Contains(src, want) {
			t.Errorf("empty-interface binding missing %q\n---\n%s", want, src)
		}
	}
	// Keys() must return a non-nil empty slice, not `return nil`, so JS sees [].
	if strings.Contains(src, "return nil") {
		t.Errorf("empty-interface Keys() should return an empty slice literal, not nil\n---\n%s", src)
	}
}

// --- mixed members coexist in one accessor (headline) -----------------------

func TestBinding_MixedMembers_Coexist(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "Node",
		attr("id", true, idlAttrType("DOMString")),
		attr("className", false, idlAttrType("DOMString")),
		op("getAttribute", idlType("DOMString"), arg("name", "DOMString")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// All three own members must appear together in Get.
	for _, want := range []string{
		`case "id"`, "b.impl.IdAttr()",
		`case "className"`, "b.impl.ClassNameAttr()",
		`case "getAttribute"`, "b.impl.GetAttribute",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("mixed Get missing %q\n---\n%s", want, src)
		}
	}
	// Writable setter present; readonly attr gets NO setter.
	if !strings.Contains(src, "b.impl.SetClassNameAttr(") {
		t.Errorf("writable attr missing setter dispatch\n---\n%s", src)
	}
	if strings.Contains(src, "SetIdAttr") {
		t.Errorf("readonly attr `id` must not get a setter dispatch\n---\n%s", src)
	}
	// Inheritance wired alongside own members.
	if !strings.Contains(src, "parent *NodeBinding") || !strings.Contains(src, "b.parent.Keys()") {
		t.Errorf("mixed accessor lost inheritance wiring\n---\n%s", src)
	}
}

// --- operation closure must coerce args and wrap the return -----------------

func TestBinding_Operation_CoercesArgsAndWrapsReturn(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "",
		op("appendChild", idlType("any"), arg("node", "any")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// The closure must actually read its argument from the call...
	if !strings.Contains(src, "call.Argument(0)") {
		t.Errorf("operation closure ignores its argument (no call.Argument(0))\n---\n%s", src)
	}
	// ...and wrap the Go return back into a goja.Value.
	if !strings.Contains(src, "b.ctx.vm.ToValue(") {
		t.Errorf("operation closure must wrap its return via b.ctx.vm.ToValue(...)\n---\n%s", src)
	}
}

// --- name collision: a single case label (else duplicate-case = build error) -

func TestBinding_NameCollision_FirstWins(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Foo", "",
		attr("thing", true, idlAttrType("DOMString")),
		op("thing", idlType("any")))
	diag := codegen.NewDiagnostics()
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, diag)), gojaPkg)

	if n := strings.Count(src, `case "thing"`); n != 1 {
		t.Errorf("collision must yield exactly one `case \"thing\"`, got %d (duplicate case = compile error)\n---\n%s", n, src)
	}
}

// --- Has / Delete / Set bodies (not just signatures) ------------------------

func TestBinding_Has_DelegatesToParent(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "Node", attr("className", false, idlAttrType("DOMString")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// Has must recognise an own member and fall through to the parent.
	if !strings.Contains(src, `"className"`) {
		t.Errorf("Has/source must reference own member name\n---\n%s", src)
	}
	if !strings.Contains(src, "b.parent.Has(key)") {
		t.Errorf("Has must delegate unknown keys to b.parent.Has(key)\n---\n%s", src)
	}
}

func TestBinding_Delete_RootReturnsFalse(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "", attr("nodeType", true, idlAttrType("unsigned short")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// A plain accessor cannot delete native props: Delete returns false.
	deleteBody := sliceBetween(src, "func (b *NodeBinding) Delete(key string) bool", "\n}")
	if !strings.Contains(deleteBody, "return false") {
		t.Errorf("root Delete must return false\n---\n%s", src)
	}
}

func TestBinding_Set_UnknownKeyReturnsFalse(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "", attr("nodeType", true, idlAttrType("unsigned short")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	setBody := sliceBetween(src, "func (b *NodeBinding) Set(key string, val goja.Value) bool", "\nfunc ")
	// readonly attr => no setter dispatch, and Set falls through to false.
	if strings.Contains(setBody, "nodeType") {
		t.Errorf("readonly attr must not appear in Set\n---\n%s", setBody)
	}
	if !strings.Contains(setBody, "return false") {
		t.Errorf("Set must return false for unknown/readonly keys\n---\n%s", setBody)
	}
}

// ===========================================================================
// CATH-64: broader-scope member kinds (special ops, stringifier, constants,
// iterables) — per the chosen first-cut scope.
// ===========================================================================

func TestBinding_SpecialIndexedOps(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("NodeList", "",
		specialOp("getter", idlType("any"), arg("index", "unsigned long")),
		specialOp("setter", idlType("undefined"), arg("index", "unsigned long"), arg("value", "any")),
		specialOp("deleter", idlType("undefined"), arg("index", "unsigned long")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// Indexed access routes into the layer-1 Index/SetIndex/Delete(uint32).
	if !strings.Contains(src, "b.impl.Index(") {
		t.Errorf("indexed getter must dispatch b.impl.Index(...)\n---\n%s", src)
	}
	if !strings.Contains(src, "b.impl.SetIndex(") {
		t.Errorf("indexed setter must dispatch b.impl.SetIndex(...)\n---\n%s", src)
	}
	// The deleter routes the uint32 Delete; a string `index` key must not appear.
	if strings.Contains(src, `case "index"`) {
		t.Errorf("indexed special ops must not surface a string `case \"index\"`\n---\n%s", src)
	}
}

func TestBinding_Stringifier_ToString(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("URL", "",
		&webidl.Attribute{Name: "href", IDLType: idlAttrType("DOMString"), Special: "stringifier"})
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, `case "toString"`) {
		t.Errorf("stringifier must expose a `toString` key\n---\n%s", src)
	}
	if !strings.Contains(src, "b.impl.String()") {
		t.Errorf("toString must dispatch b.impl.String()\n---\n%s", src)
	}
}

func TestBinding_Constants_ExposedAsKeys(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "",
		constMember("ELEMENT_NODE", idlConstType("unsigned short"), "1"))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, `case "ELEMENT_NODE"`) {
		t.Errorf("constant must be exposed as a readable key\n---\n%s", src)
	}
	// Dispatches the layer-1 const name: typeName + IdentSanitize(constName).
	if !strings.Contains(src, "NodeELEMENTNODE") {
		t.Errorf("constant case must reference the generated const NodeELEMENTNODE\n---\n%s", src)
	}
}

func TestBinding_Iterable_RoutesMethods(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("NodeList", "", iterable("any"))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// JS-visible iteration methods route into the layer-1 iterable methods.
	for _, want := range []string{
		`case "values"`, "b.impl.Values()",
		`case "entries"`, "b.impl.Entries()",
		`case "forEach"`, "b.impl.ForEach",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("iterable routing missing %q\n---\n%s", want, src)
		}
	}
}

// ===========================================================================
// CATH-64: code-review fixes — the binding must mirror the layer-1 generator's
// member naming + drop decisions (shared source of truth), not re-derive them.
// ===========================================================================

// F1: an injected iteration method (maplike get/has/...) colliding with a
// same-named declared operation is benign (distinct Go names in layer-1) and
// must NOT record an error that aborts the whole GenerateBindings run.
func TestBinding_MaplikeMethodCollidesWithOp_NonFatal(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Headers", "",
		op("get", idlType("DOMString"), arg("name", "DOMString")),
		maplike("DOMString", "DOMString", false))
	diag := codegen.NewDiagnostics()
	decls := codegen.NewBindingDecls(def, tm, diag)
	if !diag.IsClean() {
		t.Errorf("benign maplike/op name collision must be a warning, not an error:\n%s", diag.Format())
	}
	src := sourceOf(t, firstDecl(t, decls), gojaPkg)
	if n := strings.Count(src, `case "get"`); n != 1 {
		t.Errorf("want exactly one `case \"get\"` (declared op wins), got %d\n%s", n, src)
	}
}

// F2: an operation and an iterable method whose JS names differ but sanitize to
// the SAME Go name (op `for_each` → ForEach ; iterable `forEach` → ForEach)
// collide in layer-1 (Go-name dedup drops the iterable's). The binding must
// make the same drop — not emit the iterable's forEach case (callbackFn form)
// dispatching into the op's ForEach with mismatched args.
func TestBinding_GoNameCollision_MirrorsLayer1Drop(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Thing", "",
		op("for_each", idlType("undefined"), arg("x", "any")),
		iterable("any"))
	diag := codegen.NewDiagnostics()
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, diag)), gojaPkg)
	if strings.Contains(src, "callbackFn") {
		t.Errorf("iterable forEach must be dropped — Go name ForEach already claimed by op for_each\n%s", src)
	}
	if !strings.Contains(src, `case "for_each"`) {
		t.Errorf("declared op for_each should still be emitted\n%s", src)
	}
	if !diag.IsClean() {
		t.Errorf("the dropped injected method must be a warning, not an error:\n%s", diag.Format())
	}
}

// F3: constants must pass through the same Go-name dedup the layer-1 const block
// applies. `a-b` and `a_b` both sanitize to Go const NodeAB; layer-1 drops the
// second, so the binding must reference NodeAB exactly once (not emit a case for
// a const the layer-1 backend never declared).
func TestBinding_Constants_GoNameCollision_MirrorsLayer1(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "",
		constMember("a-b", idlConstType("unsigned short"), "1"),
		constMember("a_b", idlConstType("unsigned short"), "2"))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	if n := strings.Count(src, "NodeAB"); n != 1 {
		t.Errorf("Go-name-colliding consts: want one NodeAB reference (layer-1 drops the second), got %d\n%s", n, src)
	}
}

// F4: a void/undefined-returning indexed getter produces a layer-1 `Index(uint32)`
// with NO return; the binding must not wrap that no-value call in ToValue.
func TestBinding_VoidIndexGetter_NoToValueWrap(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Sink", "",
		specialOp("getter", idlType("undefined"), arg("index", "unsigned long")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	if strings.Contains(src, "ToValue(b.impl.Index(i))") {
		t.Errorf("void index getter must not wrap b.impl.Index(i) in ToValue (Index has no return)\n%s", src)
	}
	if !strings.Contains(src, "b.impl.Index(i)") {
		t.Errorf("index getter must still dispatch b.impl.Index(i)\n%s", src)
	}
}

// ===========================================================================
// CATH-64: code-review pass 2 fixes
// ===========================================================================

// Special index ops must participate in the same Go-name dedup as named members
// (mirroring iface.go's addSpecialMethod, which claims Index/SetIndex/Delete).
// op `delete(DOMString)` (Go name Delete) declared before a deleter (also Delete)
// → layer-1 keeps one Delete; the binding must drop the deleter, not emit a
// Delete(uint32) index branch the interface never generated.
func TestBinding_NamedOpVsDeleter_NoDoubleDelete(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Bag", "",
		op("delete", idlType("undefined"), arg("key", "DOMString")),
		specialOp("deleter", idlType("undefined"), arg("key", "DOMString")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	if !strings.Contains(src, `case "delete"`) {
		t.Errorf("named op `delete` (declared first) should win\n%s", src)
	}
	if strings.Contains(src, "b.impl.Delete(i)") {
		t.Errorf("deleter must be dropped — op `delete` already claimed Go name Delete; "+
			"emitting b.impl.Delete(i) (uint32) dispatches into a method the interface lacks\n%s", src)
	}
}

// Symmetric ordering: a deleter declared before a named `delete` op → the index
// deleter wins, the named op is dropped (no string case dispatching Delete).
func TestBinding_DeleterVsNamedOp_NoDoubleDelete(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Bag", "",
		specialOp("deleter", idlType("undefined"), arg("key", "DOMString")),
		op("delete", idlType("undefined"), arg("key", "DOMString")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	if !strings.Contains(src, "b.impl.Delete(i)") {
		t.Errorf("deleter (declared first) should win → index Delete branch present\n%s", src)
	}
	if strings.Contains(src, `case "delete"`) {
		t.Errorf("named op `delete` must be dropped — deleter already claimed Go name Delete\n%s", src)
	}
}

// --- GenerateBindings pipeline (previously untested) ------------------------

func readGenerated(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestGenerateBindings_WritesBindingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ir := mustIR(t, "interface Node { readonly attribute unsigned short nodeType; };")
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}
	src := readGenerated(t, dir, "bindings.go")
	for _, want := range []string{"package gen", "github.com/dop251/goja", "type NodeBinding struct", "func (b *NodeBinding) Get(key string) goja.Value"} {
		if !strings.Contains(src, want) {
			t.Errorf("bindings.go missing %q\n%s", want, src)
		}
	}
}

// An IR with no regular interfaces must not emit an unused goja import (which
// would make bindings.go fail to compile).
func TestGenerateBindings_EmptyIR_NoUnusedImport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ir := mustIR(t, "callback interface CB { undefined handleEvent(); };")
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}
	src := readGenerated(t, dir, "bindings.go")
	if strings.Contains(src, "dop251/goja") {
		t.Errorf("no regular interfaces → must not import goja (unused import won't compile)\n%s", src)
	}
}

// Two interfaces whose names collide under IdentSanitize must fail loudly, not
// silently drop a binding.
func TestGenerateBindings_NameCollision_FailsLoud(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ir := mustIR(t, "interface foo {}; interface Foo {};")
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err == nil {
		t.Error("two interfaces sanitizing to the same Go name must error, not silently drop one binding")
	}
}

// --- broader-scope coverage the prior pass flagged as missing ---------------

func TestBinding_Setlike_RoutesMethods(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Tags", "", setlike("DOMString", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	for _, want := range []string{
		`case "has"`, "b.impl.Has(",
		`case "values"`, "b.impl.Values()",
		`case "add"`, "b.impl.Add(",
		`case "clear"`, "b.impl.Clear()",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("setlike routing missing %q\n%s", want, src)
		}
	}
}

func TestBinding_ReadonlySetlike_NoMutators(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Tags", "", setlike("DOMString", true))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	for _, mutator := range []string{`case "add"`, `case "delete"`, `case "clear"`} {
		if strings.Contains(src, mutator) {
			t.Errorf("readonly setlike must not expose %q\n%s", mutator, src)
		}
	}
}

// All member kinds coexisting in one interface (the real coexistence case).
func TestBinding_AllKinds_Coexist(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Mega", "Node",
		attr("id", true, idlAttrType("DOMString")),
		attr("name", false, idlAttrType("DOMString")),
		op("compute", idlType("DOMString"), arg("x", "DOMString")),
		constMember("MAX", idlConstType("unsigned short"), "9"),
		&webidl.Attribute{Name: "label", IDLType: idlAttrType("DOMString"), Special: "stringifier"},
		iterable("any"))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	for _, want := range []string{
		`case "id"`, "b.impl.IdAttr()",
		`case "name"`, "b.impl.SetNameAttr(",
		`case "compute"`, "b.impl.Compute",
		`case "MAX"`, "MegaMAX",
		`case "toString"`, "b.impl.String()",
		`case "values"`, "b.impl.Values()",
		"parent *NodeBinding", "b.parent.Keys()",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("all-kinds coexistence missing %q\n%s", want, src)
		}
	}
}

// --- iteration coverage (refactor-safety for resolveIterMethods) ------------

func TestBinding_Maplike_RoutesMethods(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Map1", "", maplike("DOMString", "long", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	for _, want := range []string{
		`case "get"`, `case "has"`, `case "keys"`, `case "values"`,
		`case "entries"`, `case "size"`, `case "set"`, `case "delete"`, `case "clear"`,
		// wrap shapes — assert each render branch, not just method presence:
		"b.ctx.wrapSeq(b.impl.Values())",         // renderSeq
		"b.ctx.vm.ToValue(b.impl.Get(",           // renderScalar
		"b.ctx.vm.ToValue(b.impl.Size())",        // renderScalar, no args
	} {
		if !strings.Contains(src, want) {
			t.Errorf("maplike routing missing %q\n%s", want, src)
		}
	}
	// renderVoid: the mutator must NOT be ToValue-wrapped (it's a statement
	// returning goja.Undefined()).
	if strings.Contains(src, "ToValue(b.impl.Set(") || !strings.Contains(src, "b.impl.Set(") {
		t.Errorf("maplike `set` must render as a void mutator, not a ToValue-wrapped expression\n%s", src)
	}
}

func TestBinding_ReadonlyMaplike_NoMutators(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Map1", "", maplike("DOMString", "long", true))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	if !strings.Contains(src, `case "get"`) {
		t.Errorf("readonly maplike must still expose readers\n%s", src)
	}
	for _, mutator := range []string{`case "set"`, `case "delete"`, `case "clear"`} {
		if strings.Contains(src, mutator) {
			t.Errorf("readonly maplike must not expose %q\n%s", mutator, src)
		}
	}
}

// Async iteration is deferred (CATH-66+): an async iterable produces a valid
// binding with no iteration cases, no panic, no spurious AsyncValues dispatch.
func TestBinding_AsyncIterable_Skipped(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Stream", "", asyncIterable("any"))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	if strings.Contains(src, "AsyncValues") || strings.Contains(src, `case "values"`) {
		t.Errorf("async iterable must be skipped by the binding (deferred)\n%s", src)
	}
	if !strings.Contains(src, "func (b *StreamBinding) Get(key string) goja.Value") {
		t.Errorf("async-iterable interface still needs the DynamicObject method set\n%s", src)
	}
}

// A setter with fewer than 2 args falls back to coerce[any] for the value.
func TestBinding_IndexedSetter_OneArg_DefaultsAny(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Arr", "",
		specialOp("setter", idlType("undefined"), arg("index", "unsigned long")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	if !strings.Contains(src, "b.impl.SetIndex(i, coerce[any](") {
		t.Errorf("1-arg indexed setter must default the value coercion to any\n%s", src)
	}
}

// --- golden-file snapshot (explicit acceptance criterion) -------------------

func TestBinding_Golden_Element(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Element", "Node",
		attr("id", true, idlAttrType("DOMString")),
		attr("className", false, idlAttrType("DOMString")),
		op("getAttribute", idlType("DOMString"), arg("name", "DOMString")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	assertGolden(t, "element_binding.golden", src)
}

// Golden for a non-readonly maplike — pins the full iteration rendering (the
// three wrap shapes: wrapSeq for keys/values/entries, ToValue for get/has/size,
// void mutators for set/delete/clear), which the Element golden does not cover.
func TestBinding_Golden_Maplike(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("StringMap", "", maplike("DOMString", "DOMString", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)
	assertGolden(t, "stringmap_binding.golden", src)
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

// assertGolden compares got against testdata/<name>, or rewrites it with
// -update-golden. A missing golden file is a failure (Phase-0 RED until the
// generator is implemented and the golden is captured).
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s missing (run with -update-golden after implementing): %v", path, err)
	}
	if string(want) != got {
		t.Errorf("golden %s mismatch\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// sliceBetween returns the substring of s from the first occurrence of start up
// to the next occurrence of end after it (or end-of-string). Returns "" if
// start is absent.
func sliceBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i:]
	if j := strings.Index(rest[len(start):], end); j >= 0 {
		return rest[:len(start)+j]
	}
	return rest
}
