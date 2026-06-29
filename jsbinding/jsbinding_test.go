package jsbinding_test

// CATH-66 Phase-0 red tests for the runtime shim. They stand up a real goja
// runtime and exercise the contract; they FAIL against the Phase-0 stubs and turn
// green as the shim is implemented.

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/iansmith/webidl/jsbinding"
)

// --- fakes ------------------------------------------------------------------

type fakeEnv struct{ root any }

func (e fakeEnv) Root() any                            { return e.root }
func (e fakeEnv) Construct(string, []any) any          { return nil }

type fakeNode struct{ attrs map[string]string }

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
		if recover() == nil {
			t.Errorf("ThrowType must panic with a JS TypeError")
		}
	}()
	jsbinding.ThrowType(vm, "boom")
}
