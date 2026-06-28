package codegen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iansmith/webidl/typemap"
	"github.com/iansmith/webidl/webidl"
)

// Options controls what Generate produces.
type Options struct {
	// OutputDir is the directory where generated .go files are written.
	// It must already exist.
	OutputDir string

	// PackageName is the Go package declaration for generated files.
	PackageName string

	// ExposureGlobal is the JS global the binding backend targets when honoring
	// [Exposed] (CATH-65). An interface not exposed to this global gets no
	// binding and no registry entry. Empty means "Window". Ignored by the
	// layer-1 generator (Generate) — exposure is a binding-only concern.
	ExposureGlobal string
}

// exposureGlobalOrDefault returns the configured exposure global, defaulting to
// "Window" when unset.
func (o Options) exposureGlobalOrDefault() string {
	if o.ExposureGlobal == "" {
		return "Window"
	}
	return o.ExposureGlobal
}

// Generate runs the full codegen pipeline: it iterates all definitions in ir,
// dispatches each to the appropriate sub-generator, deduplicates shared
// declarations (e.g. EntryTypeDecl), and writes a single generated.go file to
// opts.OutputDir.
//
// Mixin-only interfaces are silently skipped. Definitions whose kind has no
// sub-generator emit a diagnostic warning and are omitted from output.
//
// Returns an error if ir is nil, opts.PackageName is empty, opts.OutputDir
// does not exist, or the rendered source cannot be written.
func Generate(ir *webidl.IR, opts Options) error {
	if ir == nil {
		return errors.New("codegen.Generate: ir is nil")
	}
	if opts.PackageName == "" {
		return errors.New("codegen.Generate: Options.PackageName is required")
	}
	if fi, err := os.Stat(opts.OutputDir); err != nil {
		return fmt.Errorf("codegen.Generate: OutputDir %q: %w", opts.OutputDir, err)
	} else if !fi.IsDir() {
		return fmt.Errorf("codegen.Generate: OutputDir %q: not a directory", opts.OutputDir)
	}

	tm := typemap.Mapper{}
	diag := NewDiagnostics()
	f := NewFile(opts.PackageName)

	var allDecls []Decl
	for _, def := range ir.All() {
		allDecls = append(allDecls, dispatchDef(def, tm, diag)...)
	}
	for _, d := range DedupeDecls(allDecls) {
		f.AddDecl(d)
	}

	if !diag.IsClean() {
		return fmt.Errorf("codegen.Generate: type-mapping errors:\n%s", diag.Format())
	}
	if ws := diag.Format(); ws != "" {
		fmt.Fprintf(os.Stderr, "codegen: warnings:\n%s", ws)
	}

	src, err := f.Render()
	if err != nil {
		return fmt.Errorf("codegen.Generate: render: %w", err)
	}
	outPath := filepath.Join(opts.OutputDir, "generated.go")
	if err := os.WriteFile(outPath, src, 0o644); err != nil {
		return fmt.Errorf("codegen.Generate: write %q: %w", outPath, err)
	}
	return nil
}

// dispatchDef maps a single MergedDef to zero or more Decls by switching on
// the concrete type of def.Primary.
func dispatchDef(def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics) []Decl {
	switch p := def.Primary.(type) {
	case *webidl.Enum:
		return []Decl{NewEnumDecl(p.Name, p.Values, diag)}
	case *webidl.Dictionary:
		return dispatchDict(p, def, tm, diag)
	case *webidl.Interface:
		if p.Variant == webidl.IfaceMixin {
			return nil
		}
		return NewInterfaceDecls(def, tm, diag)
	case *webidl.Typedef:
		gt, err := tm.MapType(p.IDLType)
		if err != nil {
			diag.Add("error", fmt.Sprintf("typedef %q: type mapping: %v", p.Name, err))
			return nil
		}
		return []Decl{NewTypedefDecl(p.Name, gt.String(), true, diag)}
	case *webidl.CallbackFunction:
		return dispatchCallback(p, def, tm, diag)
	case *webidl.Namespace:
		return []Decl{NewNamespaceDecl(p.Name, buildNsMethods(def.Members, tm, diag, p.Name), diag)}
	default:
		diag.Add("warning", fmt.Sprintf("unhandled definition kind %T — skipped", def.Primary))
		return nil
	}
}

func dispatchDict(p *webidl.Dictionary, def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics) []Decl {
	fields := make([]DictField, 0, len(def.Members))
	for _, m := range def.Members {
		f, ok := m.(*webidl.Field)
		if !ok {
			continue
		}
		gt, err := tm.MapType(f.IDLType)
		if err != nil {
			diag.Add("error", fmt.Sprintf("dict %q field %q: type mapping: %v", p.Name, f.Name, err))
			continue
		}
		fields = append(fields, DictField{IDLName: f.Name, GoType: gt.String(), Optional: !f.Required})
	}
	parent := ""
	if p.Inheritance != "" {
		parent = IdentSanitize(p.Inheritance)
	}
	return []Decl{NewDictDecl(p.Name, parent, fields, diag)}
}

func dispatchCallback(p *webidl.CallbackFunction, def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics) []Decl {
	raisesException := false
	for _, attr := range p.ExtAttrs {
		if attr.Name == "RaisesException" {
			raisesException = true
			break
		}
	}
	return []Decl{NewCallbackFuncDecl(
		p.Name,
		buildCallbackParams(p.Arguments, tm, diag, p.Name),
		buildReturnType(p.ReturnType, tm, diag, p.Name, ""),
		raisesException,
		diag,
	)}
}

func buildCallbackParams(args []*webidl.Argument, tm typemap.Mapper, diag *Diagnostics, ctx string) []callbackParam {
	out := make([]callbackParam, 0, len(args))
	for _, a := range args {
		gt, err := tm.MapType(a.IDLType)
		if err != nil {
			diag.Add("error", fmt.Sprintf("callback %q param %q: type mapping: %v", ctx, a.Name, err))
			continue
		}
		out = append(out, callbackParam{goType: gt.String(), variadic: a.Variadic})
	}
	return out
}

func buildNsMethods(members []webidl.Member, tm typemap.Mapper, diag *Diagnostics, ctx string) []nsMethod {
	var out []nsMethod
	for _, m := range members {
		op, ok := m.(*webidl.Operation)
		if !ok || op.Name == "" {
			continue
		}
		out = append(out, nsMethod{
			goName:     opGoName(op.Name),
			params:     buildParams(op.Arguments, tm, diag, ctx),
			returnType: buildReturnType(op.ReturnType, tm, diag, ctx, op.Name),
		})
	}
	return out
}
