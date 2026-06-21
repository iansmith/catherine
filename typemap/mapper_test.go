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
	if !got.Unresolved {
		t.Error("MapType(union stub).Unresolved = false; stub must be marked Unresolved")
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
	if !got.Unresolved {
		t.Error("MapType(sequence<long> stub).Unresolved = false; stub must be marked Unresolved")
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
	if !got.Unresolved {
		t.Error("MapType(record<DOMString,long> stub).Unresolved = false; stub must be marked Unresolved")
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
	if !got.Unresolved {
		t.Error("MapType(Promise<long> stub).Unresolved = false; stub must be marked Unresolved")
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

// ---------------------------------------------------------------------------
// CATH-45: String and special/sentinel type mappings
// ---------------------------------------------------------------------------

// --- Edge / boundary ---

// TestMapTypeNonScalarBaseNullablePromotion verifies nullable promotion for all
// string and special/sentinel type bases. string is a value type so T? → *T;
// any is a reference type so T? → T (no extra pointer). Expected pointer is
// derived from valueTypeNames, keeping the test in sync with the promotion logic.
func TestMapTypeNonScalarBaseNullablePromotion(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	for base, goName := range nonScalarGoTypes {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			got, err := m.MapType(&webidl.IDLType{Base: base, Nullable: true})
			if err != nil {
				t.Fatalf("MapType(%q?) returned error: %v", base, err)
			}
			if got.Name != goName {
				t.Errorf("MapType(%q?).Name = %q, want %q", base, got.Name, goName)
			}
			if got.PkgPath != "" {
				t.Errorf("MapType(%q?).PkgPath = %q, want \"\"", base, got.PkgPath)
			}
			wantPointer := valueTypeNames[goName]
			if got.Pointer != wantPointer {
				t.Errorf("MapType(%q?).Pointer = %v, want %v", base, got.Pointer, wantPointer)
			}
		})
	}
}

// --- Error / rejection ---

// TestMapTypeUnknownBaseStillFallsThrough ensures that adding explicit string/special
// maps does not break the existing fallback for unrecognised bases (interface names, etc.).
func TestMapTypeUnknownBaseStillFallsThrough(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	// "EventTarget" is a WebIDL interface name — not a scalar, string, or special type.
	got, err := m.MapType(&webidl.IDLType{Base: "EventTarget"})
	if err != nil {
		t.Fatalf("MapType(EventTarget) returned error: %v", err)
	}
	if got.Name == "" {
		t.Error("MapType(EventTarget) returned GoType with empty Name; fallback expected")
	}
	if !got.Unresolved {
		t.Error("MapType(EventTarget).Unresolved = false; unrecognised bases must be marked Unresolved so codegen can distinguish them from intentional any mappings")
	}
}

// --- Cross-feature interaction ---

// TestMapTypeStringTypeInUnionSubtype verifies that string bases as union subtypes
// do not cause dispatch errors (the union stub ignores subtypes, but must not panic).
func TestMapTypeStringTypeInUnionSubtype(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}},
	}
	_, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType((DOMString or long)) returned error: %v", err)
	}
}

// TestMapTypeStringTypeInGenericSubtype verifies that string bases inside generic
// types do not cause dispatch errors.
func TestMapTypeStringTypeInGenericSubtype(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "sequence",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}},
	}
	_, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(sequence<DOMString>) returned error: %v", err)
	}
}

// --- Happy path ---

// TestMapTypeNonScalarBasesExact verifies every IDL string and special/sentinel type
// base maps to the correct predeclared Go type with no package path and no pointer.
func TestMapTypeNonScalarBasesExact(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	for base, want := range nonScalarGoTypes {
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
				t.Errorf("MapType(%q).PkgPath = %q, want \"\"", base, got.PkgPath)
			}
			if got.Pointer {
				t.Errorf("MapType(%q).Pointer = true, want false for non-nullable", base)
			}
			if got.Unresolved {
				t.Errorf("MapType(%q).Unresolved = true; explicitly mapped types must not be marked unresolved", base)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CATH-45: Adversary gap tests
// ---------------------------------------------------------------------------

// TestNonScalarGoTypesValueNameInvariant enforces that valueTypeNames is consistent
// with nonScalarGoTypes: string entries must be value types (nullable → *T); any
// entries must not be (any is already nil-able). A future edit adding a new Go type
// name to nonScalarGoTypes without classifying it here will fail the default case.
func TestNonScalarGoTypesValueNameInvariant(t *testing.T) {
	t.Parallel()
	for base, goName := range nonScalarGoTypes {
		switch goName {
		case "string":
			if !valueTypeNames[goName] {
				t.Errorf("nonScalarGoTypes[%q]=%q not in valueTypeNames; nullable would skip pointer promotion", base, goName)
			}
		case "any":
			if valueTypeNames[goName] {
				t.Errorf("nonScalarGoTypes[%q]=%q is in valueTypeNames; nullable any would incorrectly gain a pointer", base, goName)
			}
		default:
			t.Errorf("nonScalarGoTypes[%q]=%q is an unclassified Go type; update this test to specify whether it is a value type", base, goName)
		}
	}
}

// TestNonScalarBasesAbsentFromScalarGoTypes guards against accidentally adding string
// or special type bases to scalarGoTypes, which would route them through the wrong
// dispatch branch while tests still passed.
func TestNonScalarBasesAbsentFromScalarGoTypes(t *testing.T) {
	t.Parallel()
	for base := range nonScalarGoTypes {
		if _, ok := scalarGoTypes[base]; ok {
			t.Errorf("scalarGoTypes contains %q; non-scalar bases must not be in the scalar map (wrong dispatch path)", base)
		}
	}
}

// TestMapTypeByteStringMapsToStringNotBytes is a standalone contract test for the
// ByteString design decision: ByteString → string (not []byte). See CATH-45.
func TestMapTypeByteStringMapsToStringNotBytes(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	want := GoType{Name: "string"}
	got, err := m.MapType(&webidl.IDLType{Base: "ByteString"})
	if err != nil {
		t.Fatalf("MapType(ByteString) returned error: %v", err)
	}
	if got != want {
		t.Errorf("MapType(ByteString) = %+v, want %+v (must be string, not []byte)", got, want)
	}
	wantNullable := GoType{Name: "string", Pointer: true}
	gotNullable, err := m.MapType(&webidl.IDLType{Base: "ByteString", Nullable: true})
	if err != nil {
		t.Fatalf("MapType(ByteString?) returned error: %v", err)
	}
	if gotNullable != wantNullable {
		t.Errorf("MapType(ByteString?) = %+v, want %+v", gotNullable, wantNullable)
	}
}

// TestMapTypeDOMStringMapsToString is a hardcoded oracle for DOMString — the most
// common IDL string type. Unlike the table-driven test, this catches wrong values
// in nonScalarGoTypes itself (e.g. DOMString remapped to "[]byte").
func TestMapTypeDOMStringMapsToString(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	want := GoType{Name: "string"}
	got, err := m.MapType(&webidl.IDLType{Base: "DOMString"})
	if err != nil {
		t.Fatalf("MapType(DOMString) returned error: %v", err)
	}
	if got != want {
		t.Errorf("MapType(DOMString) = %+v, want %+v", got, want)
	}
}

// TestMapTypeVoidMapsToAny documents that void → GoType{Name:"any", Unresolved:false}:
// an intentional mapping, not a fallback. Unresolved=false distinguishes it from
// unrecognised interface names, which the codegen layer (CATH-46+) must treat differently.
func TestMapTypeVoidMapsToAny(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	want := GoType{Name: "any"}
	got, err := m.MapType(&webidl.IDLType{Base: "void"})
	if err != nil {
		t.Fatalf("MapType(void) returned error: %v", err)
	}
	if got != want {
		t.Errorf("MapType(void) = %+v, want %+v", got, want)
	}
}

// TestWebIDLStringTypesCoveredByNonScalarGoTypes ensures that every IDL string type
// recognized by the webidl package (webidl.StringTypes) is explicitly mapped to
// "string" in nonScalarGoTypes. A new entry in webidl.StringTypes that is missing
// here would silently map to GoType{Name:"any", Unresolved:true} instead of "string".
func TestWebIDLStringTypesCoveredByNonScalarGoTypes(t *testing.T) {
	t.Parallel()
	for _, base := range webidl.StringTypes {
		if nonScalarGoTypes[base] != "string" {
			t.Errorf("nonScalarGoTypes[%q] = %q, want \"string\"; add it when webidl.StringTypes gains a new entry", base, nonScalarGoTypes[base])
		}
	}
}

// TestMapTypeCSSOMStringMapsToString guards the CSSOMString entry: it is not in
// webidl.StringTypes (a tokenizer concept), so TestWebIDLStringTypesCoveredByNonScalarGoTypes
// does not protect it.
func TestMapTypeCSSOMStringMapsToString(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	want := GoType{Name: "string"}
	got, err := m.MapType(&webidl.IDLType{Base: "CSSOMString"})
	if err != nil {
		t.Fatalf("MapType(CSSOMString) returned error: %v", err)
	}
	if got != want {
		t.Errorf("MapType(CSSOMString) = %+v, want %+v", got, want)
	}
}
