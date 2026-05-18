package webidl

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// knownBadWebRefSpecs lists files in webref/ed/idl/ that are not valid
// modern Web IDL. webidl2.js (the reference parser) also rejects them with
// the same error messages we produce. They're carried in webref for historical
// completeness — both files predate modern Web IDL syntax.
//
//   - DOM-Style.idl: uses DOM Level 2 syntax (`in T name`, `raises(...)`).
//   - svg-paths.idl: declares interface members without the `attribute` keyword
//     and omits the `;` after a dictionary body.
var knownBadWebRefSpecs = map[string]string{
	"DOM-Style.idl": "DOM Level 2 syntax (in, raises) — rejected by webidl2.js as well",
	"svg-paths.idl": "non-modern syntax; rejected by webidl2.js as well",
}

// TestWebRefCorpus runs the parser against every shipping web spec's IDL
// (~334 files from w3c/webref) as an integration check. Each file must parse
// without error. We do not compare ASTs here — webref doesn't ship per-spec
// JSON baselines in a form that matches webidl2.js's output, and the goal is
// just to confirm the parser handles real-world IDL.
func TestWebRefCorpus(t *testing.T) {
	t.Parallel()
	const dir = "../webref/ed/idl"
	files, err := filepath.Glob(filepath.Join(dir, "*.idl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("webref corpus not present; skipping")
	}
	sort.Strings(files)
	for _, in := range files {
		name := filepath.Base(in)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Parse(string(src))
			if reason, bad := knownBadWebRefSpecs[name]; bad {
				if err == nil {
					t.Fatalf("expected parse error (webidl2.js rejects this too: %s)", reason)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
		})
	}
}
