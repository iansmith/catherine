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

// TestCATH73_Maplike_ForEach_Layer1Method asserts the shared resolveIterMethods
// change also surfaces a ForEach method on the layer-1 interface (anti-drift:
// the binding's b.impl.ForEach(...) call only compiles if layer-1 declares it).
func TestCATH73_Maplike_ForEach_Layer1Method(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("StringNodeMap", "", maplike("DOMString", "Node", false))
	src := sourceOf(t, firstDecl(t, codegen.NewInterfaceDecls(def, tm, codegen.NewDiagnostics())), "iter")

	if !strings.Contains(src, "ForEach(") {
		t.Errorf("layer-1 maplike interface must declare a ForEach method\n%s", src)
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
