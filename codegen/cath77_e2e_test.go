package codegen_test

// CATH-77 Phase 0: red test — a true end-to-end harness that runs the REAL
// generator, compiles its output with a hand-written fake impl, executes a JS
// program in goja against the generated bindings, and asserts the JS output.
//
// This closes the gap between TestCATH66_GeneratedCompiles (generate → go build,
// never runs JS) and jsbinding's TestEndToEnd_* (runs JS, but against
// hand-written bindings, not generated ones).
//
// RED until runGeneratedJS is implemented: the helper is a stub that fails.

import (
	"os/exec"
	"testing"
)

// cath77MaplikeIDL is the seed fixture: a bare maplike interface (no
// constructor, so the runner seeds it via ctx.Wrap rather than env.Construct).
const cath77MaplikeIDL = `
[Exposed=Window]
interface StringMap {
  maplike<DOMString, long>;
};
`

// cath77MaplikeRunner is a package-gen *_test.go dropped into the generated temp
// module. It must be package gen to construct StringMapBinding (unexported
// fields). It seeds a fake impl {a:1, b:2} (insertion-ordered so forEach is
// deterministic), runs Set.prototype.forEach-style iteration from JS, and
// asserts the value-first callback arg order end-to-end: the pushed string is
// "<key>=<value>", so out == "a=1,b=2" proves v is the number and k the key.
const cath77MaplikeRunner = `package gen

import (
	"iter"
	"testing"

	"github.com/dop251/goja"
	"github.com/iansmith/webidl/jsbinding"
)

type e2eEnv struct{}

func (e2eEnv) Root() any                   { return nil }
func (e2eEnv) Construct(string, []any) any { return nil }

// fakeStringMap implements the generated StringMap interface. Backed by ordered
// slices so ForEach iterates in insertion order (a Go map would be random).
type fakeStringMap struct {
	keys []string
	vals []int32
}

func (m *fakeStringMap) Get(k string) int32 {
	for i, kk := range m.keys {
		if kk == k {
			return m.vals[i]
		}
	}
	return 0
}
func (m *fakeStringMap) Has(k string) bool {
	for _, kk := range m.keys {
		if kk == k {
			return true
		}
	}
	return false
}
func (m *fakeStringMap) Keys() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, k := range m.keys {
			if !yield(k) {
				return
			}
		}
	}
}
func (m *fakeStringMap) Values() iter.Seq[int32] {
	return func(yield func(int32) bool) {
		for _, v := range m.vals {
			if !yield(v) {
				return
			}
		}
	}
}
func (m *fakeStringMap) Entries() iter.Seq2[string, int32] {
	return func(yield func(string, int32) bool) {
		for i := range m.keys {
			if !yield(m.keys[i], m.vals[i]) {
				return
			}
		}
	}
}
func (m *fakeStringMap) Size() int { return len(m.keys) }
func (m *fakeStringMap) ForEach(fn func(int32, string)) {
	for i := range m.keys {
		fn(m.vals[i], m.keys[i]) // (value, key) — value-first per the generated signature
	}
}
func (m *fakeStringMap) Set(k string, v int32) { m.keys = append(m.keys, k); m.vals = append(m.vals, v) }
func (m *fakeStringMap) Delete(string)         {}
func (m *fakeStringMap) Clear()                { m.keys, m.vals = nil, nil }

func TestE2E(t *testing.T) {
	vm := goja.New()
	ctx := jsbinding.NewCtx(vm, e2eEnv{})
	impl := &fakeStringMap{}
	impl.Set("a", 1)
	impl.Set("b", 2)
	m := ctx.Wrap(impl, func() goja.DynamicObject { return &StringMapBinding{ctx: ctx, impl: impl} })
	vm.Set("m", m)

	res, err := vm.RunString("var o=[]; m.forEach(function(v,k){o.push(k+'='+v)}); o.join(',')")
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got := res.String(); got != "a=1,b=2" {
		t.Fatalf("forEach output = %q, want \"a=1,b=2\"", got)
	}
}
`

// TestCATH77_EndToEnd_MaplikeForEach_RunsJS is the headline: generate a real
// maplike IDL, compile the output with the fake impl + runner above, run the JS
// forEach program in goja, and assert it produces "a=1,b=2".
func TestCATH77_EndToEnd_MaplikeForEach_RunsJS(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found; skipping end-to-end run")
	}
	runGeneratedJS(t, cath77MaplikeIDL, cath77MaplikeRunner)
}

// runGeneratedJS is the reusable harness: given a WebIDL fixture and a package-gen
// runner test source, it generates layer-1 + bindings into a temp module, drops
// the runner in, and runs `go test` — failing t (with the captured output) if the
// generated code doesn't compile or the JS assertion doesn't hold.
//
// STUB — implemented in the CATH-77 implementation phase. Fails RED until then.
func runGeneratedJS(t *testing.T, idl, runnerSrc string) {
	t.Helper()
	t.Fatal("runGeneratedJS not implemented (CATH-77 Phase 0 red)")
}
