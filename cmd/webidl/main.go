// Command webidl reads Web IDL from a file or stdin and prints the parsed AST
// as pretty JSON.
//
// Usage:
//
//	webidl path/to/spec.idl    # parse a file
//	webidl                     # parse stdin
//	webidl -tree foo.idl       # human-readable tree view (not JSON)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iansmith/webidl/codegen"
	"github.com/iansmith/webidl/webidl"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "codegen" {
		runCodegen(os.Args[2:])
		return
	}

	tree := flag.Bool("tree", false, "print a human-readable tree instead of JSON")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: webidl [-tree] [path]")
		flag.PrintDefaults()
	}
	flag.Parse()

	var src []byte
	var err error
	switch flag.NArg() {
	case 0:
		src, err = io.ReadAll(os.Stdin)
	case 1:
		src, err = os.ReadFile(flag.Arg(0))
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}

	defs, err := webidl.Parse(string(src))
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(1)
	}

	if *tree {
		if err := printTree(os.Stdout, defs); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			os.Exit(1)
		}
		return
	}
	shaped := webidl.ToJSONShape(defs)
	out, err := json.MarshalIndent(shaped, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(append(out, '\n')); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
}

// errWriter wraps an io.Writer so that callers can fire-and-forget many Write
// calls and check a single Err at the end. Subsequent writes after an error
// are no-ops (cf. bufio.Writer's error-sticky behavior).
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	e.err = err
	return n, err
}

// printTree renders a compact human-readable summary of the AST. It prefers
// readability over completeness — for the full picture use the default JSON.
func printTree(out io.Writer, defs []webidl.Definition) error {
	w := &errWriter{w: out}
	for _, d := range defs {
		switch x := d.(type) {
		case *webidl.Interface:
			variant := "interface"
			switch x.Variant {
			case webidl.IfaceMixin:
				variant = "interface mixin"
			case webidl.IfaceCallback:
				variant = "callback interface"
			}
			head := fmt.Sprintf("%s %s", variant, x.Name)
			if x.Inheritance != "" {
				head += " : " + x.Inheritance
			}
			if x.Partial {
				head = "partial " + head
			}
			if ea := fmtExtAttrs(x.ExtAttrs); ea != "" {
				head = ea + " " + head
			}
			fmt.Fprintln(w, head, "{")
			for _, m := range x.Members {
				fmt.Fprintln(w, "  "+fmtMember(m))
			}
			fmt.Fprintln(w, "};")
		case *webidl.Dictionary:
			head := "dictionary " + x.Name
			if x.Inheritance != "" {
				head += " : " + x.Inheritance
			}
			if x.Partial {
				head = "partial " + head
			}
			fmt.Fprintln(w, head, "{")
			for _, f := range x.Members {
				req := ""
				if f.Required {
					req = "required "
				}
				def := ""
				if f.Default != nil {
					def = " = " + fmtConstValue(f.Default)
				}
				fmt.Fprintf(w, "  %s%s %s%s;\n", req, fmtType(f.IDLType), f.Name, def)
			}
			fmt.Fprintln(w, "};")
		case *webidl.Enum:
			fmt.Fprintf(w, "enum %s { %s };\n", x.Name, quoteAll(x.Values))
		case *webidl.Typedef:
			fmt.Fprintf(w, "typedef %s %s;\n", fmtType(x.IDLType), x.Name)
		case *webidl.Includes:
			fmt.Fprintf(w, "%s includes %s;\n", x.Target, x.Includes)
		case *webidl.Namespace:
			head := "namespace " + x.Name
			if x.Partial {
				head = "partial " + head
			}
			fmt.Fprintln(w, head, "{")
			for _, m := range x.Members {
				fmt.Fprintln(w, "  "+fmtMember(m))
			}
			fmt.Fprintln(w, "};")
		case *webidl.CallbackFunction:
			args := make([]string, 0, len(x.Arguments))
			for _, a := range x.Arguments {
				args = append(args, fmtArg(a))
			}
			fmt.Fprintf(w, "callback %s = %s (%s);\n", x.Name, fmtType(x.ReturnType), strings.Join(args, ", "))
		}
	}
	return w.err
}

func fmtMember(m webidl.Member) string {
	switch x := m.(type) {
	case *webidl.Attribute:
		ro := ""
		if x.Readonly {
			ro = "readonly "
		}
		sp := ""
		if x.Special != "" {
			sp = x.Special + " "
		}
		return fmt.Sprintf("%s%sattribute %s %s;", sp, ro, fmtType(x.IDLType), x.Name)
	case *webidl.Operation:
		args := make([]string, 0, len(x.Arguments))
		for _, a := range x.Arguments {
			args = append(args, fmtArg(a))
		}
		sp := ""
		if x.Special != "" {
			sp = x.Special + " "
		}
		ret := ""
		if x.ReturnType != nil {
			ret = fmtType(x.ReturnType) + " "
		}
		return fmt.Sprintf("%s%s%s(%s);", sp, ret, x.Name, strings.Join(args, ", "))
	case *webidl.Constant:
		return fmt.Sprintf("const %s %s = %s;", fmtType(x.IDLType), x.Name, fmtConstValue(x.Value))
	case *webidl.Constructor:
		args := make([]string, 0, len(x.Arguments))
		for _, a := range x.Arguments {
			args = append(args, fmtArg(a))
		}
		return fmt.Sprintf("constructor(%s);", strings.Join(args, ", "))
	case *webidl.IterableLike:
		types := make([]string, 0, len(x.Types))
		for _, t := range x.Types {
			types = append(types, fmtType(t))
		}
		ro := ""
		if x.Readonly {
			ro = "readonly "
		}
		return fmt.Sprintf("%s%s<%s>;", ro, x.Kind, strings.Join(types, ", "))
	}
	return "?"
}

func fmtArg(a *webidl.Argument) string {
	opt := ""
	if a.Optional {
		opt = "optional "
	}
	v := ""
	if a.Variadic {
		v = "..."
	}
	def := ""
	if a.Default != nil {
		def = " = " + fmtConstValue(a.Default)
	}
	return fmt.Sprintf("%s%s%s %s%s", opt, fmtType(a.IDLType), v, a.Name, def)
}

func fmtType(t *webidl.IDLType) string {
	if t == nil {
		return "?"
	}
	var s string
	switch {
	case t.Union:
		parts := make([]string, 0, len(t.Subtypes))
		for _, sub := range t.Subtypes {
			parts = append(parts, fmtType(sub))
		}
		s = "(" + strings.Join(parts, " or ") + ")"
	case t.Generic != "":
		parts := make([]string, 0, len(t.Subtypes))
		for _, sub := range t.Subtypes {
			parts = append(parts, fmtType(sub))
		}
		s = fmt.Sprintf("%s<%s>", t.Generic, strings.Join(parts, ", "))
	default:
		s = t.Base
	}
	if t.Nullable {
		s += "?"
	}
	return s
}

func fmtConstValue(v *webidl.ConstValue) string {
	if v == nil {
		return "null"
	}
	switch v.Kind {
	case webidl.CVString:
		return fmt.Sprintf("%q", v.String)
	case webidl.CVNumber:
		return v.Number
	case webidl.CVBoolean:
		if v.Bool {
			return "true"
		}
		return "false"
	case webidl.CVNull:
		return "null"
	case webidl.CVInfinity:
		if v.Negative {
			return "-Infinity"
		}
		return "Infinity"
	case webidl.CVNaN:
		return "NaN"
	case webidl.CVSequence:
		return "[]"
	case webidl.CVDictionary:
		return "{}"
	}
	return "?"
}

func fmtExtAttrs(eas []*webidl.ExtAttr) string {
	if len(eas) == 0 {
		return ""
	}
	parts := make([]string, 0, len(eas))
	for _, ea := range eas {
		switch {
		case ea.RHS == nil && len(ea.Arguments) == 0:
			parts = append(parts, ea.Name)
		case ea.RHS != nil && !ea.RHS.IsList:
			parts = append(parts, ea.Name+"="+ea.RHS.Value)
		case ea.RHS != nil && ea.RHS.IsList:
			vals := make([]string, 0, len(ea.RHS.Items))
			for _, it := range ea.RHS.Items {
				vals = append(vals, it.Value)
			}
			parts = append(parts, fmt.Sprintf("%s=(%s)", ea.Name, strings.Join(vals, ", ")))
		default:
			parts = append(parts, ea.Name+"(...)")
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func quoteAll(ss []string) string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, fmt.Sprintf("%q", s))
	}
	return strings.Join(out, ", ")
}

func runCodegen(args []string) {
	fs := flag.NewFlagSet("codegen", flag.ExitOnError)
	outDir := fs.String("o", ".", "output directory for generated .go files")
	pkgName := fs.String("pkg", "", "Go package name for generated files (required)")
	bindings := fs.Bool("bindings", false, "also emit the goja bindings (bindings.go)")
	exposed := fs.String("exposed", "", "binding exposure global (default Window; -bindings only)")
	rtPath := fs.String("rt", "", "runtime shim import path (default in-repo jsbinding; -bindings only)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: webidl codegen -pkg name [-o dir] [-bindings [-exposed global] [-rt import/path]] file.idl ...")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *pkgName == "" {
		fmt.Fprintln(os.Stderr, "codegen: -pkg is required")
		fs.Usage()
		os.Exit(2)
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "codegen: at least one .idl file is required")
		fs.Usage()
		os.Exit(2)
	}

	var allDefs []webidl.Definition
	for _, path := range fs.Args() {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read:", err)
			os.Exit(1)
		}
		defs, err := webidl.Parse(string(src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", path, err)
			os.Exit(1)
		}
		allDefs = append(allDefs, defs...)
	}

	ir, errs := webidl.Merge(allDefs)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "merge:", e)
		}
		os.Exit(1)
	}

	if err := codegen.Generate(ir, codegen.Options{
		OutputDir:   *outDir,
		PackageName: *pkgName,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "codegen:", err)
		os.Exit(1)
	}

	if *bindings {
		if err := codegen.GenerateBindings(ir, codegen.Options{
			OutputDir:         *outDir,
			PackageName:       *pkgName,
			ExposureGlobal:    *exposed, // "" → codegen defaults to Window
			RuntimeImportPath: *rtPath,  // "" → codegen defaults to the in-repo jsbinding
		}); err != nil {
			fmt.Fprintln(os.Stderr, "codegen bindings:", err)
			os.Exit(1)
		}
	} else if *exposed != "" || *rtPath != "" {
		fmt.Fprintln(os.Stderr, "warning: -exposed/-rt ignored without -bindings")
	}
}
