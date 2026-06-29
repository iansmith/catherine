// Package jsbinding is the hand-written runtime shim that catherine's generated
// goja binding code (CATH-64/65 accessors + CATH-66 Register) depends on. It owns
// the goja seam: the per-runtime context + identity cache (wrap/unwrap), argument
// coercion, the [Reflect] attribute-store bridge, overload arg-kind classification,
// and the JS error helper.
//
// Design: CATH-66. The generated code references these symbols qualified (alias
// `rt`). The engine (louis14) supplies an Env and impls satisfying the layer-1
// interfaces + AttrStore; this is louis14's only catherine runtime dependency.
//
// NOTE: bodies are Phase-0 stubs — they compile but do not implement the contract
// yet, so the red tests fail. Implementation lands in CATH-66.
package jsbinding

import "github.com/dop251/goja"

// Env is the engine-supplied runtime environment.
type Env interface {
	// Root returns the impl backing the global (Window-like) object whose members
	// are installed as globals by the generated Register. nil → no root.
	Root() any
	// Construct builds an impl for a constructible interface from coerced JS args.
	Construct(name string, args []any) any
}

// AttrStore is the content-attribute interface a reflected impl must satisfy
// (CATH-65 [Reflect] passes the impl here). louis14's *html.Node satisfies it.
type AttrStore interface {
	GetAttribute(name string) (string, bool)
	SetAttribute(name, value string)
	RemoveAttribute(name string)
	HasAttribute(name string) bool
}

// Kind is the coarse runtime kind of a JS value, used by generated overload
// dispatch (CATH-65 argKind).
type Kind int

const (
	KindUndefined Kind = iota
	KindNull
	KindBoolean
	KindNumber
	KindString
	KindObject
)

// ExposedBinding is one entry of the generated exposure registry (CATH-65).
type ExposedBinding struct {
	Name    string
	Globals []string
	New     func(c *Ctx, impl any) goja.Value
}

// Ctx is the per-runtime context + bidirectional identity cache.
type Ctx struct {
	vm  *goja.Runtime
	env Env
	fwd map[any]*goja.Object
	rev map[*goja.Object]any
}

// NewCtx returns a Ctx bound to vm and env with empty caches.
func NewCtx(vm *goja.Runtime, env Env) *Ctx {
	return &Ctx{vm: vm, env: env, fwd: map[any]*goja.Object{}, rev: map[*goja.Object]any{}}
}

// VM exposes the runtime (generated code calls c.vm via the alias too).
func (c *Ctx) VM() *goja.Runtime { return c.vm }

// Env exposes the environment.
func (c *Ctx) Environment() Env { return c.env }

// Wrap returns the cached goja value for impl, or creates one via mk and caches
// it in both directions so JS === holds and Unwrap round-trips. STUB.
func (c *Ctx) Wrap(impl any, mk func() goja.DynamicObject) goja.Value {
	return nil
}

// Unwrap returns the impl behind a wrapped value, or nil for null/undefined or a
// non-wrapped value. STUB.
func (c *Ctx) Unwrap(v goja.Value) any {
	return nil
}

// Coerce converts a JS value to the Go type T via goja's ExportTo (primitives,
// sequences). Object/interface args use Unwrap instead (the generator chooses).
// STUB.
func Coerce[T any](c *Ctx, v goja.Value) T {
	var zero T
	return zero
}

// AsArrayIndex reports whether key is a canonical array index. STUB.
func AsArrayIndex(key string) (uint32, bool) {
	return 0, false
}

// ArgKind classifies a JS value for overload dispatch. STUB.
func (c *Ctx) ArgKind(v goja.Value) Kind {
	return KindUndefined
}

// SameObject memoizes an attribute's object so repeated reads are === (CATH-65).
// STUB.
func (c *Ctx) SameObject(owner any, key string, compute func() goja.Value) goja.Value {
	return nil
}

// WrapSeq wraps an iter.Seq result as a JS iterator (CATH-64). STUB.
func (c *Ctx) WrapSeq(seq any) goja.Value {
	return nil
}

// Callback extracts the callable from a JS value for callback-typed args
// (CATH-63/64). STUB.
func (c *Ctx) Callback(v goja.Value) goja.Callable {
	return nil
}

// Reflected content-attribute accessors (CATH-65). impl must satisfy AttrStore.
// STUBS.

func (c *Ctx) ReflectGetString(impl any, name string) string  { return "" }
func (c *Ctx) ReflectSetString(impl any, name, v string)      {}
func (c *Ctx) ReflectGetBool(impl any, name string) bool      { return false }
func (c *Ctx) ReflectSetBool(impl any, name string, v bool)   {}
func (c *Ctx) ReflectGetInt32(impl any, name string) int32    { return 0 }
func (c *Ctx) ReflectSetInt32(impl any, name string, v int32) {}
func (c *Ctx) ReflectGetUint32(impl any, name string) uint32  { return 0 }
func (c *Ctx) ReflectSetUint32(impl any, name string, v uint32) {}

// ThrowType panics with a JS TypeError (arg-count / type failures). STUB.
func ThrowType(vm *goja.Runtime, msg string) {}
