package codegen_test

// CATH-77 Phase 0: red tests — a true end-to-end harness that runs the REAL
// generator, compiles its output with a hand-written fake impl, executes a JS
// program in goja against the generated bindings, and asserts the JS behavior.
//
// This closes the gap between TestCATH66_GeneratedCompiles (generate → go build,
// never runs JS) and jsbinding's TestEndToEnd_* (runs JS, but against
// hand-written bindings, not generated ones).
//
// RED until runGeneratedJS is implemented: the helper is a stub that fails.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

// cath77MaplikeIDL is the seed fixture: a bare maplike interface (no
// constructor, so the runner seeds it via ctx.Wrap rather than env.Construct).
const cath77MaplikeIDL = `
[Exposed=Window]
interface StringMap {
  maplike<DOMString, long>;
};
`

// cath77RunnerPrefix is the shared head of every package-gen runner dropped into
// the generated temp module: it must be package gen to construct
// StringMapBinding (unexported fields). It defines a fake Env and a fake
// StringMap impl backed by ordered slices (so ForEach iterates in insertion
// order — a Go map would be random). Per-case test functions are appended.
const cath77RunnerPrefix = `package gen

import (
	"iter"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/iansmith/webidl/jsbinding"
)

type e2eEnv struct{}

func (e2eEnv) Root() any                   { return nil }
func (e2eEnv) Construct(string, []any) any { return nil }

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

func seed(t *testing.T) (*goja.Runtime, *jsbinding.Ctx) {
	t.Helper()
	vm := goja.New()
	ctx := jsbinding.NewCtx(vm, e2eEnv{})
	impl := &fakeStringMap{}
	impl.Set("a", 1)
	impl.Set("b", 2)
	m := ctx.Wrap(impl, func() goja.DynamicObject { return &StringMapBinding{ctx: ctx, impl: impl} })
	vm.Set("m", m)
	return vm, ctx
}

var _ = strings.Contains
`

// cath77ForEachHappyTest asserts value-first callback arg order end-to-end: the
// pushed string is "<key>=<value>", so out == "a=1,b=2" proves v is the number
// and k the key (a swapped binding would yield "1=a,2=b").
const cath77ForEachHappyTest = `
func TestE2E(t *testing.T) {
	vm, _ := seed(t)
	res, err := vm.RunString("var o=[]; m.forEach(function(v,k){o.push(k+'='+v)}); o.join(',')")
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got := res.String(); got != "a=1,b=2" {
		t.Fatalf("forEach output = %q, want \"a=1,b=2\"", got)
	}
}
`

// cath77ForEachThrowTest exercises the generated forEach's *goja.Exception
// re-panic path end-to-end: a throwing JS callback must surface as a RunString
// error (not a swallowed throw, not a Go panic/crash).
const cath77ForEachThrowTest = `
func TestE2E(t *testing.T) {
	vm, _ := seed(t)
	_, err := vm.RunString("m.forEach(function(v,k){ throw new Error('boom '+k) })")
	if err == nil {
		t.Fatalf("a throwing forEach callback must surface as a RunString error, got nil")
	}
	if !strings.Contains(err.Error(), "boom a") {
		t.Fatalf("thrown error must propagate to JS caller; got %q, want it to contain \"boom a\"", err.Error())
	}
}
`

// TestCATH77_EndToEnd_MaplikeForEach_RunsJS is the headline: generate a real
// maplike IDL, compile the output with the fake impl + runner, run the JS
// forEach program in goja, and assert it produces "a=1,b=2".
func TestCATH77_EndToEnd_MaplikeForEach_RunsJS(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found; skipping end-to-end run")
	}
	runGeneratedJS(t, cath77MaplikeIDL, cath77RunnerPrefix+cath77ForEachHappyTest)
}

// TestCATH77_EndToEnd_MaplikeForEach_ThrowPropagates asserts a thrown JS
// exception from the callback propagates out through the generated adapter.
func TestCATH77_EndToEnd_MaplikeForEach_ThrowPropagates(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found; skipping end-to-end run")
	}
	runGeneratedJS(t, cath77MaplikeIDL, cath77RunnerPrefix+cath77ForEachThrowTest)
}

// runGeneratedJS is the reusable harness: given a WebIDL fixture and a package-gen
// runner test source, it generates layer-1 + bindings into a temp module, drops
// the runner in, and runs `go test` — failing t (with the captured output) if the
// generated code doesn't compile, the runner test does not actually execute, or
// the JS assertion doesn't hold.
//
func runGeneratedJS(t *testing.T, idl, runnerSrc string) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not found; skipping end-to-end run")
	}

	dir := t.TempDir()
	ir := mustIR(t, idl)
	if err := codegen.Generate(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("Generate (layer-1): %v", err)
	}
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}

	// go.mod / go.sum / offline env — same shape as TestCATH66_GeneratedCompiles:
	// replace the webidl module with the local checkout and resolve goja (+ deps)
	// from the module cache offline (they are cached because catherine requires goja).
	cathRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	gomod := "module gentest\n\ngo 1.26\n\nrequire (\n" +
		"\tgithub.com/iansmith/webidl v0.0.0\n" +
		"\tgithub.com/dop251/goja v0.0.0-20260106131823-651366fbe6e3\n)\n\n" +
		"replace github.com/iansmith/webidl => " + cathRoot + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if sum, err := os.ReadFile(filepath.Join(cathRoot, "go.sum")); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644)
	}

	// The runner is package gen (reaches the binding's unexported fields) and runs
	// under `go test`.
	if err := os.WriteFile(filepath.Join(dir, "zz_e2e_test.go"), []byte(runnerSrc), 0o644); err != nil {
		t.Fatalf("write runner: %v", err)
	}

	cmd := exec.Command(goBin, "test", "-run", "TestE2E", "-v", "-count=1", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	out, runErr := cmd.CombinedOutput()

	// Integrity: `go test` exits 0 even when -run matches nothing ("no tests to
	// run"), so a passing exit code is NOT sufficient — require the runner test to
	// have actually executed and passed.
	if runErr != nil || !strings.Contains(string(out), "--- PASS: TestE2E") {
		t.Fatalf("end-to-end run did not pass (exit: %v):\n%s", runErr, out)
	}
}
