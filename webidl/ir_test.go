package webidl

import (
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
