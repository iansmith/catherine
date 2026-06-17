package typemap

import (
	"testing"

	"github.com/iansmith/webidl/webidl"
)

// ---------------------------------------------------------------------------
// GoType.String()
// ---------------------------------------------------------------------------

func TestGoTypeStringSimpleName(t *testing.T) {
	t.Parallel()
	gt := GoType{Name: "bool"}
	if got := gt.String(); got != "bool" {
		t.Errorf("GoType{Name:\"bool\"}.String() = %q, want %q", got, "bool")
	}
}

func TestGoTypeStringWithPointer(t *testing.T) {
	t.Parallel()
	gt := GoType{Name: "int32", Pointer: true}
	if got := gt.String(); got != "*int32" {
		t.Errorf("GoType{Name:\"int32\",Pointer:true}.String() = %q, want %q", got, "*int32")
	}
}

func TestGoTypeStringWithPkgPath(t *testing.T) {
	t.Parallel()
	// last segment of PkgPath is used as the qualifier
	gt := GoType{PkgPath: "net/http", Name: "Request"}
	if got := gt.String(); got != "http.Request" {
		t.Errorf("GoType{PkgPath:\"net/http\",Name:\"Request\"}.String() = %q, want %q", got, "http.Request")
	}
}

func TestGoTypeStringWithPkgPathAndPointer(t *testing.T) {
	t.Parallel()
	gt := GoType{PkgPath: "net/http", Name: "Request", Pointer: true}
	if got := gt.String(); got != "*http.Request" {
		t.Errorf("GoType{PkgPath:\"net/http\",Name:\"Request\",Pointer:true}.String() = %q, want %q", got, "*http.Request")
	}
}

func TestGoTypeStringSingleSegmentPkgPath(t *testing.T) {
	t.Parallel()
	// PkgPath with no slash: the whole path is the qualifier
	gt := GoType{PkgPath: "mypkg", Name: "Foo"}
	if got := gt.String(); got != "mypkg.Foo" {
		t.Errorf("GoType{PkgPath:\"mypkg\",Name:\"Foo\"}.String() = %q, want %q", got, "mypkg.Foo")
	}
}

func TestGoTypeStringDeepPkgPath(t *testing.T) {
	t.Parallel()
	gt := GoType{PkgPath: "github.com/iansmith/webidl/webidl", Name: "IDLType"}
	if got := gt.String(); got != "webidl.IDLType" {
		t.Errorf("GoType{PkgPath:\"github.com/iansmith/webidl/webidl\",Name:\"IDLType\"}.String() = %q, want %q", got, "webidl.IDLType")
	}
}

func TestGoTypeStringZeroValue(t *testing.T) {
	t.Parallel()
	// Zero-value GoType: Name="" → String() returns ""
	gt := GoType{}
	if got := gt.String(); got != "" {
		t.Errorf("GoType{}.String() = %q, want %q", got, "")
	}
}

// ---------------------------------------------------------------------------
// Mapper.MapType — error cases (boundary / rejection)
// ---------------------------------------------------------------------------

func TestMapTypeNilReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	_, err := m.MapType(nil)
	if err == nil {
		t.Error("MapType(nil) expected error, got nil")
	}
}

func TestMapTypeEmptyIDLTypeReturnsError(t *testing.T) {
	t.Parallel()
	// IDLType with Union=false, Generic="", Base="" — no dispatch branch can fire
	m := Mapper{}
	_, err := m.MapType(&webidl.IDLType{})
	if err == nil {
		t.Error("MapType(empty IDLType) expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Mapper.MapType — happy-path dispatch (no error, non-empty GoType)
// ---------------------------------------------------------------------------

func TestMapTypeUnionNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(union) returned error: %v", err)
	}
	if got.Name == "" {
		t.Error("MapType(union) returned GoType with empty Name")
	}
}

func TestMapTypeGenericSequenceNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "sequence",
		Subtypes: []*webidl.IDLType{{Base: "long"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(sequence<long>) returned error: %v", err)
	}
	if got.Name == "" {
		t.Error("MapType(sequence<long>) returned GoType with empty Name")
	}
}

func TestMapTypeGenericRecordNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(record<DOMString,long>) returned error: %v", err)
	}
	if got.Name == "" {
		t.Error("MapType(record<DOMString,long>) returned GoType with empty Name")
	}
}

func TestMapTypeGenericPromiseNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "Promise",
		Subtypes: []*webidl.IDLType{{Base: "long"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(Promise<long>) returned error: %v", err)
	}
	if got.Name == "" {
		t.Error("MapType(Promise<long>) returned GoType with empty Name")
	}
}

func TestMapTypeBaseNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{Base: "boolean"}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(boolean) returned error: %v", err)
	}
	if got.Name == "" {
		t.Error("MapType(boolean) returned GoType with empty Name")
	}
}

// ---------------------------------------------------------------------------
// Nullable handling — must not panic, must not error
// ---------------------------------------------------------------------------

func TestMapTypeNullableBaseNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{Base: "boolean", Nullable: true}
	_, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(boolean?) returned error: %v", err)
	}
}

func TestMapTypeNullableUnionNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Union:    true,
		Nullable: true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}},
	}
	_, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType((DOMString or long)?) returned error: %v", err)
	}
}

func TestMapTypeNullableGenericNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "sequence",
		Nullable: true,
		Subtypes: []*webidl.IDLType{{Base: "long"}},
	}
	_, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(sequence<long>?) returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Cross-feature: nested generics — must not panic, must not error
// ---------------------------------------------------------------------------

func TestMapTypeNestedSequenceNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	// sequence<sequence<long>>
	inner := &webidl.IDLType{Generic: "sequence", Subtypes: []*webidl.IDLType{{Base: "long"}}}
	outer := &webidl.IDLType{Generic: "sequence", Subtypes: []*webidl.IDLType{inner}}
	_, err := m.MapType(outer)
	if err != nil {
		t.Fatalf("MapType(sequence<sequence<long>>) returned error: %v", err)
	}
}

func TestMapTypeSequenceOfUnionNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	// sequence<(DOMString or long)>
	union := &webidl.IDLType{Union: true, Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}}}
	seq := &webidl.IDLType{Generic: "sequence", Subtypes: []*webidl.IDLType{union}}
	_, err := m.MapType(seq)
	if err != nil {
		t.Fatalf("MapType(sequence<(DOMString or long)>) returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Adversary gap tests
// ---------------------------------------------------------------------------

// GoType.String() with PkgPath set but Name empty produces only the qualifier prefix.
// This is an odd input but must not panic.
func TestGoTypeStringEmptyNameWithPkgPath(t *testing.T) {
	t.Parallel()
	gt := GoType{PkgPath: "net/http", Name: ""}
	got := gt.String() // must not panic; exact value up to implementation
	_ = got
}

// A generic IDLType with no Subtypes at all must not panic (the skeleton does not
// recurse into subtypes — that is CATH-46's job).
func TestMapTypeGenericEmptySubtypesNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{Generic: "sequence", Subtypes: nil}
	_, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(sequence with nil Subtypes) returned error: %v", err)
	}
}

// FrozenArray is a valid generic keyword; dispatch must handle it.
func TestMapTypeGenericFrozenArrayNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "FrozenArray",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}},
	}
	_, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(FrozenArray<DOMString>) returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// pkgQualifier / versioned module paths (fix for path.Base bug)
// ---------------------------------------------------------------------------

// gopkg.in-style versioned path: last segment contains a dot ("yaml.v3").
func TestGoTypeStringVersionedDotPkgPath(t *testing.T) {
	t.Parallel()
	gt := GoType{PkgPath: "gopkg.in/yaml.v3", Name: "Node"}
	if got := gt.String(); got != "yaml.Node" {
		t.Errorf("GoType{PkgPath:\"gopkg.in/yaml.v3\",Name:\"Node\"}.String() = %q, want %q", got, "yaml.Node")
	}
}

// github.com-style major-version path: last segment is a bare "vN" directory.
func TestGoTypeStringMajorVersionPkgPath(t *testing.T) {
	t.Parallel()
	gt := GoType{PkgPath: "github.com/user/repo/v2", Name: "Client"}
	if got := gt.String(); got != "repo.Client" {
		t.Errorf("GoType{PkgPath:\"github.com/user/repo/v2\",Name:\"Client\"}.String() = %q, want %q", got, "repo.Client")
	}
}

// ---------------------------------------------------------------------------
// MapType — malformed node guard (Union + Generic both set)
// ---------------------------------------------------------------------------

func TestMapTypeUnionAndGenericBothSetReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Union:    true,
		Generic:  "sequence",
		Subtypes: []*webidl.IDLType{{Base: "long"}},
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(Union=true, Generic=\"sequence\") expected error for malformed node, got nil")
	}
}

// ---------------------------------------------------------------------------
// Mapper value-receiver ergonomics
// ---------------------------------------------------------------------------

// Mapper zero value can be used via a named variable — no initializer needed.
// (Separate from the pointer-receiver compile-time check handled by the Go compiler.)
func TestMapperZeroValueUsable(t *testing.T) {
	t.Parallel()
	var m Mapper
	_, err := m.MapType(&webidl.IDLType{Base: "boolean"})
	if err != nil {
		t.Fatalf("var m Mapper; m.MapType(boolean) returned error: %v", err)
	}
}

// async_sequence is rejected by validate.go but may still arrive; must not panic.
func TestMapTypeGenericAsyncSequenceNoError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "async_sequence",
		Subtypes: []*webidl.IDLType{{Base: "long"}},
	}
	// No assertion on error — async_sequence may legitimately produce a diagnostic.
	// The requirement is only that MapType does not panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MapType(async_sequence<long>) panicked: %v", r)
			}
		}()
		_, _ = m.MapType(idlType)
	}()
}

// ---------------------------------------------------------------------------
// CATH-44: Scalar primitive type mappings
// ---------------------------------------------------------------------------

// TestScalarGoTypesAllValuesInValueTypeNames enforces that every Go type name
// produced by scalarGoTypes appears in valueTypeNames. Without this invariant,
// adding a new IDL scalar mapping without updating valueTypeNames would silently
// break nullable pointer promotion for that type.
func TestScalarGoTypesAllValuesInValueTypeNames(t *testing.T) {
	t.Parallel()
	for base, goName := range scalarGoTypes {
		if !valueTypeNames[goName] {
			t.Errorf("scalarGoTypes[%q]=%q not in valueTypeNames; nullable scalar would silently skip pointer promotion", base, goName)
		}
	}
}

// TestMapTypeScalarExact verifies every IDL primitive scalar base maps to the
// correct predeclared Go type.
func TestMapTypeScalarExact(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	for base, want := range scalarGoTypes {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			got, err := m.MapType(&webidl.IDLType{Base: base})
			if err != nil {
				t.Fatalf("MapType(%q) returned error: %v", base, err)
			}
			if got.Name != want {
				t.Errorf("MapType(%q).Name = %q, want %q", base, got.Name, want)
			}
			if got.PkgPath != "" {
				t.Errorf("MapType(%q).PkgPath = %q, want \"\" (predeclared type)", base, got.PkgPath)
			}
			if got.Pointer {
				t.Errorf("MapType(%q).Pointer = true, want false for non-nullable", base)
			}
		})
	}
}

// TestMapTypeUnrestrictedFloatIdenticalToFloat verifies that "float" and
// "unrestricted float" map to the same GoType struct in every field.
func TestMapTypeUnrestrictedFloatIdenticalToFloat(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	restricted, err := m.MapType(&webidl.IDLType{Base: "float"})
	if err != nil {
		t.Fatalf("MapType(float) error: %v", err)
	}
	unrestricted, err := m.MapType(&webidl.IDLType{Base: "unrestricted float"})
	if err != nil {
		t.Fatalf("MapType(unrestricted float) error: %v", err)
	}
	want := GoType{Name: "float32"}
	if restricted != want {
		t.Errorf("MapType(float) = %+v, want %+v", restricted, want)
	}
	if unrestricted != want {
		t.Errorf("MapType(unrestricted float) = %+v, want %+v", unrestricted, want)
	}
}

// TestMapTypeUnrestrictedDoubleIdenticalToDouble verifies that "double" and
// "unrestricted double" map to the same GoType struct in every field.
func TestMapTypeUnrestrictedDoubleIdenticalToDouble(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	restricted, err := m.MapType(&webidl.IDLType{Base: "double"})
	if err != nil {
		t.Fatalf("MapType(double) error: %v", err)
	}
	unrestricted, err := m.MapType(&webidl.IDLType{Base: "unrestricted double"})
	if err != nil {
		t.Fatalf("MapType(unrestricted double) error: %v", err)
	}
	want := GoType{Name: "float64"}
	if restricted != want {
		t.Errorf("MapType(double) = %+v, want %+v", restricted, want)
	}
	if unrestricted != want {
		t.Errorf("MapType(unrestricted double) = %+v, want %+v", unrestricted, want)
	}
}

// TestMapTypeScalarNullableBecomesPointer verifies that nullable scalar IDL
// types produce GoType.Pointer == true (T → *T).
func TestMapTypeScalarNullableBecomesPointer(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	for base, want := range scalarGoTypes {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			got, err := m.MapType(&webidl.IDLType{Base: base, Nullable: true})
			if err != nil {
				t.Fatalf("MapType(%q?) returned error: %v", base, err)
			}
			if got.Name != want {
				t.Errorf("MapType(%q?).Name = %q, want %q", base, got.Name, want)
			}
			if got.PkgPath != "" {
				t.Errorf("MapType(%q?).PkgPath = %q, want \"\" (predeclared type)", base, got.PkgPath)
			}
			if !got.Pointer {
				t.Errorf("MapType(%q?).Pointer = false, want true for nullable scalar", base)
			}
		})
	}
}
