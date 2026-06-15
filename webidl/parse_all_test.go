package webidl

import (
	"strings"
	"testing"
)

// goodIface is a valid WebIDL interface used as a "good" definition in
// multi-definition test cases.
const goodIfaceA = `interface GoodA { attribute DOMString name; };`
const goodIfaceB = `interface GoodB { attribute long count; };`
const goodIfaceC = `interface GoodC { attribute boolean flag; };`

// badIface is an interface with a missing attribute type — a reliable grammar
// error that does NOT require an unrecognised character (so it's a parser
// error, not a tokenizer error).
const badIfaceA = `interface BrokenA { attribute; };`
const badIfaceB = `interface BrokenB { attribute; };`
const badIfaceC = `interface BrokenC { attribute; };`

// joinIDL concatenates IDL fragments with a newline separator so line numbers
// in errors are distinct across definitions.
func joinIDL(parts ...string) string {
	return strings.Join(parts, "\n")
}

// ---- Edge / boundary cases -----------------------------------------------

func TestParseAllEmpty(t *testing.T) {
	defs, errs := ParseAll("")
	if len(defs) != 0 {
		t.Errorf("empty input: want 0 defs, got %d", len(defs))
	}
	if len(errs) != 0 {
		t.Errorf("empty input: want 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestParseAllWhitespaceOnly(t *testing.T) {
	defs, errs := ParseAll("   \n\t  ")
	if len(defs) != 0 {
		t.Errorf("whitespace input: want 0 defs, got %d", len(defs))
	}
	if len(errs) != 0 {
		t.Errorf("whitespace input: want 0 errors, got %d: %v", len(errs), errs)
	}
}

// Two consecutive bad definitions — the critical edge case the ticket was
// filed for. The stub returns only one error; the full implementation must
// return two.
func TestParseAllTwoBadDefsProduceTwoErrors(t *testing.T) {
	src := joinIDL(badIfaceA, badIfaceB)
	_, errs := ParseAll(src)
	if len(errs) != 2 {
		t.Errorf("two bad defs: want 2 errors, got %d\nerrs: %v", len(errs), errs)
	}
}

// Three bad definitions — extends the boundary check to N > 2.
func TestParseAllThreeBadDefsProduceThreeErrors(t *testing.T) {
	src := joinIDL(badIfaceA, badIfaceB, badIfaceC)
	_, errs := ParseAll(src)
	if len(errs) != 3 {
		t.Errorf("three bad defs: want 3 errors, got %d\nerrs: %v", len(errs), errs)
	}
}

// ---- Error / rejection cases -----------------------------------------------

// One bad definition → exactly one error; zero definitions.
func TestParseAllSingleBadDef(t *testing.T) {
	_, errs := ParseAll(badIfaceA)
	if len(errs) != 1 {
		t.Errorf("single bad def: want 1 error, got %d: %v", len(errs), errs)
	}
}

// Each error must carry a non-zero line number so callers can report location.
func TestParseAllErrorsCarryLineNumbers(t *testing.T) {
	src := joinIDL(badIfaceA, badIfaceB)
	_, errs := ParseAll(src)
	for i, e := range errs {
		if e.Line == 0 {
			t.Errorf("error[%d] has Line==0; want a non-zero line: %v", i, e)
		}
	}
}

// Errors for distinct definitions must report distinct line numbers.
func TestParseAllDistinctErrorLines(t *testing.T) {
	src := joinIDL(badIfaceA, badIfaceB)
	_, errs := ParseAll(src)
	if len(errs) < 2 {
		t.Skipf("need at least 2 errors to check distinct lines (got %d)", len(errs))
	}
	if errs[0].Line == errs[1].Line {
		t.Errorf("two errors from two distinct definitions share line %d; want different lines", errs[0].Line)
	}
}

// ---- Cross-feature interaction cases ----------------------------------------

// Valid definitions that appear BEFORE a bad one must be returned.
func TestParseAllGoodBeforeBadIsReturned(t *testing.T) {
	src := joinIDL(goodIfaceA, badIfaceA)
	defs, errs := ParseAll(src)
	if len(defs) != 1 {
		t.Errorf("good-then-bad: want 1 def, got %d", len(defs))
	}
	if len(errs) != 1 {
		t.Errorf("good-then-bad: want 1 error, got %d: %v", len(errs), errs)
	}
}

// Valid definitions that appear AFTER a bad one must also be returned — the
// key cross-feature regression. Parse() stops here; ParseAll must not.
func TestParseAllGoodAfterBadIsReturned(t *testing.T) {
	src := joinIDL(badIfaceA, goodIfaceA)
	defs, errs := ParseAll(src)
	if len(errs) != 1 {
		t.Errorf("bad-then-good: want 1 error, got %d: %v", len(errs), errs)
	}
	if len(defs) != 1 {
		t.Errorf("bad-then-good: want 1 def (the good one), got %d", len(defs))
	}
}

// Good–bad–good: definitions surrounding a bad one must all appear.
func TestParseAllGoodBadGoodPattern(t *testing.T) {
	src := joinIDL(goodIfaceA, badIfaceA, goodIfaceB)
	defs, errs := ParseAll(src)
	if len(errs) != 1 {
		t.Errorf("good-bad-good: want 1 error, got %d: %v", len(errs), errs)
	}
	if len(defs) != 2 {
		t.Errorf("good-bad-good: want 2 defs, got %d", len(defs))
	}
}

// Bad–good–bad: only the middle good definition must survive.
func TestParseAllBadGoodBadPattern(t *testing.T) {
	src := joinIDL(badIfaceA, goodIfaceA, badIfaceB)
	defs, errs := ParseAll(src)
	if len(errs) != 2 {
		t.Errorf("bad-good-bad: want 2 errors, got %d: %v", len(errs), errs)
	}
	if len(defs) != 1 {
		t.Errorf("bad-good-bad: want 1 def, got %d", len(defs))
	}
}

// ParseAll with a fully valid source must equal what Parse returns and produce
// no errors — interop sanity check.
func TestParseAllFullyValidMatchesParse(t *testing.T) {
	src := joinIDL(goodIfaceA, goodIfaceB, goodIfaceC)
	defs, errs := ParseAll(src)
	if len(errs) != 0 {
		t.Errorf("all-good: want 0 errors, got %d: %v", len(errs), errs)
	}
	parseDefs, parseErr := Parse(src)
	if parseErr != nil {
		t.Fatalf("Parse() unexpectedly failed: %v", parseErr)
	}
	if len(defs) != len(parseDefs) {
		t.Errorf("all-good: ParseAll returned %d defs, Parse returned %d", len(defs), len(parseDefs))
	}
}

// ---- Happy-path case --------------------------------------------------------

// ParseAll with a single valid definition returns that definition and no errors.
func TestParseAllSingleGoodDef(t *testing.T) {
	defs, errs := ParseAll(goodIfaceA)
	if len(errs) != 0 {
		t.Errorf("single good def: want 0 errors, got %d: %v", len(errs), errs)
	}
	if len(defs) != 1 {
		t.Errorf("single good def: want 1 def, got %d", len(defs))
	}
}

// ---- Adversary gap tests (A–F) ---------------------------------------------

// A — Bad dictionary followed by a good definition: the good one must survive.
const badDict = `dictionary BrokenDict { required; };`

func TestParseAllBadDictionaryGoodAfterReturned(t *testing.T) {
	src := joinIDL(badDict, goodIfaceA)
	defs, errs := ParseAll(src)
	if len(errs) == 0 {
		t.Error("bad dictionary: want at least 1 error, got 0")
	}
	if len(defs) != 1 {
		t.Errorf("bad dictionary: want 1 def (good interface after), got %d", len(defs))
	}
}

// B — Bad enum followed by a good definition: the good one must survive.
const badEnum = `enum BrokenEnum { };`

func TestParseAllBadEnumGoodAfterReturned(t *testing.T) {
	src := joinIDL(badEnum, goodIfaceA)
	defs, errs := ParseAll(src)
	if len(errs) == 0 {
		t.Error("bad enum: want at least 1 error, got 0")
	}
	if len(defs) != 1 {
		t.Errorf("bad enum: want 1 def (good interface after), got %d", len(defs))
	}
}

// C — Bad partial keyword followed by a good definition: the good one must
// survive even though `partial` was consumed before the error fired.
func TestParseAllBadPartialGoodAfterReturned(t *testing.T) {
	src := joinIDL(`partial enum Bad { "x" };`, goodIfaceA)
	defs, errs := ParseAll(src)
	if len(errs) == 0 {
		t.Error("bad partial: want at least 1 error, got 0")
	}
	if len(defs) != 1 {
		t.Errorf("bad partial: want 1 def (good interface after), got %d", len(defs))
	}
}

// D — Good–bad–good: verify the returned definitions have the right names and
// types, not just the right count.
func TestParseAllGoodBadGoodDefsHaveCorrectIdentity(t *testing.T) {
	src := joinIDL(goodIfaceA, badIfaceA, goodIfaceB)
	defs, errs := ParseAll(src)
	if len(errs) != 1 || len(defs) != 2 {
		t.Fatalf("good-bad-good: want 1 error and 2 defs, got %d errors and %d defs (errs=%v)",
			len(errs), len(defs), errs)
	}
	for i, d := range defs {
		iface, ok := d.(*Interface)
		if !ok {
			t.Errorf("defs[%d]: want *Interface, got %T", i, d)
			continue
		}
		want := []string{"GoodA", "GoodB"}[i]
		if iface.Name != want {
			t.Errorf("defs[%d]: want Name=%q, got %q", i, want, iface.Name)
		}
	}
}

// E — Errors must be returned in source (ascending line-number) order.
func TestParseAllErrorsInSourceOrder(t *testing.T) {
	src := joinIDL(badIfaceA, badIfaceB, badIfaceC)
	_, errs := ParseAll(src)
	if len(errs) < 2 {
		t.Skipf("need at least 2 errors to check ordering (got %d); run after full implementation", len(errs))
	}
	for i := 1; i < len(errs); i++ {
		if errs[i].Line < errs[i-1].Line {
			t.Errorf("errors out of order: errs[%d].Line=%d < errs[%d].Line=%d",
				i, errs[i].Line, i-1, errs[i-1].Line)
		}
	}
}

// F — Every error must carry a non-empty Message field.
func TestParseAllErrorsHaveNonEmptyMessage(t *testing.T) {
	src := joinIDL(badIfaceA, badIfaceB)
	_, errs := ParseAll(src)
	if len(errs) == 0 {
		t.Fatal("want at least 1 error")
	}
	for i, e := range errs {
		if e.Message == "" {
			t.Errorf("error[%d] has empty Message (Line=%d)", i, e.Line)
		}
	}
}

// ---- Regression: malformed extended attributes that bleed into the definition body ----

// When an extended-attribute block is malformed with a trailing comma ([Foo,) and is
// immediately followed by a definition, the entire unit — extattr + definition — is
// treated as one broken definition and dropped.  The interface is NOT returned as a
// separate good definition because the user intended it to be decorated by [Foo].
func TestParseAllBadExtAttrTreatedAsOneUnit(t *testing.T) {
	src := "[Foo,\n" + goodIfaceA
	defs, errs := ParseAll(src)
	if len(errs) == 0 {
		t.Error("malformed extattr: want at least 1 error, got 0")
	}
	if len(defs) != 0 {
		t.Errorf("malformed extattr: [Foo, interface GoodA] is one broken unit — want 0 defs, got %d", len(defs))
	}
}

// ParseAll leaves the existing Parse() untouched — it must still work and
// return exactly one error for a bad source.
func TestParseBackwardsCompatibility(t *testing.T) {
	src := joinIDL(badIfaceA, badIfaceB)
	_, err := Parse(src)
	if err == nil {
		t.Error("Parse() on bad source must still return an error")
	}
	// Parse() returns at most one error — this is the preserved contract.
	if _, ok := err.(*ParseError); !ok {
		t.Errorf("Parse() must return *ParseError, got %T: %v", err, err)
	}
}
