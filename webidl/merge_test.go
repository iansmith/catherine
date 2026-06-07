package webidl

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Edge / boundary cases
// ---------------------------------------------------------------------------

// TestMergeEmptyInput verifies that Merge on nil/empty input returns a valid,
// empty, non-nil IR with no errors.
func TestMergeEmptyInput(t *testing.T) {
	t.Parallel()
	ir, errs := Merge(nil)
	if ir == nil {
		t.Fatal("Merge(nil) returned nil IR")
	}
	if len(errs) != 0 {
		t.Fatalf("Merge(nil) returned unexpected errors: %v", errs)
	}
	if ir.Lookup("Anything") != nil {
		t.Error("Lookup on empty IR should return nil")
	}
	if ir.Len() != 0 {
		t.Errorf("expected empty IR (Len==0), got %d", ir.Len())
	}
}

// TestMergePartialFolding verifies that multiple partial interfaces are folded
// into the primary: member count equals the sum across all parts.
func TestMergePartialFolding(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Foo {
  attribute long a;
};
partial interface Foo {
  attribute long b;
};
partial interface Foo {
  attribute long c;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	md := ir.Lookup("Foo")
	if md == nil {
		t.Fatal("Lookup(\"Foo\") returned nil")
	}
	// Primary contributes 1 member; two partials contribute 1 each → 3 total.
	if got := len(md.Members); got != 3 {
		t.Errorf("expected 3 merged members, got %d", got)
	}
}

// TestMergePartialDictionaryFolding verifies partial dictionary merging.
func TestMergePartialDictionaryFolding(t *testing.T) {
	t.Parallel()
	src := `
dictionary D {
  long a;
};
partial dictionary D {
  long b;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	md := ir.Lookup("D")
	if md == nil {
		t.Fatal("Lookup(\"D\") returned nil")
	}
	if got := len(md.Members); got != 2 {
		t.Errorf("expected 2 dictionary members after partial fold, got %d", got)
	}
}

// TestMergePartialUnknownPrimary verifies that a partial with no matching
// primary produces a non-fatal merge error rather than silently dropping it.
func TestMergePartialUnknownPrimary(t *testing.T) {
	t.Parallel()
	src := `
partial interface Ghost {
  attribute long x;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if ir == nil {
		t.Fatal("Merge returned nil IR on unknown-primary input")
	}
	if len(mergeErrs) == 0 {
		t.Error("expected a merge error for partial with no primary, got none")
	}
}

// TestMergeOnlyPartials verifies that two partials with no primary both produce
// merge errors (one per orphaned partial).
func TestMergeOnlyPartials(t *testing.T) {
	t.Parallel()
	src := `
partial interface Ghost {
  attribute long x;
};
partial interface Ghost {
  attribute long y;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if ir == nil {
		t.Fatal("Merge returned nil IR")
	}
	if len(mergeErrs) < 2 {
		t.Errorf("expected at least 2 merge errors for 2 orphaned partials, got %d", len(mergeErrs))
	}
}

// ---------------------------------------------------------------------------
// Mixin application
// ---------------------------------------------------------------------------

// TestMergeMixinApplication verifies that 'X includes M' grafts M's members
// onto X's merged member list.
func TestMergeMixinApplication(t *testing.T) {
	t.Parallel()
	src := `
interface mixin M {
  attribute long mx;
};
[Exposed=Window]
interface X {
  attribute long own;
};
X includes M;
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	md := ir.Lookup("X")
	if md == nil {
		t.Fatal("Lookup(\"X\") returned nil")
	}
	// X has its own 'own' member + M's 'mx' member grafted in.
	if got := len(md.Members); got != 2 {
		t.Errorf("expected 2 members after mixin application, got %d", got)
	}
}

// TestMergeMixinIncludedByMultiple verifies that a mixin included by two
// distinct interfaces is applied independently to each.
func TestMergeMixinIncludedByMultiple(t *testing.T) {
	t.Parallel()
	src := `
interface mixin M {
  attribute long mx;
};
[Exposed=Window]
interface X {};
[Exposed=Window]
interface Y {};
X includes M;
Y includes M;
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	for _, name := range []string{"X", "Y"} {
		md := ir.Lookup(name)
		if md == nil {
			t.Fatalf("Lookup(%q) returned nil", name)
		}
		if got := len(md.Members); got != 1 {
			t.Errorf("%s: expected 1 member from mixin, got %d", name, got)
		}
	}
}

// TestMergeMixinUnknownTarget verifies that an includes statement whose target
// interface is not defined produces a merge error.
func TestMergeMixinUnknownTarget(t *testing.T) {
	t.Parallel()
	src := `
interface mixin M {
  attribute long mx;
};
Ghost includes M;
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if ir == nil {
		t.Fatal("Merge returned nil IR")
	}
	if len(mergeErrs) == 0 {
		t.Error("expected a merge error for includes with unknown target, got none")
	}
}

// TestMergeMixinIncludesNonMixin verifies that an includes statement referencing
// a regular (non-mixin) interface produces a merge error rather than silently
// grafting members. This guards the iface.Variant != IfaceMixin check in
// applyMixins.
func TestMergeMixinIncludesNonMixin(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface RegularIface {
  attribute long x;
};
[Exposed=Window]
interface Target {};
Target includes RegularIface;
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, mergeErrs := Merge(defs)
	if len(mergeErrs) == 0 {
		t.Error("expected a merge error when including a non-mixin interface, got none")
	}
}

// TestMergeMixinUnknownMixin verifies that an includes statement referencing a
// non-existent mixin produces a merge error.
func TestMergeMixinUnknownMixin(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface X {};
X includes Ghost;
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if ir == nil {
		t.Fatal("Merge returned nil IR")
	}
	if len(mergeErrs) == 0 {
		t.Error("expected a merge error for includes with unknown mixin, got none")
	}
}

// ---------------------------------------------------------------------------
// Inheritance chain resolution
// ---------------------------------------------------------------------------

// TestMergeInheritanceChain verifies that a three-level chain (C:B:A) results
// in C.InheritedMembers containing both B's and A's own members.
func TestMergeInheritanceChain(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface A {
  attribute long ax;
};
[Exposed=Window]
interface B : A {
  attribute long bx;
};
[Exposed=Window]
interface C : B {
  attribute long cx;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	mdC := ir.Lookup("C")
	if mdC == nil {
		t.Fatal("Lookup(\"C\") returned nil")
	}
	// C's own member: cx (1).  Inherited from B (bx) and A (ax): 2 total.
	if got := len(mdC.Members); got != 1 {
		t.Errorf("C: expected 1 own member, got %d", got)
	}
	if got := len(mdC.InheritedMembers); got != 2 {
		t.Errorf("C: expected 2 inherited members (bx + ax), got %d", got)
	}
}

// TestMergeInheritanceUnknownParent verifies that inheriting from an
// undefined parent produces a merge error rather than a panic.
func TestMergeInheritanceUnknownParent(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Child : NonExistent {};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if ir == nil {
		t.Fatal("Merge returned nil IR")
	}
	if len(mergeErrs) == 0 {
		t.Error("expected a merge error for unknown parent, got none")
	}
}

// TestMergeInheritanceCycle verifies that a cycle (A:B, B:A) produces a merge
// error and does NOT hang.
func TestMergeInheritanceCycle(t *testing.T) {
	t.Parallel()
	// Build a synthetic AST directly — bypasses the parser so we can create
	// an illegal cycle without hitting parse errors first.
	a := &Interface{Name: "A", Inheritance: "B", Variant: IfaceRegular}
	b := &Interface{Name: "B", Inheritance: "A", Variant: IfaceRegular}
	defs := []Definition{a, b}

	// Must not hang; must return a merge error describing the cycle.
	ir, mergeErrs := Merge(defs)
	if ir == nil {
		t.Fatal("Merge returned nil IR on cycle input")
	}
	if len(mergeErrs) == 0 {
		t.Error("expected a merge error for inheritance cycle, got none")
	}
}

// TestMergeInheritanceCycleDeepChain verifies that a cycle nested in a deeper
// chain (A:B, B:C, C:B) is detected without hanging. A is outside the B↔C
// cycle, so the error message must reference the cycle nodes (B or C), not A.
func TestMergeInheritanceCycleDeepChain(t *testing.T) {
	t.Parallel()
	// Synthetic AST: A inherits B, B inherits C, C inherits B (cycle is B:C:B).
	a := &Interface{Name: "A", Inheritance: "B", Variant: IfaceRegular}
	b := &Interface{Name: "B", Inheritance: "C", Variant: IfaceRegular}
	c := &Interface{Name: "C", Inheritance: "B", Variant: IfaceRegular}
	defs := []Definition{a, b, c}

	ir, mergeErrs := Merge(defs)
	if ir == nil {
		t.Fatal("Merge returned nil IR on deep-cycle input")
	}
	if len(mergeErrs) == 0 {
		t.Error("expected merge errors for inheritance cycle, got none")
	}
}

// ---------------------------------------------------------------------------
// Multi-file merge
// ---------------------------------------------------------------------------

// TestMergeFilesPartialAcrossFiles verifies that a partial defined in a
// separate file is correctly folded into the primary from another file.
func TestMergeFilesPartialAcrossFiles(t *testing.T) {
	t.Parallel()
	src1 := `
[Exposed=Window]
interface Foo {
  attribute long a;
};
`
	src2 := `
partial interface Foo {
  attribute long b;
};
`
	defs1, err := Parse(src1)
	if err != nil {
		t.Fatalf("parse src1: %v", err)
	}
	defs2, err := Parse(src2)
	if err != nil {
		t.Fatalf("parse src2: %v", err)
	}
	ir, mergeErrs := MergeFiles([][]Definition{defs1, defs2})
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	md := ir.Lookup("Foo")
	if md == nil {
		t.Fatal("Lookup(\"Foo\") returned nil after cross-file merge")
	}
	// Primary (1) + cross-file partial (1) = 2.
	if got := len(md.Members); got != 2 {
		t.Errorf("expected 2 members from cross-file merge, got %d", got)
	}
}

// TestMergeFilesEmpty verifies that MergeFiles on an empty slice behaves the
// same as Merge(nil).
func TestMergeFilesEmpty(t *testing.T) {
	t.Parallel()
	ir, errs := MergeFiles(nil)
	if ir == nil {
		t.Fatal("MergeFiles(nil) returned nil IR")
	}
	if len(errs) != 0 {
		t.Fatalf("MergeFiles(nil) returned unexpected errors: %v", errs)
	}
	if ir.Len() != 0 {
		t.Errorf("expected empty IR, got %d definitions", ir.Len())
	}
}
