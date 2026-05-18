package webidl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	syntaxIDLDir       = "../webidl2.js/test/syntax/idl"
	syntaxBaselineDir  = "../webidl2.js/test/syntax/baseline"
	invalidIDLDir      = "../webidl2.js/test/invalid/idl"
	invalidBaselineDir = "../webidl2.js/test/invalid/baseline"
)

// TestSyntaxCorpus parses every .webidl file in the syntax corpus and compares
// the AST (in webidl2.js JSON shape) against the .json baseline.
func TestSyntaxCorpus(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join(syntaxIDLDir, "*.webidl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no inputs found in %s", syntaxIDLDir)
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
			defs, err := Parse(string(src))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			got := any(ToJSONShape(defs))

			baselineFile := filepath.Join(syntaxBaselineDir, strings.TrimSuffix(name, ".webidl")+".json")
			bs, err := os.ReadFile(baselineFile)
			if err != nil {
				t.Fatal(err)
			}
			var want any
			if err := json.Unmarshal(bs, &want); err != nil {
				t.Fatalf("baseline JSON unmarshal: %v", err)
			}
			want = stripEOF(want)

			if diff := jsonDiff("", got, want); diff != "" {
				gotBytes, _ := json.MarshalIndent(got, "", "  ")
				t.Fatalf("AST mismatch:\n%s\n\nfull got JSON:\n%s", diff, gotBytes)
			}
		})
	}
}

// TestInvalidCorpus parses every .webidl file in the invalid corpus and
// asserts that parsing fails. Per project decision we do NOT compare error
// text — only that an error is produced.
func TestInvalidCorpus(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join(invalidIDLDir, "*.webidl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no inputs found in %s", invalidIDLDir)
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
			if _, err := Parse(string(src)); err == nil {
				// Some "invalid" corpus entries are only invalid per validator
				// rules (semantics), not the grammar. We don't implement the
				// validator. Skip those by checking whether the baseline error
				// text suggests a validator-only issue.
				baselineFile := filepath.Join(invalidBaselineDir, strings.TrimSuffix(name, ".webidl")+".txt")
				bs, readErr := os.ReadFile(baselineFile)
				if readErr == nil && isValidatorOnly(string(bs)) {
					t.Skipf("validator-only check: %s", strings.TrimSpace(string(bs)))
					return
				}
				t.Fatalf("expected parse error, got nil")
			}
		})
	}
}

// isValidatorOnly returns true if the baseline error text looks like a
// validator (post-parse semantic) check rather than a grammar error.
//
// Heuristic: webidl2.js emits validator errors via `validate()` which prefix
// messages with `Validation error`. Parse errors use other phrasing.
func isValidatorOnly(s string) bool {
	return strings.Contains(s, "Validation error")
}

// stripEOF removes a trailing {type:"eof", value:""} entry from a top-level array.
func stripEOF(v any) any {
	arr, ok := v.([]any)
	if !ok {
		return v
	}
	if len(arr) == 0 {
		return arr
	}
	last, ok := arr[len(arr)-1].(map[string]any)
	if !ok {
		return arr
	}
	if last["type"] == "eof" {
		return arr[:len(arr)-1]
	}
	return arr
}

// jsonDiff returns a string describing the first difference between got and
// want, or "" if they are deeply equal. Path identifies the location.
func jsonDiff(path string, got, want any) string {
	if reflect.DeepEqual(got, want) {
		return ""
	}
	switch g := got.(type) {
	case map[string]any:
		w, ok := want.(map[string]any)
		if !ok {
			return fmt.Sprintf("at %s: type mismatch — got %T, want %T\n  got:  %v\n  want: %v", path, got, want, got, want)
		}
		// keys missing in want
		for k := range g {
			if _, ok := w[k]; !ok {
				return fmt.Sprintf("at %s: extra key %q\n  got:  %v\n  want: <missing>", path, k, g[k])
			}
		}
		for k := range w {
			if _, ok := g[k]; !ok {
				return fmt.Sprintf("at %s: missing key %q\n  got:  <missing>\n  want: %v", path, k, w[k])
			}
		}
		// recurse
		for _, k := range sortedKeys(g) {
			if d := jsonDiff(path+"."+k, g[k], w[k]); d != "" {
				return d
			}
		}
	case []any:
		w, ok := want.([]any)
		if !ok {
			return fmt.Sprintf("at %s: type mismatch — got array, want %T", path, want)
		}
		if len(g) != len(w) {
			return fmt.Sprintf("at %s: array length differs — got %d, want %d\n  got:  %v\n  want: %v", path, len(g), len(w), g, w)
		}
		for i := range g {
			if d := jsonDiff(fmt.Sprintf("%s[%d]", path, i), g[i], w[i]); d != "" {
				return d
			}
		}
	default:
		return fmt.Sprintf("at %s: scalar mismatch\n  got:  %v (%T)\n  want: %v (%T)", path, got, got, want, want)
	}
	return ""
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

