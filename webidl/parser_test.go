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
			defs, parseErr := Parse(string(src))
			if parseErr != nil {
				return // grammar-level error: pass
			}
			// Parse succeeded: check whether this is a validator-only case.
			baselineFile := filepath.Join(invalidBaselineDir, strings.TrimSuffix(name, ".webidl")+".txt")
			bs, readErr := os.ReadFile(baselineFile)
			if readErr == nil && isValidatorOnly(string(bs)) {
				rule := ruleFromBaseline(string(bs))
				if rule == "" {
					t.Fatalf("unrecognised baseline format in %s: %s", name, strings.TrimSpace(string(bs)))
				}
				if !implementedRules[rule] {
					t.Skipf("validator rule %q not yet implemented: %s", rule, strings.TrimSpace(string(bs)))
					return
				}
				errs := Validate(defs)
				if len(errs) == 0 {
					t.Fatalf("expected validation error for rule %q, got none", rule)
				}
				var found bool
				for _, e := range errs {
					if ve, ok := e.(*ValidationError); ok && ve.Rule == rule {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("no error matched rule %q (got: %v)", rule, errs)
				}
				return
			}
			if readErr != nil {
				t.Fatalf("no baseline file for %s (%v)", name, readErr)
			}
			t.Fatalf("expected parse error for %s, got nil", name)
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

// ruleFromBaseline extracts the rule name from the first line of a validation
// baseline. The format is "(rule-name) Validation error ...".
func ruleFromBaseline(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '(' {
		return ""
	}
	end := strings.IndexByte(s, ')')
	if end < 0 {
		return ""
	}
	return s[1:end]
}

// implementedRules is the set of validator rules this implementation handles.
// Add the rule name here when landing each CATH-2 sub-ticket.
var implementedRules = map[string]bool{
	"no-duplicate":                   true,
	"no-cross-overload":              true,
	"constructor-member":             true,
	"incomplete-op":                  true,
	"attr-invalid-type":              true, // CATH-7
	"no-nullable-union-dict":         true, // CATH-7
	"async-sequence-idl-to-js":       true, // CATH-7
	"dict-arg-default":               true, // CATH-8
	"dict-arg-optional":              true, // CATH-8
	"no-nullable-dict-arg":           true, // CATH-8
	"require-exposed":                true, // CATH-9
	"no-constructible-global":        true, // CATH-9
	"renamed-legacy":                 true, // CATH-9
	"migrate-allowshared":            true, // CATH-9
	"replace-void":                   true, // CATH-9
	"obsolete-async-iterable-syntax": true, // CATH-9
	"overload-not-distinguishable":   true, // CATH-28
}

// TestValidateAsyncSequenceIdlToJs tests that async_sequence types cannot be
// used as operation return types or as callback arguments (the
// async-sequence-idl-to-js rule). These cases are not covered by the corpus
// test because async-sequence-idl-to-js.webidl's baseline starts with
// attr-invalid-type as the first rule.
func TestValidateAsyncSequenceIdlToJs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "operation return type",
			src: `[Exposed=Window]
interface I {
  async_sequence<DOMString> foo();
};`,
		},
		{
			name: "callback argument",
			src:  `callback CB = boolean (async_sequence<DOMString> arg);`,
		},
		{
			name: "operation argument",
			src: `[Exposed=Window]
interface I {
  Promise<undefined> f(async_sequence<DOMString> arg);
};`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defs, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			errs := Validate(defs)
			var found bool
			for _, e := range errs {
				if ve, ok := e.(*ValidationError); ok && ve.Rule == "async-sequence-idl-to-js" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected a ValidationError with rule %q; got: %v", "async-sequence-idl-to-js", errs)
			}
		})
	}
}

// TestValidateNullableUnionDictGaps covers three spec-compliance gaps found in
// code review of CATH-7: multi-hop typedef resolution, async-iterable argument
// types, and constructor argument types.
func TestValidateNullableUnionDictGaps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
	}{
		{
			// Finding 1: validateNullableUnionDict only follows one typedef hop.
			// A two-hop chain (V → U → union) must still fire no-nullable-union-dict.
			name: "multi-hop typedef chain",
			src: `dictionary Dict { long x; };
typedef (boolean or Dict) U;
typedef U V;
typedef V? ChainedNullable;`,
		},
		{
			// Finding 2: IterableLike.Arguments never checked.
			// An async iterable whose argument has a nullable union containing a
			// dictionary must fire no-nullable-union-dict.
			name: "async iterable argument",
			src: `dictionary Dict { long x; };
[Exposed=Window]
interface I {
  async iterable<long>(optional (Dict or boolean)? bufferSize);
};`,
		},
		{
			// Finding 3: *Constructor has no validateMember, so constructor
			// argument types are never validated.
			name: "constructor argument",
			src: `dictionary Dict { long x; };
[Exposed=Window]
interface I {
  constructor((Dict or boolean)? arg);
};`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defs, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			errs := Validate(defs)
			var found bool
			for _, e := range errs {
				if ve, ok := e.(*ValidationError); ok && ve.Rule == "no-nullable-union-dict" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected a ValidationError with rule %q; got: %v", "no-nullable-union-dict", errs)
			}
		})
	}
}

// TestValidateDictArgRules covers the three dictionary-argument constraint rules
// (CATH-8): dict-arg-default, dict-arg-optional, no-nullable-dict-arg.
func TestValidateDictArgRules(t *testing.T) {
	t.Parallel()

	mustFire := func(t *testing.T, rule, src string) {
		t.Helper()
		defs, err := Parse(src)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		errs := Validate(defs)
		for _, e := range errs {
			if ve, ok := e.(*ValidationError); ok && ve.Rule == rule {
				return
			}
		}
		t.Errorf("expected a ValidationError with rule %q; got: %v", rule, errs)
	}

	mustNotFire := func(t *testing.T, rule, src string) {
		t.Helper()
		defs, err := Parse(src)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		errs := Validate(defs)
		for _, e := range errs {
			if ve, ok := e.(*ValidationError); ok && ve.Rule == rule {
				t.Errorf("unexpected ValidationError with rule %q; got: %v", rule, errs)
				return
			}
		}
	}

	// dict-arg-default: optional argument whose type includes a dictionary must
	// have a default of {}.  An optional dict arg with no default fires.
	t.Run("dict-arg-default/missing default fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "dict-arg-default", `
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(optional D d);
};`)
	})
	t.Run("dict-arg-default/empty-brace default OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "dict-arg-default", `
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(optional D d = {});
};`)
	})
	t.Run("dict-arg-default/union missing default fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "dict-arg-default", `
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(optional (boolean or D) u);
};`)
	})

	// dict-arg-optional: non-optional argument whose type includes a dictionary
	// with no required fields must be optional.
	t.Run("dict-arg-optional/non-optional fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "dict-arg-optional", `
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(D d);
};`)
	})
	t.Run("dict-arg-optional/required-field dict exempt", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "dict-arg-optional", `
dictionary D { required long x; };
[Exposed=Window]
interface I {
  undefined op(D d);
};`)
	})
	t.Run("dict-arg-optional/subsequent required arg exempts", func(t *testing.T) {
		t.Parallel()
		// op9 pattern: Optional notLast, DOMString yay — should NOT fire because
		// DOMString yay is a required arg that comes after.
		mustNotFire(t, "dict-arg-optional", `
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(D d, DOMString s);
};`)
	})

	// no-nullable-dict-arg: argument whose type is a nullable dictionary fires.
	// Regression: nullable typedef aliases (TD? where TD→Dict) must fire
	// no-nullable-dict-arg but must NOT also fire dict-arg-optional. The two
	// rules should never co-fire — the early-return in validateArgDictRules
	// ensures this (see CATH-8 code review finding).
	t.Run("no-nullable-dict-arg/nullable typedef alias fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "no-nullable-dict-arg", `
typedef D TD;
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(TD? d);
};`)
	})
	t.Run("dict-arg-optional/nullable typedef alias exempt", func(t *testing.T) {
		t.Parallel()
		// dict-arg-optional must NOT fire alongside no-nullable-dict-arg.
		mustNotFire(t, "dict-arg-optional", `
typedef D TD;
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(TD? d);
};`)
	})
	t.Run("no-nullable-dict-arg/nullable dict fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "no-nullable-dict-arg", `
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(optional D? d = {});
};`)
	})
	t.Run("no-nullable-dict-arg/non-nullable dict OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "no-nullable-dict-arg", `
dictionary D {};
[Exposed=Window]
interface I {
  undefined op(optional D d = {});
};`)
	})
	t.Run("no-nullable-dict-arg/required nullable dict fires", func(t *testing.T) {
		t.Parallel()
		// Nullable can fire even on non-optional required arg.
		mustFire(t, "no-nullable-dict-arg", `
dictionary D { required long x; };
[Exposed=Window]
interface I {
  undefined op(D? d);
};`)
	})
}

// TestValidateExtAttrRules covers the six extended-attribute and deprecation
// rules introduced by CATH-9: require-exposed, no-constructible-global,
// renamed-legacy, migrate-allowshared, replace-void, and
// obsolete-async-iterable-syntax.
func TestValidateExtAttrRules(t *testing.T) {
	t.Parallel()

	mustFire := func(t *testing.T, rule, src string) {
		t.Helper()
		defs, err := Parse(src)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		errs := Validate(defs)
		for _, e := range errs {
			if ve, ok := e.(*ValidationError); ok && ve.Rule == rule {
				return
			}
		}
		t.Errorf("expected a ValidationError with rule %q; got: %v", rule, errs)
	}

	mustNotFire := func(t *testing.T, rule, src string) {
		t.Helper()
		defs, err := Parse(src)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		errs := Validate(defs)
		for _, e := range errs {
			if ve, ok := e.(*ValidationError); ok && ve.Rule == rule {
				t.Errorf("unexpected ValidationError with rule %q; got: %v", rule, errs)
				return
			}
		}
	}

	// ── require-exposed ────────────────────────────────────────────────────
	// Rule: every non-partial interface and namespace must carry [Exposed].

	t.Run("require-exposed/interface without Exposed fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "require-exposed", `
interface Unexposed {};`)
	})
	t.Run("require-exposed/interface with Exposed OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "require-exposed", `
[Exposed=Window]
interface Exposed {};`)
	})
	t.Run("require-exposed/namespace without Exposed fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "require-exposed", `
namespace UnexposedNS {};`)
	})
	t.Run("require-exposed/namespace with Exposed OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "require-exposed", `
[Exposed=Window]
namespace ExposedNS {};`)
	})
	t.Run("require-exposed/partial interface exempt", func(t *testing.T) {
		t.Parallel()
		// The partial declaration itself must not fire; only the canonical one does.
		mustNotFire(t, "require-exposed", `
[Exposed=Window]
interface Base {};
partial interface Base { undefined op(); };`)
	})

	// ── no-constructible-global ────────────────────────────────────────────
	// Rule: [Global] interfaces cannot have constructor() or LegacyFactoryFunction.

	t.Run("no-constructible-global/Global interface with constructor fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "no-constructible-global", `
[Global, Exposed=Window]
interface G {
  constructor();
};`)
	})
	t.Run("no-constructible-global/Global interface with LegacyFactoryFunction fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "no-constructible-global", `
[Global, Exposed=Window, LegacyFactoryFunction=G()]
interface G {};`)
	})
	t.Run("no-constructible-global/non-Global interface with constructor OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "no-constructible-global", `
[Exposed=Window]
interface NotGlobal {
  constructor();
};`)
	})

	// ── renamed-legacy ─────────────────────────────────────────────────────
	// Rule: deprecated extended-attribute names must be replaced with their
	// [Legacy…] equivalents.

	t.Run("renamed-legacy/NoInterfaceObject fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "renamed-legacy", `
[Exposed=Window, NoInterfaceObject]
interface I {};`)
	})
	t.Run("renamed-legacy/LegacyNoInterfaceObject OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "renamed-legacy", `
[Exposed=Window, LegacyNoInterfaceObject]
interface I {};`)
	})
	t.Run("renamed-legacy/Unforgeable member fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "renamed-legacy", `
[Exposed=Window]
interface I {
  [Unforgeable] readonly attribute DOMString x;
};`)
	})
	t.Run("renamed-legacy/LegacyUnforgeable member OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "renamed-legacy", `
[Exposed=Window]
interface I {
  [LegacyUnforgeable] readonly attribute DOMString x;
};`)
	})

	// ── migrate-allowshared ────────────────────────────────────────────────
	// Rule: [AllowShared] BufferSource → AllowSharedBufferSource.

	t.Run("migrate-allowshared/AllowShared BufferSource fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "migrate-allowshared", `
[Exposed=Window]
interface I {
  undefined foo([AllowShared] BufferSource source);
};`)
	})
	t.Run("migrate-allowshared/AllowSharedBufferSource OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "migrate-allowshared", `
[Exposed=Window]
interface I {
  undefined foo(AllowSharedBufferSource source);
};`)
	})

	// ── replace-void ───────────────────────────────────────────────────────
	// Rule: void return type → undefined.

	t.Run("replace-void/void return fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "replace-void", `
[Exposed=Window]
interface I {
  void foo();
};`)
	})
	t.Run("replace-void/undefined return OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "replace-void", `
[Exposed=Window]
interface I {
  undefined foo();
};`)
	})

	// ── obsolete-async-iterable-syntax ─────────────────────────────────────
	// Rule: `async iterable` (space) → `async_iterable` (underscore).

	t.Run("obsolete-async-iterable-syntax/async iterable fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, "obsolete-async-iterable-syntax", `
[Exposed=Window]
interface I {
  async iterable<long>;
};`)
	})
	t.Run("obsolete-async-iterable-syntax/async_iterable OK", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, "obsolete-async-iterable-syntax", `
[Exposed=Window]
interface I {
  async_iterable<long>;
};`)
	})
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

// TestValidateOverloadDistinguishability covers the §3.2.11
// overload-not-distinguishable rule: within a single interface or namespace,
// all overload pairs for the same operation name must be distinguishable at
// some argument position.
func TestValidateOverloadDistinguishability(t *testing.T) {
	t.Parallel()

	mustFire := func(t *testing.T, rule, src string) {
		t.Helper()
		defs, err := Parse(src)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		errs := Validate(defs)
		for _, e := range errs {
			if ve, ok := e.(*ValidationError); ok && ve.Rule == rule {
				return
			}
		}
		t.Errorf("expected a ValidationError with rule %q; got: %v", rule, errs)
	}

	mustNotFire := func(t *testing.T, rule, src string) {
		t.Helper()
		defs, err := Parse(src)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		errs := Validate(defs)
		for _, e := range errs {
			if ve, ok := e.(*ValidationError); ok && ve.Rule == rule {
				t.Errorf("unexpected ValidationError with rule %q; got: %v", rule, errs)
				return
			}
		}
	}

	const rule = "overload-not-distinguishable"

	// Edge / boundary -----------------------------------------------------------

	// Two zero-argument overloads share an empty effective argument list —
	// there is no position at which to distinguish them.
	t.Run("zero-arg/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo();
  undefined foo();
};`)
	})

	// An overload that uses `any` at position 0 can never be distinguished from
	// any other overload at that position: `any` matches all types.
	t.Run("any-type/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(any x);
  undefined foo(long y);
};`)
	})

	// Error / rejection ---------------------------------------------------------

	// Same base type at position 0 — canonical done-when case from the ticket.
	t.Run("same-type-pos0/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x);
  undefined foo(long y);
};`)
	})

	// Three overloads: the (long, long) pair is indistinguishable even though
	// the third overload (DOMString) is distinguishable from the first.
	t.Run("three-overloads-bad-pair/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x);
  undefined foo(long y);
  undefined foo(DOMString s);
};`)
	})

	// Cross-feature interaction --------------------------------------------------

	// Namespace operations (not just interface members) must satisfy the check.
	t.Run("namespace-scope/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
namespace N {
  undefined foo(long x);
  undefined foo(long y);
};`)
	})

	// Happy path ----------------------------------------------------------------

	// Different types at position 0 — other canonical done-when case from the ticket.
	t.Run("different-types-pos0/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x);
  undefined foo(DOMString y);
};`)
	})

	// Single overload — no pair to compare, rule cannot fire.
	t.Run("single-overload/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x);
};`)
	})

	// Adversary gap tests -------------------------------------------------------

	// 1a: Optional arg expansion — f(long) and f(long, optional DOMString) both
	// appear in the effective overload set at size 1 with type (long). Ambiguous
	// at that size even though size 2 is distinguishable.
	t.Run("optional-arg-ambiguous-effective-tuple/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x);
  undefined foo(long x, optional DOMString y);
};`)
	})

	// 1b: Variadic arg — foo(long... x) contributes a size-1 effective tuple of
	// type (long), which collides with foo(long y) at size 1.
	t.Run("variadic-same-type/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long... x);
  undefined foo(long y);
};`)
	})

	// 1c: Optional stripping produces two size-0 effective tuples with no argument
	// positions — indistinguishable at size 0 even though size-1 types differ.
	t.Run("zero-arg-via-optional-stripping/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(optional long x);
  undefined foo(optional DOMString y);
};`)
	})

	// 1d: Completely different arities with no shared effective-set size must
	// NOT fire — there is no argument count at which both appear.
	t.Run("different-arity-no-shared-size/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x);
  undefined foo(DOMString x, long y);
};`)
	})

	// 2a: String-type same bucket — DOMString and USVString are both string types
	// and are not distinguishable from each other per the spec table.
	t.Run("string-same-bucket/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(DOMString x);
  undefined foo(USVString y);
};`)
	})

	// 2b: Both args are object — same type, clearly not distinguishable.
	t.Run("object-vs-object/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(object x);
  undefined foo(object y);
};`)
	})

	// 2e: Sequence-like same bucket — sequence<T> and FrozenArray<T> are in the
	// same "sequence types" bucket regardless of element type.
	t.Run("sequence-vs-frozenarray/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(sequence<long> x);
  undefined foo(FrozenArray<DOMString> y);
};`)
	})

	// 3a: Static and non-static operations of the same name are in separate
	// effective overload sets — the pair must NOT trigger the rule.
	t.Run("static-vs-nonstatic-same-name/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x);
  static undefined foo(long y);
};`)
	})

	// 3b: Two indistinguishable static overloads must fire — static operations
	// have their own effective overload set that is checked independently.
	t.Run("static-same-type/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  static undefined foo(long x);
  static undefined foo(long y);
};`)
	})

	// 3c: Same-type ops across base and partial are caught by no-cross-overload,
	// not overload-not-distinguishable. The wrong rule must not fire here.
	t.Run("cross-partial-boundary/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x);
};
partial interface I {
  undefined foo(long y);
};`)
	})

	// 4a: `any` blocks distinguishability at position 0, but position 1 has
	// different types — the pair IS distinguishable and must NOT fire.
	t.Run("any-pos0-diff-pos1/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(any x, long a);
  undefined foo(any x, DOMString b);
};`)
	})

	// 5a: Typedef resolved to the same base type — textual comparison would
	// incorrectly say "distinguishable"; resolution is required.
	t.Run("typedef-same-base-type/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
typedef long MyLong;
[Exposed=Window]
interface I {
  undefined foo(long x);
  undefined foo(MyLong y);
};`)
	})

	// 5b: Nullable strips to the same numeric type — long? and long are both in
	// the numeric-types bucket and are not distinguishable.
	t.Run("nullable-vs-nonnullable-same-base/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long? x);
  undefined foo(long y);
};`)
	})

	// 5c: Spec step 1 short-circuit — one type is nullable and the other is a
	// dictionary; per §3.2.11.1 step 1 these are immediately not distinguishable.
	// Required field on D prevents dict-arg-optional from firing on this fixture.
	t.Run("nullable-scalar-vs-dictionary/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
dictionary D { required long x; };
[Exposed=Window]
interface I {
  undefined foo(long? x);
  undefined foo(D y);
};`)
	})

	// 6a: Cross-category no-fire cases — each pair falls in different spec buckets
	// and must NOT fire.

	t.Run("boolean-vs-numeric/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(boolean x);
  undefined foo(long y);
};`)
	})

	t.Run("sequence-vs-numeric/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(sequence<long> x);
  undefined foo(long y);
};`)
	})

	t.Run("dictionary-vs-string/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
dictionary D {};
[Exposed=Window]
interface I {
  undefined foo(D d);
  undefined foo(DOMString s);
};`)
	})

	// 6b: Position > 0 distinguishability — both directions must work correctly.

	// Same type at pos 0, different types at pos 1: distinguishable → must NOT fire.
	t.Run("same-pos0-diff-pos1/no-fire", func(t *testing.T) {
		t.Parallel()
		mustNotFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x, DOMString a);
  undefined foo(long x, long b);
};`)
	})

	// Same type at pos 0 AND pos 1: no distinguishing position → must fire.
	t.Run("same-pos0-same-pos1/fires", func(t *testing.T) {
		t.Parallel()
		mustFire(t, rule, `
[Exposed=Window]
interface I {
  undefined foo(long x, DOMString a);
  undefined foo(long x, DOMString b);
};`)
	})
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

