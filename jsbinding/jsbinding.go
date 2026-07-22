// Package jsbinding is the hand-written runtime shim that catherine's generated
// goja binding code (CATH-64/65 accessors + CATH-66 Register) depends on. It owns
// the goja seam: the per-runtime context + identity cache (wrap/unwrap), argument
// coercion, the [Reflect] attribute-store bridge, overload arg-kind classification,
// and the JS error helper.
//
// Design: CATH-66. The generated code references these symbols qualified (alias
// `rt`). The engine (louis14) supplies an Env and impls satisfying the layer-1
// interfaces + AttrStore; this is louis14's only catherine runtime dependency.
package jsbinding

import (
	"reflect"
	"strconv"

	"github.com/dop251/goja"
)

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

// sameKey identifies a [SameObject] memoization slot.
type sameKey struct {
	owner any
	key   string
}

// Ctx is the per-runtime context + bidirectional identity cache.
type Ctx struct {
	vm   *goja.Runtime
	env  Env
	fwd  map[any]*goja.Object   // impl → wrapped object
	rev  map[*goja.Object]any   // wrapped object → impl
	same map[sameKey]goja.Value // [SameObject] memo
}

// NewCtx returns a Ctx bound to vm and env with empty caches.
func NewCtx(vm *goja.Runtime, env Env) *Ctx {
	return &Ctx{
		vm:   vm,
		env:  env,
		fwd:  map[any]*goja.Object{},
		rev:  map[*goja.Object]any{},
		same: map[sameKey]goja.Value{},
	}
}

// VM exposes the runtime. Generated code in the consumer's package reaches the
// runtime through this accessor (the vm field is unexported).
func (c *Ctx) VM() *goja.Runtime { return c.vm }

// Environment exposes the engine environment.
func (c *Ctx) Environment() Env { return c.env }

// Wrap returns the cached goja value for impl, or creates one via mk and caches
// it in both directions so JS === holds and Unwrap round-trips.
func (c *Ctx) Wrap(impl any, mk func() goja.DynamicObject) goja.Value {
	if obj, ok := c.fwd[impl]; ok {
		return obj
	}
	obj := c.vm.NewDynamicObject(mk())
	c.fwd[impl] = obj
	c.rev[obj] = impl
	return obj
}

// nullish reports whether v is the absent JS value: a nil goja.Value, null, or
// undefined. Coercion and Unwrap collapse all three to the Go zero value.
func nullish(v goja.Value) bool {
	return v == nil || goja.IsNull(v) || goja.IsUndefined(v)
}

// Unwrap returns the impl behind a wrapped value, or nil for null/undefined or a
// value that was never wrapped.
func (c *Ctx) Unwrap(v goja.Value) any {
	if nullish(v) {
		return nil
	}
	return c.rev[v.ToObject(c.vm)]
}

// WrapAny wraps an object-typed return value whose static interface type the
// generator could not resolve (it maps to Go `any`). Identity is preserved for
// an impl already in the cache (the common "method returns an object it was
// handed" case); an as-yet-unwrapped impl degrades to vm.ToValue, since the shim
// cannot know which binding type to construct. A typed wrap (with the binding)
// happens at construction time via Register/the manifest New factory. nil → null.
func (c *Ctx) WrapAny(impl any) goja.Value {
	if impl == nil {
		return goja.Null()
	}
	if obj, ok := c.fwd[impl]; ok {
		return obj
	}
	return c.vm.ToValue(impl)
}

// Coerce converts a JS value to the Go type T via goja's ExportTo (primitives,
// sequences, nullable pointers). null/undefined yield the zero value of T (nil
// for pointer/slice T). Object/interface args use Unwrap instead (the generator
// chooses per argument class).
func Coerce[T any](c *Ctx, v goja.Value) T {
	var out T
	if nullish(v) {
		return out
	}
	_ = c.vm.ExportTo(v, &out)
	return out
}

// CoerceArgs coerces the trailing JS call arguments (from index `from` onward)
// into a []T, one Coerce[T] per argument. Used to spread a variadic WebIDL
// operation's arguments into a Go variadic parameter. Returns nil when there
// are no arguments at or after `from`.
func CoerceArgs[T any](c *Ctx, call goja.FunctionCall, from int) []T {
	if from >= len(call.Arguments) {
		return nil
	}
	out := make([]T, 0, len(call.Arguments)-from)
	for _, a := range call.Arguments[from:] {
		out = append(out, Coerce[T](c, a))
	}
	return out
}

// UnwrapArgs is the object-typed counterpart of CoerceArgs: it Unwraps each
// trailing JS call argument (from index `from` onward) to its impl, for a
// variadic operation whose element type is an interface. Returns nil when there
// are no arguments at or after `from`.
func (c *Ctx) UnwrapArgs(call goja.FunctionCall, from int) []any {
	if from >= len(call.Arguments) {
		return nil
	}
	out := make([]any, 0, len(call.Arguments)-from)
	for _, a := range call.Arguments[from:] {
		out = append(out, c.Unwrap(a))
	}
	return out
}

// AsArrayIndex reports whether key is a canonical array index ("0", "42", … but
// not "01", "-1", "", or an overflow).
func AsArrayIndex(key string) (uint32, bool) {
	n, err := strconv.ParseUint(key, 10, 32)
	if err != nil {
		return 0, false
	}
	if strconv.FormatUint(n, 10) != key { // reject non-canonical forms (leading zeros)
		return 0, false
	}
	return uint32(n), true
}

// ArgKind classifies a JS value for overload dispatch.
func (c *Ctx) ArgKind(v goja.Value) Kind {
	if v == nil || goja.IsUndefined(v) {
		return KindUndefined
	}
	if goja.IsNull(v) {
		return KindNull
	}
	switch t := v.ExportType(); t.Kind() {
	case reflect.Bool:
		return KindBoolean
	case reflect.String:
		return KindString
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return KindNumber
	default:
		return KindObject
	}
}

// SameObject memoizes an attribute's object so repeated reads are === (CATH-65),
// keyed by (owner, attribute name). compute runs at most once per slot.
func (c *Ctx) SameObject(owner any, key string, compute func() goja.Value) goja.Value {
	k := sameKey{owner, key}
	if v, ok := c.same[k]; ok {
		return v
	}
	v := compute()
	c.same[k] = v
	return v
}

// WrapSeq materializes an iter.Seq[T] (passed as any) into a JS array (CATH-64
// iteration). Lazy iterator-protocol wrapping is a future refinement; an array
// satisfies for…of, which is what the generated values()/keys()/entries() need.
func (c *Ctx) WrapSeq(seq any) goja.Value {
	rv := reflect.ValueOf(seq)
	if rv.Kind() != reflect.Func || rv.Type().NumIn() != 1 {
		return goja.Undefined()
	}
	var items []any
	yield := reflect.MakeFunc(rv.Type().In(0), func(args []reflect.Value) []reflect.Value {
		if len(args) == 1 {
			items = append(items, args[0].Interface())
		}
		return []reflect.Value{reflect.ValueOf(true)}
	})
	rv.Call([]reflect.Value{yield})
	arr := c.vm.NewArray()
	for i, it := range items {
		_ = arr.Set(strconv.Itoa(i), c.vm.ToValue(it))
	}
	return arr
}

// Callback extracts the callable from a JS value for callback-typed args
// (CATH-63/64). Returns nil if v is not callable.
func (c *Ctx) Callback(v goja.Value) goja.Callable {
	fn, ok := goja.AssertFunction(v)
	if !ok {
		return nil
	}
	return fn
}

// Reflected content-attribute accessors (CATH-65). impl must satisfy AttrStore;
// a non-AttrStore impl degrades to zero/no-op rather than panicking.

func (c *Ctx) ReflectGetString(impl any, name string) string {
	if s, ok := impl.(AttrStore); ok {
		v, _ := s.GetAttribute(name)
		return v
	}
	return ""
}

func (c *Ctx) ReflectSetString(impl any, name, v string) {
	if s, ok := impl.(AttrStore); ok {
		s.SetAttribute(name, v)
	}
}

// Boolean reflection is presence-based: true ⇔ the attribute is present.
func (c *Ctx) ReflectGetBool(impl any, name string) bool {
	if s, ok := impl.(AttrStore); ok {
		return s.HasAttribute(name)
	}
	return false
}

func (c *Ctx) ReflectSetBool(impl any, name string, v bool) {
	s, ok := impl.(AttrStore)
	if !ok {
		return
	}
	if v {
		s.SetAttribute(name, "")
	} else {
		s.RemoveAttribute(name)
	}
}

func (c *Ctx) ReflectGetInt32(impl any, name string) int32 {
	if s, ok := impl.(AttrStore); ok {
		if raw, present := s.GetAttribute(name); present {
			if n, err := strconv.ParseInt(raw, 10, 32); err == nil {
				return int32(n)
			}
		}
	}
	return 0 // default when absent or unparseable
}

func (c *Ctx) ReflectSetInt32(impl any, name string, v int32) {
	if s, ok := impl.(AttrStore); ok {
		s.SetAttribute(name, strconv.FormatInt(int64(v), 10))
	}
}

func (c *Ctx) ReflectGetUint32(impl any, name string) uint32 {
	if s, ok := impl.(AttrStore); ok {
		if raw, present := s.GetAttribute(name); present {
			if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
				return uint32(n)
			}
		}
	}
	return 0
}

func (c *Ctx) ReflectSetUint32(impl any, name string, v uint32) {
	if s, ok := impl.(AttrStore); ok {
		s.SetAttribute(name, strconv.FormatUint(uint64(v), 10))
	}
}

// ThrowType panics with a JS TypeError. Called from inside generated accessor /
// constructor closures (i.e. within a goja call), where goja converts the panic
// into a catchable JS exception.
func ThrowType(vm *goja.Runtime, msg string) {
	panic(vm.NewTypeError(msg))
}
