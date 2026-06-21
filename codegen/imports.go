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
		if strings.Contains(p, "/") {
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
