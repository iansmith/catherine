package codegen_test

// CATH-73 Phase 0: red tests — resolveIterMethods must emit a forEach entry for
// maplike interfaces (WebIDL §3.6.9), rendered as a typed adapter closure via the
// CATH-71 renderForEach path (not a raw goja.Callable).
//
// These describe the expected post-fix behavior. They fail on current code because
// the IterMaplike branch of resolveIterMethods (codegen/members.go) omits forEach
// entirely — so neither the layer-1 interface nor the binding emit it.

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// TestCATH73_Maplike_ForEach_BindingTypedAdapter asserts the maplike binding
// dispatches a "forEach" case whose body is a typed adapter closure, mirroring
// the iterable forEach fix from CATH-71 — not a bare goja.Callable handoff.
func TestCATH73_Maplike_ForEach_BindingTypedAdapter(t *testing.T) {
	t.Parallel()
	// DOMString→string key, Node→any value.
	def := regularMergedDef("StringNodeMap", "", maplike("DOMString", "Node", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, `case "forEach"`) {
		t.Errorf("maplike binding must dispatch a \"forEach\" case (WebIDL §3.6.9)\n%s", src)
	}
	// The fix emits a typed closure literal as the ForEach argument.
	if !strings.Contains(src, "b.impl.ForEach(func(") {
		t.Errorf("maplike forEach must receive a typed adapter closure starting with func(\n%s", src)
	}
	// The broken shape would pass Callback(...) directly to ForEach.
	if strings.Contains(src, "ForEach(b.ctx.Callback(") {
		t.Errorf("maplike forEach must not receive a raw goja.Callable\n%s", src)
	}
	// The adapter must wrap the object/any value via WrapAny and the string key
	// via VM().ToValue.
	if !strings.Contains(src, "b.ctx.WrapAny(") {
		t.Errorf("adapter closure must wrap the value arg via WrapAny for object/any types\n%s", src)
	}
	if !strings.Contains(src, "b.ctx.VM().ToValue(") {
		t.Errorf("adapter closure must wrap the string key arg via VM().ToValue\n%s", src)
	}
}

// TestCATH73_Maplike_ForEach_CallbackArgOrder_ValueBeforeKey pins the WebIDL
// §3.6.9 invocation shape: the callback is called `callback(value, key, map)`.
// For DOMString→string key, Node→any value the ordered fragment is
// `b.ctx.WrapAny(_v), b.ctx.VM().ToValue(_k), call.This`. This is the one
// assertion that distinguishes a correct value-first emission from a key-first
// one AND proves the wraps live inside the adapter (not merely somewhere in the
// file, where get/has/entries also emit them) AND that the map (call.This) is
// passed as the third callback arg.
func TestCATH73_Maplike_ForEach_CallbackArgOrder_ValueBeforeKey(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("StringNodeMap", "", maplike("DOMString", "Node", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// value (WrapAny) before key (ToValue), then the map as call.This — all in
	// one ordered invocation.
	const want = "b.ctx.WrapAny(_v), b.ctx.VM().ToValue(_k), call.This"
	if !strings.Contains(src, want) {
		t.Errorf("maplike forEach must invoke callback(value, key, map) — expected ordered fragment %q\n%s", want, src)
	}
}

// TestCATH73_Maplike_Layer1_ForEach_HasTypedFnParam asserts the layer-1
// interface declares ForEach with the value-before-key typed func param, not
// just some `ForEach(` substring. Anti-drift: the binding closure func(any,
// string) only compiles against a matching layer-1 signature.
func TestCATH73_Maplike_Layer1_ForEach_HasTypedFnParam(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("StringNodeMap", "", maplike("DOMString", "Node", false))
	src := sourceOf(t, firstDecl(t, codegen.NewInterfaceDecls(def, tm, codegen.NewDiagnostics())), "iter")

	const want = "ForEach(Fn func(any, string))"
	if !strings.Contains(src, want) {
		t.Errorf("layer-1 maplike must declare %q (value-before-key typed param)\n%s", want, src)
	}
}

// TestCATH73_Maplike_ForEach_PrimitiveValue_UsesToValue flips the wrap branch:
// maplike<Node, long> has a primitive value and an object key, so the adapter
// must wrap the value via VM().ToValue and the key via WrapAny — proving the
// per-arg wrap is selected by type, not hardcoded to WrapAny for the value slot.
func TestCATH73_Maplike_ForEach_PrimitiveValue_UsesToValue(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("NodeLongMap", "", maplike("Node", "long", false))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// value (long→ToValue) before key (Node→WrapAny), then the map.
	const want = "b.ctx.VM().ToValue(_v), b.ctx.WrapAny(_k), call.This"
	if !strings.Contains(src, want) {
		t.Errorf("maplike<Node,long> forEach must wrap primitive value via ToValue and object key via WrapAny — expected %q\n%s", want, src)
	}
}

// TestCATH73_ReadonlyMaplike_StillHasForEach asserts forEach is a reader, present
// even on readonly maplike (WebIDL exposes forEach regardless of readonly).
func TestCATH73_ReadonlyMaplike_StillHasForEach(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("RoMap", "", maplike("DOMString", "long", true))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if !strings.Contains(src, `case "forEach"`) {
		t.Errorf("readonly maplike must still expose forEach (it is a reader, not a mutator)\n%s", src)
	}
	// Sanity: mutators remain absent on readonly.
	if strings.Contains(src, `case "set"`) {
		t.Errorf("readonly maplike must not expose set\n%s", src)
	}
}
