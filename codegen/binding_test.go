package codegen_test

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

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

func TestBinding_StaticMembersSkipped(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("Node", "",
		staticAttr("version", true, idlAttrType("unsigned short")),
		staticOp("create", idlType("any")))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if strings.Contains(src, `case "version"`) || strings.Contains(src, "VersionAttr") {
		t.Errorf("static attribute must not appear in the instance accessor\n---\n%s", src)
	}
	if strings.Contains(src, `case "create"`) || strings.Contains(src, "b.impl.Create") {
		t.Errorf("static operation must not appear in the instance accessor\n---\n%s", src)
	}
}
