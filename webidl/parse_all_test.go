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
