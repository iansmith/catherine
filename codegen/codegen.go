// Package codegen provides the shared infrastructure for generating valid,
// gofmt-canonical Go source files from WebIDL definitions.
package codegen

import "strings"

// Diagnostic is a single structured message from the code-generation pipeline.
type Diagnostic struct {
	Severity string // "error" or "warning"
	Message  string
}

// Diagnostics collects structured messages during code generation. Errors make
// the pipeline dirty; warnings are informational and do not.
type Diagnostics struct {
	entries []Diagnostic
}

// NewDiagnostics returns a clean, empty Diagnostics collector.
func NewDiagnostics() *Diagnostics {
	return &Diagnostics{}
}

// Add records a diagnostic message. severity should be "error" or "warning".
func (d *Diagnostics) Add(severity, message string) {
	d.entries = append(d.entries, Diagnostic{Severity: severity, Message: message})
}

// IsClean reports whether no error-severity diagnostics have been recorded.
// Warnings do not affect cleanliness.
func (d *Diagnostics) IsClean() bool {
	for _, e := range d.entries {
		if e.Severity == "error" {
			return false
		}
	}
	return true
}

// Errors returns all error-severity diagnostics in insertion order.
func (d *Diagnostics) Errors() []Diagnostic {
	var out []Diagnostic
	for _, e := range d.entries {
		if e.Severity == "error" {
			out = append(out, e)
		}
	}
	return out
}

// Format returns a human-readable summary of all diagnostics, or "" if there
// are none.
func (d *Diagnostics) Format() string {
	if len(d.entries) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, e := range d.entries {
		sb.WriteString(e.Severity)
		sb.WriteString(": ")
		sb.WriteString(e.Message)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ImportTracker collects import paths and renders a grouped, sorted import
// block. Stdlib imports appear before external imports; duplicates are
// deduplicated automatically.
type ImportTracker struct {
	paths map[string]struct{}
}

// NewImportTracker returns an empty ImportTracker.
func NewImportTracker() *ImportTracker {
	return &ImportTracker{paths: make(map[string]struct{})}
}

// Add registers an import path. Calling Add with the same path more than once
// is a no-op.
func (t *ImportTracker) Add(path string) {
	t.paths[path] = struct{}{}
}

// Render returns a formatted import block, or "" if no imports have been
// registered.
func (t *ImportTracker) Render() string {
	// Implemented in imports.go
	return renderImports(t.paths)
}

// Decl is implemented by all declaration node types (ConstGroup, TypeAlias,
// Struct, Interface, etc.). Concrete Decl types are added in later tickets.
type Decl interface {
	declSource() string
	// declName returns the primary Go identifier declared by this node (e.g.
	// the type name for an EnumDecl). File.Render uses it to detect duplicate
	// declarations before passing source to go/format.
	declName() string
}

// File is the root of the CodeNode tree. It holds a package name, an optional
// set of imports, and an ordered list of declarations.
type File struct {
	pkgName string
	imports *ImportTracker
	decls   []Decl
}

// NewFile returns a File for the given package name. pkgName must be a valid
// Go identifier; Render returns an error if it is empty.
func NewFile(pkgName string) *File {
	return &File{pkgName: pkgName}
}

// SetImports attaches an ImportTracker to the file. A second call replaces the
// first; imports are never accumulated across multiple SetImports calls.
func (f *File) SetImports(tr *ImportTracker) {
	f.imports = tr
}

// AddDecl appends a declaration node to the file's ordered list.
func (f *File) AddDecl(d Decl) {
	f.decls = append(f.decls, d)
}

// Render assembles the file's source and returns gofmt-canonical bytes.
// Returns an error if the package name is empty or if gofmt rejects the
// assembled source.
func (f *File) Render() ([]byte, error) {
	// Implemented in file.go
	return renderFile(f)
}
