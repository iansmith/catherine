package codegen_test

// CATH-66 Phase-0 red tests: the generated bindings must be RE-QUALIFIED against
// the jsbinding runtime package (rt.*), object args/returns must route through
// Unwrap/Wrap, and a Register entrypoint must be generated. These assert on the
// generated SOURCE and fail on the current (unqualified, Register-less) output.

import (
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
	if !strings.Contains(src, "b.ctx.Wrap(") {
		t.Errorf("object-typed return must go through the cached Wrap\n%s", src)
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
