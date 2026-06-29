package codegen_test

// CATH-71 Phase 0: red tests — iterCallBody must emit a typed adapter closure
// for renderForEach, not pass a raw goja.Callable to ForEach.
//
// These tests describe the expected post-fix behavior. They fail on the current
// (broken) output because iterCallBody line 532 emits:
//
//	b.impl.ForEach(b.ctx.Callback(call.Argument(0)))
//
// which passes goja.Callable where ForEach expects func(V, uint32).

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// TestCATH71_ForEach_ValueOnly_NoRawCallable asserts that a value-only
// iterable<V> generates a typed adapter closure for forEach, not a bare
// goja.Callable argument.
func TestCATH71_ForEach_ValueOnly_NoRawCallable(t *testing.T) {
	t.Parallel()
	def := regularMergedDef("NodeList", "", iterable("any"))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// The broken emission passes Callback(...) directly to ForEach.
	if strings.Contains(src, "ForEach(b.ctx.Callback(") {
		t.Errorf("ForEach must not receive a raw goja.Callable — need a typed adapter closure\n%s", src)
	}
	// The fix emits a typed closure literal as the ForEach argument.
	if !strings.Contains(src, "b.impl.ForEach(func(") {
		t.Errorf("ForEach must receive a typed adapter closure starting with func(\n%s", src)
	}
	// The adapter must wrap values for JS (WrapAny for the "any" value type).
	if !strings.Contains(src, "b.ctx.WrapAny(") {
		t.Errorf("adapter closure must wrap the value arg via WrapAny for object/any types\n%s", src)
	}
	// The uint32 index must go through VM().ToValue.
	if !strings.Contains(src, "b.ctx.VM().ToValue(") {
		t.Errorf("adapter closure must wrap the uint32 key arg via VM().ToValue\n%s", src)
	}
}

// TestCATH71_ForEach_PairIterator_NoRawCallable asserts that a pair-iterable
// iterable<K, V> also generates a typed adapter closure for forEach.
func TestCATH71_ForEach_PairIterator_NoRawCallable(t *testing.T) {
	t.Parallel()
	// DOMString→string (primitive), Node→any (unknown interface type).
	def := regularMergedDef("Map", "", pairIterable("DOMString", "Node"))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	if strings.Contains(src, "ForEach(b.ctx.Callback(") {
		t.Errorf("pair-iterable ForEach must not receive a raw goja.Callable\n%s", src)
	}
	if !strings.Contains(src, "b.impl.ForEach(func(") {
		t.Errorf("pair-iterable ForEach must receive a typed adapter closure\n%s", src)
	}
}
