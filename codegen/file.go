package codegen

import (
	"errors"
	"fmt"
	"go/format"
	"strings"
)

// renderFile is the implementation backing File.Render.
func renderFile(f *File) ([]byte, error) {
	if f.pkgName == "" {
		return nil, errors.New("codegen: File.Render: package name must not be empty")
	}

	var sb strings.Builder
	sb.WriteString("package ")
	sb.WriteString(f.pkgName)
	sb.WriteString("\n")

	// Emit import block only when the tracker has at least one path.
	if f.imports != nil {
		block := f.imports.Render()
		if block != "" {
			sb.WriteString("\n")
			sb.WriteString(block)
		}
	}

	// Detect duplicate declared names before handing source to go/format.
	// format.Source is syntax-only; duplicate type/func declarations are a
	// type error that only go build would catch, producing an opaque message.
	seenNames := make(map[string]bool)
	for _, d := range f.decls {
		name := d.declName()
		if seenNames[name] {
			return nil, fmt.Errorf("codegen: File.Render: duplicate declaration %q in package %q", name, f.pkgName)
		}
		seenNames[name] = true
	}

	// Emit declaration sources.
	for _, d := range f.decls {
		sb.WriteString("\n")
		sb.WriteString(d.declSource())
		sb.WriteString("\n")
	}

	return format.Source([]byte(sb.String()))
}
