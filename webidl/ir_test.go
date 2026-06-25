package webidl

import (
	"strings"
	"testing"
)

func TestIR_All_nil(t *testing.T) {
	t.Parallel()
	var ir *IR
	all := ir.All()
	if all != nil {
		t.Errorf("nil IR.All(): want nil, got %v", all)
	}
}

func TestIR_All_empty(t *testing.T) {
	t.Parallel()
	ir, errs := Merge(nil)
	if len(errs) > 0 {
		t.Fatalf("Merge(nil): %v", errs)
	}
	all := ir.All()
	if len(all) != 0 {
		t.Errorf("empty IR.All(): want 0, got %d", len(all))
	}
}

func TestIR_All_singleDef(t *testing.T) {
	t.Parallel()
	defs, err := Parse(`enum Color { "red", "green", "blue" };`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ir, errs := Merge(defs)
	if len(errs) > 0 {
		t.Fatalf("Merge: %v", errs)
	}
	all := ir.All()
	if len(all) != 1 {
		t.Fatalf("All(): want 1 def, got %d", len(all))
	}
	if all[0] == nil {
		t.Error("All()[0] is nil")
	}
	enum, ok := all[0].Primary.(*Enum)
	if !ok {
		t.Errorf("Primary is %T, want *Enum", all[0].Primary)
	}
	if enum.Name != "Color" {
		t.Errorf("Primary.Name = %q, want %q", enum.Name, "Color")
	}
}

func TestIR_All_multipleDefs(t *testing.T) {
	t.Parallel()
	defs, err := Parse(`
		enum Color { "red", "green" };
		enum Direction { "north", "south" };
	`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ir, errs := Merge(defs)
	if len(errs) > 0 {
		t.Fatalf("Merge: %v", errs)
	}
	all := ir.All()
	if len(all) != 2 {
		t.Fatalf("All(): want 2 defs, got %d", len(all))
	}
	names := make(map[string]bool)
	for _, d := range all {
		if d == nil {
			t.Error("All() returned nil entry")
			continue
		}
		switch p := d.Primary.(type) {
		case *Enum:
			names[p.Name] = true
		default:
			t.Errorf("unexpected primary type %T", d.Primary)
		}
	}
	for _, want := range []string{"Color", "Direction"} {
		if !names[want] {
			t.Errorf("All() missing %q", want)
		}
	}
}

func TestIR_All_countMatchesLen(t *testing.T) {
	t.Parallel()
	defs, err := Parse(`
		enum A { "a" };
		enum B { "b" };
		enum C { "c" };
	`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ir, errs := Merge(defs)
	if len(errs) > 0 {
		t.Fatalf("Merge: %v", errs)
	}
	if got, want := len(ir.All()), ir.Len(); got != want {
		t.Errorf("len(All()) = %d, Len() = %d: must match", got, want)
	}
}

// --- Adversary gap tests ---

// Mixin definitions are present in IR.defs (by design) so callers can resolve
// folded members. All() must return them so Generate() can filter them.
func TestIR_All_mixinPresentForFilteringByCallers(t *testing.T) {
	t.Parallel()
	// A mixin definition is not removed from the IR — callers like Generate()
	// are expected to filter by Primary.(*Interface).Variant != IfaceMixin.
	defs, err := Parse(`
		interface mixin Mixable {
			undefined doSomething();
		};
		interface MyInterface {};
		MyInterface includes Mixable;
	`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ir, errs := Merge(defs)
	if len(errs) > 0 {
		t.Fatalf("Merge: %v", errs)
	}
	all := ir.All()
	// IR holds both MyInterface and Mixable (the latter stays for member-folding).
	if len(all) < 1 {
		t.Fatalf("All(): expected at least 1 def, got 0")
	}
	var hasMixin bool
	for _, d := range all {
		iface, ok := d.Primary.(*Interface)
		if ok && iface.Variant == IfaceMixin {
			hasMixin = true
		}
	}
	if !hasMixin {
		t.Error("All(): mixin entry not present; Generate() won't have a chance to filter it")
	}
}

// All() must work with non-enum definition kinds.
func TestIR_All_dictionaryType(t *testing.T) {
	t.Parallel()
	defs, err := Parse(`dictionary Point { required long x; required long y; };`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ir, errs := Merge(defs)
	if len(errs) > 0 {
		t.Fatalf("Merge: %v", errs)
	}
	all := ir.All()
	if len(all) != 1 {
		t.Fatalf("All(): want 1 def, got %d", len(all))
	}
	if _, ok := all[0].Primary.(*Dictionary); !ok {
		t.Errorf("Primary is %T, want *Dictionary", all[0].Primary)
	}
}

// All() returns defs in deterministic (sorted) order — same answer on every call.
func TestIR_All_deterministicOrder(t *testing.T) {
	t.Parallel()
	defs, err := Parse(`
		enum Zebra { "z" };
		enum Apple { "a" };
		enum Mango { "m" };
	`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ir, errs := Merge(defs)
	if len(errs) > 0 {
		t.Fatalf("Merge: %v", errs)
	}
	first := namesFrom(ir.All())
	second := namesFrom(ir.All())
	if strings.Join(first, ",") != strings.Join(second, ",") {
		t.Errorf("All() is non-deterministic:\n  call 1: %v\n  call 2: %v", first, second)
	}
}

func namesFrom(defs []*MergedDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		switch p := d.Primary.(type) {
		case *Enum:
			out = append(out, p.Name)
		case *Dictionary:
			out = append(out, p.Name)
		case *Interface:
			out = append(out, p.Name)
		case *Typedef:
			out = append(out, p.Name)
		case *Namespace:
			out = append(out, p.Name)
		case *CallbackFunction:
			out = append(out, p.Name)
		}
	}
	return out
}
