package jsbinding_test

// CATH-66 Phase-0 red tests for the runtime shim. They stand up a real goja
// runtime and exercise the contract; they FAIL against the Phase-0 stubs and turn
// green as the shim is implemented.

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/iansmith/webidl/jsbinding"
)

// --- fakes ------------------------------------------------------------------

type fakeEnv struct{ root any }

func (e fakeEnv) Root() any                            { return e.root }
func (e fakeEnv) Construct(string, []any) any          { return nil }

type fakeNode struct {
	tag   string
	attrs map[string]string
}

func (n *fakeNode) GetAttribute(k string) (string, bool) { v, ok := n.attrs[k]; return v, ok }
func (n *fakeNode) SetAttribute(k, v string) {
	if n.attrs == nil {
		n.attrs = map[string]string{}
	}
	n.attrs[k] = v
}
func (n *fakeNode) RemoveAttribute(k string)   { delete(n.attrs, k) }
func (n *fakeNode) HasAttribute(k string) bool { _, ok := n.attrs[k]; return ok }

type fakeDyn struct{}

func (fakeDyn) Get(string) goja.Value      { return goja.Undefined() }
func (fakeDyn) Set(string, goja.Value) bool { return false }
func (fakeDyn) Has(string) bool            { return false }
func (fakeDyn) Delete(string) bool         { return false }
func (fakeDyn) Keys() []string             { return nil }

func newCtx() *jsbinding.Ctx { return jsbinding.NewCtx(goja.New(), fakeEnv{}) }

// --- identity cache: wrap / unwrap ------------------------------------------

func TestWrap_IdentityCached(t *testing.T) {
	ctx := newCtx()
	impl := &fakeNode{}
	mk := func() goja.DynamicObject { return fakeDyn{} }
	v1 := ctx.Wrap(impl, mk)
	v2 := ctx.Wrap(impl, mk)
	if v1 == nil {
		t.Fatal("Wrap returned nil")
	}
	if !v1.SameAs(v2) {
		t.Errorf("Wrap must return the same goja value for the same impl (=== identity)")
	}
}

func TestUnwrap_RoundTrip(t *testing.T) {
	ctx := newCtx()
	impl := &fakeNode{}
	v := ctx.Wrap(impl, func() goja.DynamicObject { return fakeDyn{} })
	if got := ctx.Unwrap(v); got != any(impl) {
		t.Errorf("Unwrap(Wrap(impl)) = %v, want the original impl", got)
	}
}

func TestUnwrap_NullUndefined_Nil(t *testing.T) {
	ctx := newCtx()
	if ctx.Unwrap(goja.Null()) != nil || ctx.Unwrap(goja.Undefined()) != nil {
		t.Errorf("Unwrap(null/undefined) must be nil")
	}
}

// --- argument coercion ------------------------------------------------------

func TestCoerce_Primitives(t *testing.T) {
	ctx := newCtx()
	vm := ctx.VM()
	if got := jsbinding.Coerce[string](ctx, vm.ToValue("hi")); got != "hi" {
		t.Errorf("Coerce[string] = %q, want hi", got)
	}
	if got := jsbinding.Coerce[int32](ctx, vm.ToValue(42)); got != 42 {
		t.Errorf("Coerce[int32] = %d, want 42", got)
	}
	if got := jsbinding.Coerce[float64](ctx, vm.ToValue(3.5)); got != 3.5 {
		t.Errorf("Coerce[float64] = %v, want 3.5", got)
	}
	if got := jsbinding.Coerce[bool](ctx, vm.ToValue(true)); got != true {
		t.Errorf("Coerce[bool] = %v, want true", got)
	}
}

// --- overload arg-kind classification ---------------------------------------

func TestArgKind(t *testing.T) {
	ctx := newCtx()
	vm := ctx.VM()
	cases := []struct {
		val  goja.Value
		want jsbinding.Kind
	}{
		{vm.ToValue("x"), jsbinding.KindString},
		{vm.ToValue(1), jsbinding.KindNumber},
		{vm.ToValue(true), jsbinding.KindBoolean},
		{goja.Null(), jsbinding.KindNull},
		{goja.Undefined(), jsbinding.KindUndefined},
		{vm.NewObject(), jsbinding.KindObject},
	}
	for _, c := range cases {
		if got := ctx.ArgKind(c.val); got != c.want {
			t.Errorf("ArgKind(%v) = %d, want %d", c.val, got, c.want)
		}
	}
}

// --- [Reflect] attribute-store bridge ---------------------------------------

func TestReflect_StringRoundTrip(t *testing.T) {
	ctx := newCtx()
	n := &fakeNode{}
	ctx.ReflectSetString(n, "id", "hero")
	if got := ctx.ReflectGetString(n, "id"); got != "hero" {
		t.Errorf("ReflectGetString after set = %q, want hero", got)
	}
}

func TestReflect_BoolPresence(t *testing.T) {
	ctx := newCtx()
	n := &fakeNode{}
	ctx.ReflectSetBool(n, "hidden", true)
	if !ctx.ReflectGetBool(n, "hidden") {
		t.Errorf("ReflectGetBool after set true must be true (presence)")
	}
	ctx.ReflectSetBool(n, "hidden", false)
	if ctx.ReflectGetBool(n, "hidden") {
		t.Errorf("ReflectSetBool false must remove the attribute (presence)")
	}
}

func TestReflect_Uint32RoundTrip(t *testing.T) {
	ctx := newCtx()
	n := &fakeNode{}
	ctx.ReflectSetUint32(n, "width", 640)
	if got := ctx.ReflectGetUint32(n, "width"); got != 640 {
		t.Errorf("ReflectGetUint32 after set = %d, want 640", got)
	}
}

// --- error helper -----------------------------------------------------------

func TestThrowType_Panics(t *testing.T) {
	vm := goja.New()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("ThrowType must panic with a JS TypeError")
		}
		if ex, ok := r.(*goja.Exception); !ok || !strings.Contains(ex.Error(), "boom") {
			t.Errorf("ThrowType must panic with a goja TypeError carrying the message; got %T: %v", r, r)
		}
	}()
	jsbinding.ThrowType(vm, "boom")
}

// ===========================================================================
// CATH-66 adversary-gap tests (Step 0f)
// ===========================================================================

// elemBinding is a representative hand-written generated-style DynamicObject for
// the end-to-end test (the real one is generated; this exercises the shim seam).
type elemBinding struct {
	ctx  *jsbinding.Ctx
	impl *fakeNode
}

func (b *elemBinding) Get(key string) goja.Value {
	switch key {
	case "id":
		return b.ctx.VM().ToValue(b.ctx.ReflectGetString(b.impl, "id"))
	case "describe":
		return b.ctx.VM().ToValue(func(call goja.FunctionCall) goja.Value {
			return b.ctx.VM().ToValue("node:" + b.impl.tag)
		})
	}
	return goja.Undefined()
}
func (b *elemBinding) Set(key string, v goja.Value) bool {
	if key == "id" {
		b.ctx.ReflectSetString(b.impl, "id", jsbinding.Coerce[string](b.ctx, v))
		return true
	}
	return false
}
func (b *elemBinding) Has(key string) bool { return key == "id" || key == "describe" }
func (b *elemBinding) Delete(string) bool  { return false }
func (b *elemBinding) Keys() []string      { return []string{"id", "describe"} }

// G1 — the AC #1 headline: stand up a goja runtime, install a wrapped impl, and
// drive attribute get/set + method dispatch from JS into the fake impl.
func TestEndToEnd_AttrAndMethod_HitsImpl(t *testing.T) {
	ctx := newCtx()
	vm := ctx.VM()
	impl := &fakeNode{tag: "div"}
	obj := ctx.Wrap(impl, func() goja.DynamicObject { return &elemBinding{ctx: ctx, impl: impl} })
	if obj == nil {
		t.Fatal("Wrap returned nil")
	}
	vm.Set("el", obj)

	res, err := vm.RunString(`el.id = "hero"; el.id`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if res.String() != "hero" {
		t.Errorf("el.id = %q, want hero", res.String())
	}
	if impl.attrs["id"] != "hero" {
		t.Errorf("set did not reach the impl: %v", impl.attrs)
	}
	d, err := vm.RunString(`el.describe()`)
	if err != nil {
		t.Fatalf("RunString(describe): %v", err)
	}
	if d.String() != "node:div" {
		t.Errorf("el.describe() = %q, want node:div", d.String())
	}
}

// G1 — JS-side === identity for the same impl.
func TestEndToEnd_Identity_TripleEquals(t *testing.T) {
	ctx := newCtx()
	vm := ctx.VM()
	impl := &fakeNode{tag: "div"}
	mk := func() goja.DynamicObject { return &elemBinding{ctx: ctx, impl: impl} }
	vm.Set("a", ctx.Wrap(impl, mk))
	vm.Set("b", ctx.Wrap(impl, mk))
	res, err := vm.RunString(`a === b`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !res.ToBoolean() {
		t.Errorf("same impl must wrap to === objects")
	}
}

// G3 — Unwrap of a never-wrapped object must be nil (no false match).
func TestUnwrap_NonCachedObject_Nil(t *testing.T) {
	ctx := newCtx()
	ctx.Wrap(&fakeNode{}, func() goja.DynamicObject { return fakeDyn{} }) // populate cache
	if got := ctx.Unwrap(ctx.VM().NewObject()); got != nil {
		t.Errorf("Unwrap of a never-wrapped object = %v, want nil", got)
	}
}

// G3/G7 — distinct impls wrap to distinct values and unwrap to themselves.
func TestWrapUnwrap_DistinctImpls(t *testing.T) {
	ctx := newCtx()
	a, b := &fakeNode{tag: "a"}, &fakeNode{tag: "b"}
	va := ctx.Wrap(a, func() goja.DynamicObject { return &elemBinding{ctx: ctx, impl: a} })
	vb := ctx.Wrap(b, func() goja.DynamicObject { return &elemBinding{ctx: ctx, impl: b} })
	if va == nil || vb == nil {
		t.Fatal("Wrap returned nil")
	}
	if va.SameAs(vb) {
		t.Errorf("distinct impls must wrap to distinct values")
	}
	if ctx.Unwrap(va) != any(a) || ctx.Unwrap(vb) != any(b) {
		t.Errorf("Unwrap crossed the impls")
	}
}

// G4 — nullable coercion: null/undefined → nil pointer; a real value → non-nil.
func TestCoerce_Nullable(t *testing.T) {
	ctx := newCtx()
	vm := ctx.VM()
	if p := jsbinding.Coerce[*string](ctx, goja.Null()); p != nil {
		t.Errorf("Coerce[*string](null) = %v, want nil", p)
	}
	if p := jsbinding.Coerce[*string](ctx, vm.ToValue("x")); p == nil || *p != "x" {
		t.Errorf("Coerce[*string](\"x\") must be a non-nil pointer to x")
	}
}

// G4 — sequence → slice.
func TestCoerce_Sequence(t *testing.T) {
	ctx := newCtx()
	vm := ctx.VM()
	got := jsbinding.Coerce[[]string](ctx, vm.NewArray("a", "b"))
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Coerce[[]string] = %v, want [a b]", got)
	}
}

// G4 — callback: the extracted Callable invokes the JS function.
func TestCallback_Invokes(t *testing.T) {
	ctx := newCtx()
	vm := ctx.VM()
	fnVal, err := vm.RunString(`(function(x){ return x + 1; })`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	cb := ctx.Callback(fnVal)
	if cb == nil {
		t.Fatal("Callback returned nil")
	}
	res, err := cb(goja.Undefined(), vm.ToValue(41))
	if err != nil || res.ToInteger() != 42 {
		t.Errorf("Callback invoke = %v (err %v), want 42", res, err)
	}
}

// G8 — SameObject memoizes: same (owner,key) → === and compute runs once.
func TestSameObject_Memoizes(t *testing.T) {
	ctx := newCtx()
	vm := ctx.VM()
	owner := &fakeNode{}
	n := 0
	compute := func() goja.Value { n++; return vm.NewObject() }
	a := ctx.SameObject(owner, "frames", compute)
	b := ctx.SameObject(owner, "frames", compute)
	if a == nil {
		t.Fatal("SameObject returned nil")
	}
	if !a.SameAs(b) {
		t.Errorf("SameObject must return the same object on repeated reads")
	}
	if n != 1 {
		t.Errorf("compute ran %d times, want 1 (memoized)", n)
	}
}

// G9 — AsArrayIndex canonical-index rules.
func TestAsArrayIndex_Canonical(t *testing.T) {
	cases := []struct {
		in  string
		idx uint32
		ok  bool
	}{
		{"0", 0, true}, {"42", 42, true},
		{"x", 0, false}, {"01", 0, false}, {"-1", 0, false}, {"", 0, false},
	}
	for _, c := range cases {
		idx, ok := jsbinding.AsArrayIndex(c.in)
		if ok != c.ok || (ok && idx != c.idx) {
			t.Errorf("AsArrayIndex(%q) = (%d,%v), want (%d,%v)", c.in, idx, ok, c.idx, c.ok)
		}
	}
}

// G12 — signed long reflect round-trip + get-on-absent default 0.
func TestReflect_Int32RoundTripAndDefault(t *testing.T) {
	ctx := newCtx()
	n := &fakeNode{}
	if got := ctx.ReflectGetInt32(n, "tabindex"); got != 0 {
		t.Errorf("ReflectGetInt32 on absent attr = %d, want 0 (default)", got)
	}
	ctx.ReflectSetInt32(n, "tabindex", -3)
	if got := ctx.ReflectGetInt32(n, "tabindex"); got != -3 {
		t.Errorf("ReflectGetInt32 after set = %d, want -3", got)
	}
}
