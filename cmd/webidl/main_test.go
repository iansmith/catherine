package main

// CATH-79 Phase 0: red tests — the `webidl codegen` CLI must be able to emit the
// goja bindings (layer-2), not just the layer-1 interfaces, via a -bindings flag,
// with -rt threading the runtime import path through to Options.RuntimeImportPath.
//
// Black-box: these exec the real CLI (`go run .`) exactly as a user would. They
// fail on current code because -bindings / -rt are undefined flags, so the CLI
// exits non-zero and writes no bindings.go.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const cath79ThingIDL = `[Exposed=Window]
interface Thing {
  constructor(DOMString name);
  attribute DOMString name;
  boolean check(DOMString x);
};
`

// runCLI execs the CLI (`go run .` in this package dir) with the given args and
// returns combined output + the process error (non-nil on non-zero exit).
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found; skipping CLI exec")
	}
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// writeIDL drops the fixture into a temp dir and returns (dir, idlPath). The dir
// doubles as the -o output dir (Generate requires it to already exist).
func writeIDL(t *testing.T, idl string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "thing.idl")
	if err := os.WriteFile(p, []byte(idl), 0o644); err != nil {
		t.Fatalf("write idl: %v", err)
	}
	return dir, p
}

// TestCodegenCLI_Bindings_EmitsBindingsFile asserts `-bindings` makes the CLI
// also emit bindings.go with a generated *Binding type and a Register entrypoint.
func TestCodegenCLI_Bindings_EmitsBindingsFile(t *testing.T) {
	dir, idl := writeIDL(t, cath79ThingIDL)
	out, err := runCLI(t, "codegen", "-bindings", "-pkg", "gen", "-o", dir, idl)
	if err != nil {
		t.Fatalf("CLI codegen -bindings failed: %v\n%s", err, out)
	}
	b, rerr := os.ReadFile(filepath.Join(dir, "bindings.go"))
	if rerr != nil {
		t.Fatalf("bindings.go was not written: %v\noutput:\n%s", rerr, out)
	}
	src := string(b)
	if !strings.Contains(src, "ThingBinding") {
		t.Fatalf("bindings.go missing generated ThingBinding type:\n%s", src)
	}
	if !strings.Contains(src, "func Register(") {
		t.Fatalf("bindings.go missing Register entrypoint:\n%s", src)
	}
}

// TestCodegenCLI_Bindings_RuntimeImportPath asserts `-rt` threads through to
// Options.RuntimeImportPath, changing the runtime import the bindings reference.
func TestCodegenCLI_Bindings_RuntimeImportPath(t *testing.T) {
	dir, idl := writeIDL(t, cath79ThingIDL)
	const rtPath = "example.com/custom/shim"
	out, err := runCLI(t, "codegen", "-bindings", "-rt", rtPath, "-pkg", "gen", "-o", dir, idl)
	if err != nil {
		t.Fatalf("CLI codegen -bindings -rt failed: %v\n%s", err, out)
	}
	b, rerr := os.ReadFile(filepath.Join(dir, "bindings.go"))
	if rerr != nil {
		t.Fatalf("bindings.go was not written: %v\noutput:\n%s", rerr, out)
	}
	if !strings.Contains(string(b), rtPath) {
		t.Fatalf("-rt %q not threaded into bindings.go runtime import:\n%s", rtPath, b)
	}
}

// TestCodegenCLI_Bindings_ExposedFilters asserts `-exposed` threads to
// Options.ExposureGlobal: a Window-only interface built with -exposed Worker is
// not exposed, so no ThingBinding is emitted.
func TestCodegenCLI_Bindings_ExposedFilters(t *testing.T) {
	dir, idl := writeIDL(t, cath79ThingIDL)
	out, err := runCLI(t, "codegen", "-bindings", "-exposed", "Worker", "-pkg", "gen", "-o", dir, idl)
	if err != nil {
		t.Fatalf("CLI codegen -bindings -exposed failed: %v\n%s", err, out)
	}
	// bindings.go may or may not be written when nothing is exposed; either way
	// it must NOT contain a binding for the Window-only Thing.
	if b, rerr := os.ReadFile(filepath.Join(dir, "bindings.go")); rerr == nil {
		if strings.Contains(string(b), "ThingBinding") {
			t.Fatalf("-exposed Worker must exclude the Window-only Thing, but ThingBinding was emitted:\n%s", b)
		}
	}
}

// TestCodegenCLI_ExposedWithoutBindings_WarnsNotErrors asserts the binding-only
// flags are tolerated (warn, don't error) when -bindings is absent: layer-1 is
// still generated and the CLI exits 0.
func TestCodegenCLI_ExposedWithoutBindings_WarnsNotErrors(t *testing.T) {
	dir, idl := writeIDL(t, cath79ThingIDL)
	out, err := runCLI(t, "codegen", "-exposed", "Worker", "-pkg", "gen", "-o", dir, idl)
	if err != nil {
		t.Fatalf("-exposed without -bindings must warn, not error: %v\n%s", err, out)
	}
	if _, rerr := os.ReadFile(filepath.Join(dir, "generated.go")); rerr != nil {
		t.Fatalf("layer-1 generated.go must still be written: %v\noutput:\n%s", rerr, out)
	}
}
