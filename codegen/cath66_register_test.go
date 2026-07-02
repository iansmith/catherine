package codegen_test

// CATH-66 Phase-0 red tests: the generated bindings must be RE-QUALIFIED against
// the jsbinding runtime package (rt.*), object args/returns must route through
// Unwrap/Wrap, and a Register entrypoint must be generated. These assert on the
// generated SOURCE and fail on the current (unqualified, Register-less) output.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

const cath66IDL = `
interface Node {};

[Exposed=Window]
interface Element : Node {
  [Reflect] attribute DOMString id;
  Node appendChild(Node node);
  constructor(DOMString localName);
};
`

func cath66Generated(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ir := mustIR(t, cath66IDL)
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}
	return readGenerated(t, dir, "bindings.go")
}

// The shim symbols must be qualified against the runtime package, and the package
// imported — generated code no longer assumes package-local bindCtx/coerce.
func TestCATH66_QualifiedRuntimeRefs(t *testing.T) {
	t.Parallel()
	src := cath66Generated(t)
	for _, want := range []string{
		`"github.com/iansmith/webidl/jsbinding"`, // import of the runtime pkg
		"*rt.Ctx",                                // struct field qualified
		"rt.Coerce[",                             // arg coercion qualified
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated bindings must reference the runtime pkg — missing %q\n%s", want, src)
		}
	}
	if strings.Contains(src, "*bindCtx") {
		t.Errorf("generated bindings must not use the unqualified *bindCtx anymore\n%s", src)
	}
}

// Object-typed args unwrap to the impl; object-typed returns go through the cached
// Wrap (not goja's identity-losing default ToValue).
func TestCATH66_ObjectArgsUnwrap_ReturnsWrap(t *testing.T) {
	t.Parallel()
	src := cath66Generated(t)
	if !strings.Contains(src, "b.ctx.Unwrap(call.Argument(0))") {
		t.Errorf("object-typed operation arg must unwrap to the impl\n%s", src)
	}
	// Object-typed returns map to Go `any` (interface names don't resolve in
	// typemap), so they route through the identity-preserving WrapAny.
	if !strings.Contains(src, "b.ctx.WrapAny(") {
		t.Errorf("object-typed return must go through the cached WrapAny\n%s", src)
	}
}

// A Register(vm, env) entrypoint is generated; a constructible interface wires
// env.Construct, a non-constructible one throws an illegal-constructor TypeError.
func TestCATH66_RegisterGenerated(t *testing.T) {
	t.Parallel()
	src := cath66Generated(t)
	if !strings.Contains(src, "func Register(") {
		t.Errorf("a Register(vm, env) entrypoint must be generated\n%s", src)
	}
	if !strings.Contains(src, `env.Construct("Element"`) {
		t.Errorf("a constructible interface must wire env.Construct\n%s", src)
	}
}

// The reflected attribute still routes through the qualified reflect shim.
func TestCATH66_ReflectQualified(t *testing.T) {
	t.Parallel()
	src := cath66Generated(t)
	if !strings.Contains(src, `b.ctx.ReflectGetString(b.impl, "id")`) {
		t.Errorf("reflected getter must call the (qualified) reflect shim\n%s", src)
	}
}

// ===========================================================================
// CATH-66 adversary-gap tests (Step 0f) — re-qualification COMPLETENESS.
// A richer fixture exercises overload (ArgKind + Kind labels), [SameObject],
// an iterable (WrapSeq), and an indexed getter (AsArrayIndex) — none of which
// the simple cath66IDL hits. Generated code must compile against jsbinding, so
// NO unqualified shim symbol and NO unexported-field access may survive.
// ===========================================================================

const cath66RichIDL = `
interface Node {};

[Exposed=Window]
interface Doc {
  [SameObject] readonly attribute Node body;
  undefined add(DOMString s);
  undefined add(Node n);
  getter Node item(unsigned long index);
  iterable<Node>;
};

[Exposed=Window]
interface Range {};
`

func cath66RichGenerated(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ir := mustIR(t, cath66RichIDL)
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}
	return readGenerated(t, dir, "bindings.go")
}

// No unqualified shim symbol may survive — each would be a compile break against
// the runtime package.
func TestCATH66_NoUnqualifiedShimSymbols(t *testing.T) {
	t.Parallel()
	src := cath66RichGenerated(t)
	for _, bad := range []string{
		"coerce[",                 // → rt.Coerce[
		"asArrayIndex(",           // → rt.AsArrayIndex(
		"b.ctx.argKind",           // → b.ctx.ArgKind
		"b.ctx.sameObject",        // → b.ctx.SameObject
		"b.ctx.wrapSeq",           // → b.ctx.WrapSeq
		"b.ctx.callbackFn",        // → b.ctx.Callback
		"b.ctx.vm.",               // → b.ctx.VM().  (vm is unexported cross-package)
		"ctx.vm.NewDynamicObject", // manifest New → ctx.VM().NewDynamicObject
		" bindCtx",                // the local type is gone
	} {
		if strings.Contains(src, bad) {
			t.Errorf("generated source still contains unqualified/unexported %q — will not compile against jsbinding\n%s", bad, src)
		}
	}
}

// The unexported vm field must be reached via the VM() accessor.
func TestCATH66_UsesVMAccessor(t *testing.T) {
	t.Parallel()
	src := cath66RichGenerated(t)
	if !strings.Contains(src, "b.ctx.VM()") {
		t.Errorf("generated code must use the exported VM() accessor, not the unexported field\n%s", src)
	}
}

// Overload / collection paths must also be qualified.
func TestCATH66_QualifiedOverloadAndCollections(t *testing.T) {
	t.Parallel()
	src := cath66RichGenerated(t)
	for _, want := range []string{
		"b.ctx.ArgKind(", // overload dispatch
		"rt.KindString",  // qualified Kind case label
		"b.ctx.SameObject(",
		"rt.AsArrayIndex(", // indexed getter
		"b.ctx.WrapSeq(",   // iterable
	} {
		if !strings.Contains(src, want) {
			t.Errorf("re-qualified output missing %q\n%s", want, src)
		}
	}
}

// Register: non-constructible exposed interface throws; constructor coerces args.
func TestCATH66_Register_IllegalCtorAndArgCoercion(t *testing.T) {
	t.Parallel()
	src := cath66Generated(t) // Element (constructor) + Node (no constructor, lenient-exposed)
	if !strings.Contains(src, "Illegal constructor") {
		t.Errorf("a non-constructible exposed interface must throw an illegal-constructor TypeError\n%s", src)
	}
	if !strings.Contains(src, "rt.Coerce[string]") || !strings.Contains(src, `env.Construct("Element"`) {
		t.Errorf("the constructor must coerce its args before calling env.Construct\n%s", src)
	}
}

// Compile backstop (review G5): generate BOTH backends (layer-1 generated.go +
// bindings.go incl. Register) for a representative IDL into a temp module and
// `go build` it against the real jsbinding package. String asserts can't catch
// an unexported-field access or a leftover unqualified symbol; a real compile can.
const cath66CompileIDL = `
interface Node {};

[Exposed=Window]
interface Element : Node {
  [Reflect] attribute DOMString id;
  attribute long width;
  Node appendChild(Node node);
  constructor(DOMString localName);
  iterable<Node>;
};

[Exposed=Window]
interface StringNodeMap {
  maplike<DOMString, Node>;
};
`

func TestCATH66_GeneratedCompiles(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not found; skipping compile backstop")
	}
	dir := t.TempDir()
	ir := mustIR(t, cath66CompileIDL)
	if err := codegen.Generate(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("Generate (layer-1): %v", err)
	}
	if err := codegen.GenerateBindings(ir, codegen.Options{OutputDir: dir, PackageName: "gen"}); err != nil {
		t.Fatalf("GenerateBindings: %v", err)
	}

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
	// Reuse catherine's go.sum so goja (+ its deps) resolve from the module cache
	// offline; the replaced local module needs no checksum.
	if sum, err := os.ReadFile(filepath.Join(cathRoot, "go.sum")); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644)
	}

	cmd := exec.Command(goBin, "build", "./...")
	cmd.Dir = dir
	// -mod=mod lets the build pull goja's indirect deps into the temp go.mod from
	// the module cache; GOPROXY=off keeps it offline (the deps are already cached
	// because catherine itself requires goja).
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated output does not compile against jsbinding:\n%s", out)
	}
}
