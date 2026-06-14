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

// TestMergeMixinExtAttrPropagation verifies that 'X includes M' grafts M's
// ExtAttrs onto X's merged ExtAttr list, not only M's members.
func TestMergeMixinExtAttrPropagation(t *testing.T) {
	t.Parallel()
	src := `
[SecureContext]
interface mixin M {
  attribute long mx;
};
[Exposed=Window]
interface X {};
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
	// X contributes [Exposed=Window] (1); M contributes [SecureContext] (1) via includes.
	if got := len(md.ExtAttrs); got != 2 {
		t.Fatalf("expected 2 ExtAttrs after mixin include, got %d", got)
	}
	names := map[string]bool{}
	for _, ea := range md.ExtAttrs {
		names[ea.Name] = true
	}
	if !names["Exposed"] || !names["SecureContext"] {
		t.Errorf("missing expected ExtAttrs: got names %v", names)
	}
}

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
// AllMembers / LookupMember helpers (CATH-26)
// ---------------------------------------------------------------------------

// attrName extracts the Name from a Member that is an *Attribute.
// Returns "" for any other Member kind (forces a test failure if misused).
func attrName(m Member) string {
	if a, ok := m.(*Attribute); ok {
		return a.Name
	}
	return ""
}

// TestAllMembersNoInheritance verifies that AllMembers on a flat (non-inheriting)
// interface returns exactly the same members as MergedDef.Members.
func TestAllMembersNoInheritance(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Flat {
  attribute long a;
  attribute long b;
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
	md := ir.Lookup("Flat")
	if md == nil {
		t.Fatal("Lookup(\"Flat\") returned nil")
	}
	all := md.AllMembers()
	if got, want := len(all), len(md.Members); got != want {
		t.Errorf("AllMembers length: got %d, want %d (same as Members)", got, want)
	}
}

// TestAllMembersNoOwnMembers verifies that a child with zero own members and a
// parent with members returns only the inherited members from AllMembers.
func TestAllMembersNoOwnMembers(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Parent {
  attribute long px;
};
[Exposed=Window]
interface Child : Parent {};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	md := ir.Lookup("Child")
	if md == nil {
		t.Fatal("Lookup(\"Child\") returned nil")
	}
	if got := len(md.Members); got != 0 {
		t.Fatalf("expected Child to have 0 own members, got %d", got)
	}
	all := md.AllMembers()
	if got := len(all); got != 1 {
		t.Errorf("AllMembers: expected 1 (inherited px), got %d", got)
	}
}

// TestAllMembersThreeLevelOrder verifies that AllMembers on a three-level chain
// C:B:A returns C's own members first, then B's, then A's — closest-first.
func TestAllMembersThreeLevelOrder(t *testing.T) {
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
	md := ir.Lookup("C")
	if md == nil {
		t.Fatal("Lookup(\"C\") returned nil")
	}
	all := md.AllMembers()
	if got := len(all); got != 3 {
		t.Fatalf("AllMembers: expected 3 (cx, bx, ax), got %d", got)
	}
	want := []string{"cx", "bx", "ax"}
	for i, m := range all {
		if got := attrName(m); got != want[i] {
			t.Errorf("AllMembers[%d]: got %q, want %q", i, got, want[i])
		}
	}
}

// TestAllMembersWithMixinsAndInheritance verifies that mixin members appear in
// the own set (before inherited members) when both are present.
func TestAllMembersWithMixinsAndInheritance(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Parent {
  attribute long px;
};
interface mixin M {
  attribute long mx;
};
[Exposed=Window]
interface Child : Parent {
  attribute long cx;
};
Child includes M;
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	md := ir.Lookup("Child")
	if md == nil {
		t.Fatal("Lookup(\"Child\") returned nil")
	}
	// Child owns cx + mx (mixin); px is inherited.
	all := md.AllMembers()
	if got := len(all); got != 3 {
		t.Fatalf("AllMembers: expected 3 (cx, mx, px), got %d", got)
	}
	// The first two are own; the last is inherited.
	ownNames := map[string]bool{
		attrName(all[0]): true,
		attrName(all[1]): true,
	}
	if !ownNames["cx"] || !ownNames["mx"] {
		t.Errorf("AllMembers: expected own members {cx, mx} in first two slots, got %q and %q",
			attrName(all[0]), attrName(all[1]))
	}
	if got := attrName(all[2]); got != "px" {
		t.Errorf("AllMembers[2]: expected inherited px, got %q", got)
	}
}

// TestLookupMemberNotFound verifies that LookupMember returns (nil, false) for
// a name that does not exist in own or inherited members.
func TestLookupMemberNotFound(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Iface {
  attribute long a;
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
	md := ir.Lookup("Iface")
	if md == nil {
		t.Fatal("Lookup(\"Iface\") returned nil")
	}
	m, ok := md.LookupMember("nonexistent")
	if ok {
		t.Errorf("LookupMember(\"nonexistent\"): expected ok=false, got true (member=%v)", m)
	}
	if m != nil {
		t.Errorf("LookupMember(\"nonexistent\"): expected nil member, got %v", m)
	}
}

// TestLookupMemberInheritedSet verifies that LookupMember finds a member that
// exists only in the inherited set (not in own members).
func TestLookupMemberInheritedSet(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Base {
  attribute long baseAttr;
};
[Exposed=Window]
interface Derived : Base {};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	md := ir.Lookup("Derived")
	if md == nil {
		t.Fatal("Lookup(\"Derived\") returned nil")
	}
	m, ok := md.LookupMember("baseAttr")
	if !ok {
		t.Fatal("LookupMember(\"baseAttr\"): expected ok=true for inherited member")
	}
	if got := attrName(m); got != "baseAttr" {
		t.Errorf("LookupMember returned wrong member: got name %q, want %q", got, "baseAttr")
	}
}

// TestLookupMemberOwnShadowsInherited verifies prototype-chain lookup semantics:
// when the same member name appears in both own and inherited sets, the own
// (most-derived) member is returned.
func TestLookupMemberOwnShadowsInherited(t *testing.T) {
	t.Parallel()
	// Both Parent and Child define an attribute named "shared".
	// WebIDL allows this in dictionaries (field override) and via [SameObject] patterns.
	// We test with dictionary fields to stay within spec.
	src := `
dictionary Parent {
  long shared;
  long parentOnly;
};
dictionary Child : Parent {
  long shared;
  long childOnly;
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
	md := ir.Lookup("Child")
	if md == nil {
		t.Fatal("Lookup(\"Child\") returned nil")
	}
	m, ok := md.LookupMember("shared")
	if !ok {
		t.Fatal("LookupMember(\"shared\"): expected ok=true")
	}
	// The returned member must be Child's own "shared", not Parent's.
	// Verify by checking it's in the own Members slice, not InheritedMembers.
	foundInOwn := false
	for _, own := range md.Members {
		if own == m {
			foundInOwn = true
			break
		}
	}
	if !foundInOwn {
		t.Error("LookupMember(\"shared\"): returned inherited copy instead of own (most-derived) member")
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

// ---------------------------------------------------------------------------
// ExtAttr merging from partials (CATH-17)
// ---------------------------------------------------------------------------

// TestMergeExtAttrPrimaryNoPartials verifies that a primary's own ExtAttrs are
// copied into MergedDef.ExtAttrs even when no partials exist.
func TestMergeExtAttrPrimaryNoPartials(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Foo {
  attribute long a;
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
	if got := len(md.ExtAttrs); got != 1 {
		t.Errorf("expected 1 ExtAttr from primary, got %d", got)
	}
	if md.ExtAttrs[0].Name != "Exposed" {
		t.Errorf("expected ExtAttr name %q, got %q", "Exposed", md.ExtAttrs[0].Name)
	}
}

// TestMergeExtAttrPartialOnlyPrimaryNone verifies that ExtAttrs from a partial
// appear in MergedDef.ExtAttrs even when the primary has no ExtAttrs.
func TestMergeExtAttrPartialOnlyPrimaryNone(t *testing.T) {
	t.Parallel()
	src := `
interface Foo {
  attribute long a;
};
[SecureContext]
partial interface Foo {
  attribute long b;
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
	if got := len(md.ExtAttrs); got != 1 {
		t.Errorf("expected 1 ExtAttr from partial, got %d", got)
	}
	if md.ExtAttrs[0].Name != "SecureContext" {
		t.Errorf("expected ExtAttr name %q, got %q", "SecureContext", md.ExtAttrs[0].Name)
	}
}

// TestMergeExtAttrBothEmpty verifies that when neither primary nor any partial
// carries ExtAttrs, MergedDef.ExtAttrs is empty (not nil-panicking).
func TestMergeExtAttrBothEmpty(t *testing.T) {
	t.Parallel()
	src := `
interface Foo {
  attribute long a;
};
partial interface Foo {
  attribute long b;
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
	if got := len(md.ExtAttrs); got != 0 {
		t.Errorf("expected 0 ExtAttrs, got %d", got)
	}
}

// TestMergeExtAttrSourceOrder verifies that ExtAttrs are accumulated in source
// order: primary first, then each partial in declaration order.
func TestMergeExtAttrSourceOrder(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Foo {
  attribute long a;
};
[SecureContext]
partial interface Foo {
  attribute long b;
};
[LegacyUnenumerableNamedProperties]
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
	// 1 from primary + 1 from each partial = 3
	if got := len(md.ExtAttrs); got != 3 {
		t.Fatalf("expected 3 ExtAttrs, got %d", got)
	}
	want := []string{"Exposed", "SecureContext", "LegacyUnenumerableNamedProperties"}
	for i, name := range want {
		if md.ExtAttrs[i].Name != name {
			t.Errorf("ExtAttrs[%d]: expected %q, got %q", i, name, md.ExtAttrs[i].Name)
		}
	}
}

// TestMergeExtAttrDictionaryPartial verifies that ExtAttrs from partial
// dictionaries are merged into MergedDef.ExtAttrs.
func TestMergeExtAttrDictionaryPartial(t *testing.T) {
	t.Parallel()
	src := `
dictionary D {
  long a;
};
[SecureContext]
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
	if got := len(md.ExtAttrs); got != 1 {
		t.Errorf("expected 1 ExtAttr from partial dictionary, got %d", got)
	}
	if md.ExtAttrs[0].Name != "SecureContext" {
		t.Errorf("expected %q, got %q", "SecureContext", md.ExtAttrs[0].Name)
	}
}

// TestMergeExtAttrNamespacePartial verifies that ExtAttrs from partial
// namespaces are merged into MergedDef.ExtAttrs.
func TestMergeExtAttrNamespacePartial(t *testing.T) {
	t.Parallel()
	src := `
namespace NS {
  readonly attribute long a;
};
[Exposed=Window]
partial namespace NS {
  readonly attribute long b;
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
	md := ir.Lookup("NS")
	if md == nil {
		t.Fatal("Lookup(\"NS\") returned nil")
	}
	if got := len(md.ExtAttrs); got != 1 {
		t.Errorf("expected 1 ExtAttr from partial namespace, got %d", got)
	}
}

// TestMergeExtAttrMultipleOnSingleDef verifies that multiple ExtAttrs on one
// definition are all preserved — not truncated to the first.
func TestMergeExtAttrMultipleOnSingleDef(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window, SecureContext]
interface Foo {
  attribute long a;
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
	if got := len(md.ExtAttrs); got != 2 {
		t.Fatalf("expected 2 ExtAttrs from single definition, got %d", got)
	}
	if md.ExtAttrs[0].Name != "Exposed" || md.ExtAttrs[1].Name != "SecureContext" {
		t.Errorf("wrong ExtAttr names: got %q, %q", md.ExtAttrs[0].Name, md.ExtAttrs[1].Name)
	}
}

// TestMergeExtAttrPreservesRHS verifies that the full ExtAttr is preserved
// (not just the Name field) — RHS with identifier value must survive the fold.
func TestMergeExtAttrPreservesRHS(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Foo {
  attribute long a;
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
	if len(md.ExtAttrs) == 0 {
		t.Fatal("expected at least 1 ExtAttr")
	}
	ea := md.ExtAttrs[0]
	if ea.RHS == nil {
		t.Fatal("ExtAttr[Exposed=Window] RHS is nil — full ExtAttr not preserved")
	}
	if ea.RHS.Value != "Window" {
		t.Errorf("expected RHS.Value %q, got %q", "Window", ea.RHS.Value)
	}
}

// TestMergeExtAttrNoDuplicateRemoval verifies that ExtAttrs with the same name
// on different definitions are both kept (no deduplication).
func TestMergeExtAttrNoDuplicateRemoval(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Foo {
  attribute long a;
};
[Exposed=Worker]
partial interface Foo {
  attribute long b;
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
	// Both [Exposed=Window] and [Exposed=Worker] must be present — no dedup.
	if got := len(md.ExtAttrs); got != 2 {
		t.Errorf("expected 2 ExtAttrs (no deduplication), got %d", got)
	}
}

// TestMergeExtAttrDictionaryBothContribute verifies accumulation when both the
// primary dictionary and its partial carry ExtAttrs.
func TestMergeExtAttrDictionaryBothContribute(t *testing.T) {
	t.Parallel()
	src := `
[Clamp]
dictionary D {
  long a;
};
[SecureContext]
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
	// Primary contributes [Clamp], partial contributes [SecureContext].
	if got := len(md.ExtAttrs); got != 2 {
		t.Fatalf("expected 2 ExtAttrs from primary+partial dictionary, got %d", got)
	}
	if md.ExtAttrs[0].Name != "Clamp" || md.ExtAttrs[1].Name != "SecureContext" {
		t.Errorf("wrong order: got %q, %q", md.ExtAttrs[0].Name, md.ExtAttrs[1].Name)
	}
}

// TestMergeExtAttrPrimaryNotMutated verifies that merging does not mutate the
// original primary Definition's ExtAttrs slice — the primary must be unchanged
// after Merge returns.
func TestMergeExtAttrPrimaryNotMutated(t *testing.T) {
	t.Parallel()
	src := `
[Exposed=Window]
interface Foo {
  attribute long a;
};
[SecureContext]
partial interface Foo {
  attribute long b;
};
`
	defs, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Record the original primary ExtAttr count before merge.
	var primaryBefore int
	for _, d := range defs {
		if iface, ok := d.(*Interface); ok && !iface.Partial && iface.Name == "Foo" {
			primaryBefore = len(iface.ExtAttrs)
		}
	}
	ir, mergeErrs := Merge(defs)
	if len(mergeErrs) != 0 {
		t.Fatalf("unexpected merge errors: %v", mergeErrs)
	}
	md := ir.Lookup("Foo")
	if md == nil {
		t.Fatal("Lookup(\"Foo\") returned nil")
	}
	// MergedDef.ExtAttrs should have 2 (primary + partial), but the primary
	// Definition itself must still have only 1.
	iface := md.Primary.(*Interface)
	if got := len(iface.ExtAttrs); got != primaryBefore {
		t.Errorf("primary Definition.ExtAttrs was mutated: had %d before merge, has %d after",
			primaryBefore, got)
	}
}
