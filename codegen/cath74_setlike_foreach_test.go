package codegen_test

// CATH-74 Phase 0: red tests — resolveIterMethods must emit a forEach entry for
// setlike interfaces (WebIDL §3.6.10), rendered as a typed adapter closure via
// the CATH-71 renderForEach path (not a raw goja.Callable).
//
// Setlike's wrinkle vs maplike (CATH-73): §3.6.10 invokes the callback as
// callback(value, value, set) — key === value — so BOTH cbArgs use valType.
//
// These describe the expected post-fix behavior. They fail on current code
// because the IterSetlike branch of resolveIterMethods omits forEach entirely,
// so neither the layer-1 interface nor the binding emit it.

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// TestCATH74_Setlike_ForEach_BindingTypedAdapter asserts the setlike binding
// dispatches a "forEach" case whose body is a typed adapter closure, not a bare
// goja.Callable handoff.
func TestCATH74_Setlike_ForEach_BindingTypedAdapter(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("NodeSet", "", setlike("Node", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, `case "forEach"`) {
		t.Errorf("setlike binding must dispatch a \"forEach\" case (WebIDL §3.6.10)\n%s", src)
	}
	if !strings.Contains(src, "b.impl.ForEach(func(") {
		t.Errorf("setlike forEach must receive a typed adapter closure starting with func(\n%s", src)
	}
	if strings.Contains(src, "ForEach(b.ctx.Callback(") {
		t.Errorf("setlike forEach must not receive a raw goja.Callable\n%s", src)
	}
	if !strings.Contains(src, "b.ctx.WrapAny(") {
		t.Errorf("adapter closure must wrap the object value arg via WrapAny\n%s", src)
	}
}

// TestCATH74_Setlike_ForEach_CallbackShape_ValueValueSet pins the WebIDL
// §3.6.10 invocation shape: callback(value, value, set). For setlike<Node>
// (Node→any) the ordered fragment is
// `b.ctx.WrapAny(_v), b.ctx.WrapAny(_k), call.This` — proving both value slots
// are passed (value===key) AND the set (call.This) is the third callback arg,
// all inside the adapter (not merely somewhere in the file, where has/entries
// also emit WrapAny).
func TestCATH74_Setlike_ForEach_CallbackShape_ValueValueSet(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("NodeSet", "", setlike("Node", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	const want = "b.ctx.WrapAny(_v), b.ctx.WrapAny(_k), call.This"
	if !strings.Contains(src, want) {
		t.Errorf("setlike forEach must invoke callback(value, value, set) — expected ordered fragment %q\n%s", want, src)
	}
}

// TestCATH74_Setlike_Layer1_ForEach_HasTypedFnParam asserts the layer-1
// interface declares ForEach with the func(value, value) typed param, not just
// some `ForEach(` substring. Anti-drift: the binding closure func(any, any)
// only compiles against a matching layer-1 signature.
func TestCATH74_Setlike_Layer1_ForEach_HasTypedFnParam(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("NodeSet", "", setlike("Node", false))
	src := sourceOf(t, firstDecl(t, codegen.NewInterfaceDecls(def, tm, codegen.NewDiagnostics())), "iter")

	const want = "ForEach(Fn func(any, any))"
	if !strings.Contains(src, want) {
		t.Errorf("layer-1 setlike must declare %q (both slots are the value type)\n%s", want, src)
	}
}

// TestCATH74_Setlike_ForEach_PrimitiveValue_UsesToValue flips the wrap branch:
// setlike<long> has a primitive value, so both adapter args must wrap via
// VM().ToValue, proving the wrap is type-driven, not hardcoded to WrapAny.
func TestCATH74_Setlike_ForEach_PrimitiveValue_UsesToValue(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("LongSet", "", setlike("long", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	const want = "b.ctx.VM().ToValue(_v), b.ctx.VM().ToValue(_k), call.This"
	if !strings.Contains(src, want) {
		t.Errorf("setlike<long> forEach must wrap the primitive value via ToValue in both slots — expected %q\n%s", want, src)
	}
}

// TestCATH74_ReadonlySetlike_StillHasForEach asserts forEach is a reader,
// present even on readonly setlike (WebIDL exposes forEach regardless).
func TestCATH74_ReadonlySetlike_StillHasForEach(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("RoSet", "", setlike("Node", true))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, `case "forEach"`) {
		t.Errorf("readonly setlike must still expose forEach (it is a reader, not a mutator)\n%s", src)
	}
	if strings.Contains(src, `case "add"`) {
		t.Errorf("readonly setlike must not expose add\n%s", src)
	}
}
