package codegen

import (
	"errors"
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

	// Emit declaration sources.
	for _, d := range f.decls {
		sb.WriteString("\n")
		sb.WriteString(d.declSource())
		sb.WriteString("\n")
	}

	return format.Source([]byte(sb.String()))
}
