package codegen

import (
	"sort"
	"strings"
)

// renderImports is the implementation backing ImportTracker.Render. aliases maps
// a path to an explicit import alias (e.g. "rt"); paths absent from it import
// under their natural package name.
func renderImports(paths map[string]struct{}, aliases map[string]string) string {
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
	emit := func(p string) {
		sb.WriteString("\t")
		if a := aliases[p]; a != "" {
			sb.WriteString(a)
			sb.WriteString(" ")
		}
		sb.WriteString("\"")
		sb.WriteString(p)
		sb.WriteString("\"\n")
	}
	for _, p := range stdlib {
		emit(p)
	}
	if len(stdlib) > 0 && len(external) > 0 {
		sb.WriteString("\n")
	}
	for _, p := range external {
		emit(p)
	}
	sb.WriteString(")\n")
	return sb.String()
}
