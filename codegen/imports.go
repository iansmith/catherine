package codegen

import (
	"sort"
	"strings"
)

// renderImports is the implementation backing ImportTracker.Render.
func renderImports(paths map[string]struct{}) string {
	if len(paths) == 0 {
		return ""
	}

	var stdlib, external []string
	for p := range paths {
		// External module paths always have a dot in the first path component
		// (e.g. "github.com/foo/bar"). Stdlib paths never do (e.g. "net", "encoding").
		first, _, _ := strings.Cut(p, "/")
		if strings.Contains(first, ".") {
			external = append(external, p)
		} else {
			stdlib = append(stdlib, p)
		}
	}
	sort.Strings(stdlib)
	sort.Strings(external)

	var sb strings.Builder
	sb.WriteString("import (\n")
	for _, p := range stdlib {
		sb.WriteString("\t\"")
		sb.WriteString(p)
		sb.WriteString("\"\n")
	}
	if len(stdlib) > 0 && len(external) > 0 {
		sb.WriteString("\n")
	}
	for _, p := range external {
		sb.WriteString("\t\"")
		sb.WriteString(p)
		sb.WriteString("\"\n")
	}
	sb.WriteString(")\n")
	return sb.String()
}
