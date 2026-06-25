package codegen_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
	"github.com/iansmith/webidl/webidl"
)

func goParseSource(src []byte) (*token.FileSet, error) {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "", src, 0)
	return fset, err
}

// mustIR parses and merges an IDL string, failing the test on error.
func mustIR(t *testing.T, src string) *webidl.IR {
	t.Helper()
	defs, err := webidl.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ir, errs := webidl.Merge(defs)
	if len(errs) > 0 {
		t.Fatalf("Merge: %v", errs)
	}
	return ir
}

// readDir collects all .go files from a directory and returns their combined content.
func readDir(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") {
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			sb.Write(b)
		}
	}
	return sb.String()
}

// --- Edge / boundary ---

func TestGenerate_nilIR(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := codegen.Generate(nil, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	})
	if err == nil {
		t.Error("Generate(nil, ...) should return an error")
	}
}

func TestGenerate_emptyPackageName(t *testing.T) {
	t.Parallel()
	ir, _ := webidl.Merge(nil)
	dir := t.TempDir()
	err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "",
	})
	if err == nil {
		t.Error("Generate with empty PackageName should return an error")
	}
}

func TestGenerate_emptyIR_noError(t *testing.T) {
	t.Parallel()
	ir, _ := webidl.Merge(nil)
	dir := t.TempDir()
	err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	})
	if err != nil {
		t.Errorf("Generate with empty IR: unexpected error: %v", err)
	}
}

// --- Error / rejection ---

func TestGenerate_invalidOutputDir(t *testing.T) {
	t.Parallel()
	ir, _ := webidl.Merge(nil)
	err := codegen.Generate(ir, codegen.Options{
		OutputDir:   "/nonexistent/path/that/cannot/exist/for/cath42",
		PackageName: "testpkg",
	})
	if err == nil {
		t.Error("Generate with non-existent OutputDir should return an error")
	}
}

// --- Cross-feature interaction ---

func TestGenerate_enumAndDict(t *testing.T) {
	t.Parallel()
	ir := mustIR(t, `
		enum Color { "red", "green", "blue" };
		dictionary Point { required long x; required long y; };
	`)
	dir := t.TempDir()
	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := readDir(t, dir)
	if !strings.Contains(got, "Color") {
		t.Errorf("output missing 'Color'")
	}
	if !strings.Contains(got, "Point") {
		t.Errorf("output missing 'Point'")
	}
}

func TestGenerate_packageNameInOutput(t *testing.T) {
	t.Parallel()
	ir := mustIR(t, `enum X { "a" };`)
	dir := t.TempDir()
	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "mypkg",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := readDir(t, dir)
	if !strings.Contains(got, "package mypkg") {
		t.Errorf("output does not contain 'package mypkg':\n%s", got)
	}
}

// --- Happy path ---

func TestGenerate_enumProducesGoFile(t *testing.T) {
	t.Parallel()
	ir := mustIR(t, `enum Direction { "north", "south", "east", "west" };`)
	dir := t.TempDir()
	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var goFiles int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") {
			goFiles++
		}
	}
	if goFiles == 0 {
		t.Error("Generate produced no .go files")
	}
}

func TestGenerate_enumOutputIsValidGo(t *testing.T) {
	t.Parallel()
	ir := mustIR(t, `enum Direction { "north", "south", "east", "west" };`)
	dir := t.TempDir()
	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := readDir(t, dir)
	// Must contain type definition and consts.
	if !strings.Contains(got, "Direction") {
		t.Errorf("output missing 'Direction'")
	}
	// Must be valid Go (package decl present).
	if !strings.Contains(got, "package testpkg") {
		t.Errorf("output missing package declaration")
	}
}

func TestGenerate_multipleDefsAllAppear(t *testing.T) {
	t.Parallel()
	ir := mustIR(t, `
		enum Color { "red", "green" };
		enum Size { "small", "large" };
		enum Shape { "circle", "square" };
	`)
	dir := t.TempDir()
	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := readDir(t, dir)
	for _, want := range []string{"Color", "Size", "Shape"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

// --- Adversary gap tests ---

func TestGenerate_mixinNotInOutput(t *testing.T) {
	t.Parallel()
	// Mixin interfaces must not appear as output types — they are implementation
	// detail consumed during IR merge.
	ir := mustIR(t, `
		interface mixin Mixable {
			undefined doSomething();
		};
		interface MyInterface {};
		MyInterface includes Mixable;
	`)
	dir := t.TempDir()
	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := readDir(t, dir)
	if strings.Contains(got, "Mixable") {
		t.Errorf("mixin 'Mixable' appeared in generated output; it should be filtered out:\n%s", got)
	}
	// The regular interface should still be emitted.
	if !strings.Contains(got, "MyInterface") {
		t.Errorf("regular interface 'MyInterface' missing from output")
	}
}

func TestGenerate_outputParsesAsValidGo(t *testing.T) {
	t.Parallel()
	ir := mustIR(t, `enum Color { "red", "green", "blue" };`)
	dir := t.TempDir()
	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var checked int
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		checked++
		p := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := goParseSource(src); err != nil {
			t.Errorf("file %s does not parse as valid Go: %v\n---\n%s", e.Name(), err, src)
		}
	}
	if checked == 0 {
		t.Error("no .go files written — nothing to validate")
	}
}

func TestGenerate_interfaceProducesOutput(t *testing.T) {
	t.Parallel()
	ir := mustIR(t, `
		interface EventTarget {
			undefined addEventListener(DOMString type, EventListener? callback);
		};
		callback interface EventListener {
			undefined handleEvent(Event event);
		};
		interface Event {};
	`)
	dir := t.TempDir()
	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   dir,
		PackageName: "testpkg",
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := readDir(t, dir)
	if got == "" {
		t.Error("Generate produced no output for interface definitions")
	}
	if !strings.Contains(got, "EventTarget") {
		t.Errorf("output missing 'EventTarget'")
	}
}
