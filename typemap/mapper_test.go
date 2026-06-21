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

func TestMapTypeUnionResolved(t *testing.T) {
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
	if got.Name != "any" {
		t.Errorf("MapType(union).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(union).Unresolved = true; union→any is an intentional mapping, not an unresolved stub")
	}
}

// ---------------------------------------------------------------------------
// CATH-48: union type mapping — (A or B or C) → any (intentional, Unresolved:false)
// ---------------------------------------------------------------------------

// --- Edge / boundary ---

// TestMapTypeUnionNilSubtypes verifies that a union node with no members does
// not panic and returns an intentional any (zero members is unusual but must
// not crash).
func TestMapTypeUnionNilSubtypes(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{Union: true, Subtypes: nil}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(union nil subtypes) returned error: %v", err)
	}
	if got.Name != "any" {
		t.Errorf("MapType(union nil subtypes).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(union nil subtypes).Unresolved = true; must be intentional (false)")
	}
}

// TestMapTypeUnionSingleMember verifies that a union with exactly one member
// still returns an intentional any.
func TestMapTypeUnionSingleMember(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(union single member) returned error: %v", err)
	}
	if got.Name != "any" {
		t.Errorf("MapType(union single member).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(union single member).Unresolved = true; must be intentional (false)")
	}
}

// --- Cross-feature ---

// TestMapTypeUnionNullableNoPointer verifies that nullable union types do not
// gain an extra pointer — any is already a reference type.
func TestMapTypeUnionNullableNoPointer(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Union:    true,
		Nullable: true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType((DOMString or long)?) returned error: %v", err)
	}
	if got.Pointer {
		t.Error("MapType((DOMString or long)?).Pointer = true; any is already reference-typed, must not gain extra pointer")
	}
	if got.Unresolved {
		t.Error("MapType((DOMString or long)?).Unresolved = true; must be intentional (false)")
	}
}

// --- Happy path ---

// TestMapTypeUnionThreeMembers verifies that a three-member union produces an
// intentional any, identical to a two-member union.
func TestMapTypeUnionThreeMembers(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}, {Base: "boolean"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType((DOMString or long or boolean)) returned error: %v", err)
	}
	if got.Name != "any" {
		t.Errorf("MapType((DOMString or long or boolean)).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType((DOMString or long or boolean)).Unresolved = true; must be intentional (false)")
	}
}

// TestMapTypeUnionNestedUnion verifies that a nested union (a union whose
// member is itself a union) does not panic and still produces any.
func TestMapTypeUnionNestedUnion(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	inner := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "long"}, {Base: "boolean"}},
	}
	outer := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, inner},
	}
	got, err := m.MapType(outer)
	if err != nil {
		t.Fatalf("MapType(nested union) returned error: %v", err)
	}
	if got.Name != "any" {
		t.Errorf("MapType(nested union).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(nested union).Unresolved = true; must be intentional (false)")
	}
}

// ---------------------------------------------------------------------------
// CATH-48: extended-attribute type modifiers ([Clamp], [EnforceRange], [AllowShared])
// ---------------------------------------------------------------------------

// --- Edge / boundary ---

// TestMapTypeExtAttrUnknownIgnored verifies that an unrecognised extended
// attribute leaves the resolved type unchanged and does not set Annotation.
func TestMapTypeExtAttrUnknownIgnored(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "long",
		ExtAttrs: []*webidl.ExtAttr{{Name: "SomeUnknownAttr"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([SomeUnknownAttr] long) returned error: %v", err)
	}
	if got.Name != "int32" {
		t.Errorf("MapType([SomeUnknownAttr] long).Name = %q, want \"int32\"", got.Name)
	}
	if got.Annotation != "" {
		t.Errorf("MapType([SomeUnknownAttr] long).Annotation = %q, want \"\"", got.Annotation)
	}
}

// TestMapTypeExtAttrNoExtAttrs verifies the baseline: no ExtAttrs means
// Annotation is empty and the resolved type is unchanged.
func TestMapTypeExtAttrNoExtAttrs(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{Base: "unsigned short"}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(unsigned short) returned error: %v", err)
	}
	if got.Name != "uint16" {
		t.Errorf("MapType(unsigned short).Name = %q, want \"uint16\"", got.Name)
	}
	if got.Annotation != "" {
		t.Errorf("MapType(unsigned short).Annotation = %q, want \"\"", got.Annotation)
	}
}

// --- Cross-feature ---

// TestMapTypeExtAttrNullableWithClamp verifies that [Clamp] and nullable
// interact correctly: the numeric type gains a pointer (nullable scalar) AND
// the Annotation records "Clamp".
func TestMapTypeExtAttrNullableWithClamp(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "unsigned short",
		Nullable: true,
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrClamp}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([Clamp] unsigned short?) returned error: %v", err)
	}
	if got.Name != "uint16" {
		t.Errorf("MapType([Clamp] unsigned short?).Name = %q, want \"uint16\"", got.Name)
	}
	if !got.Pointer {
		t.Error("MapType([Clamp] unsigned short?).Pointer = false; nullable scalar must be pointer-wrapped")
	}
	if got.Annotation != webidl.ExtAttrClamp {
		t.Errorf("MapType([Clamp] unsigned short?).Annotation = %q, want \"Clamp\"", got.Annotation)
	}
}

// --- Happy path ---

// TestMapTypeExtAttrClampPreservesType verifies that [Clamp] on a numeric type
// preserves the resolved Go type and sets Annotation to "Clamp".
func TestMapTypeExtAttrClampPreservesType(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "unsigned short",
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrClamp}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([Clamp] unsigned short) returned error: %v", err)
	}
	if got.Name != "uint16" {
		t.Errorf("MapType([Clamp] unsigned short).Name = %q, want \"uint16\"", got.Name)
	}
	if got.Annotation != webidl.ExtAttrClamp {
		t.Errorf("MapType([Clamp] unsigned short).Annotation = %q, want \"Clamp\"", got.Annotation)
	}
}

// TestMapTypeExtAttrEnforceRangePreservesType verifies that [EnforceRange] on a
// numeric type preserves the resolved Go type and sets Annotation to "EnforceRange".
func TestMapTypeExtAttrEnforceRangePreservesType(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "long",
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrEnforceRange}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([EnforceRange] long) returned error: %v", err)
	}
	if got.Name != "int32" {
		t.Errorf("MapType([EnforceRange] long).Name = %q, want \"int32\"", got.Name)
	}
	if got.Annotation != webidl.ExtAttrEnforceRange {
		t.Errorf("MapType([EnforceRange] long).Annotation = %q, want \"EnforceRange\"", got.Annotation)
	}
}

// TestMapTypeExtAttrAllowShared verifies that [AllowShared] sets Annotation to
// "AllowShared". The Go type may be unresolved (ArrayBuffer is not in the
// scalar or string maps), but Annotation must still be recorded.
func TestMapTypeExtAttrAllowShared(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "ArrayBuffer",
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrAllowShared}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([AllowShared] ArrayBuffer) returned error: %v", err)
	}
	if got.Annotation != webidl.ExtAttrAllowShared {
		t.Errorf("MapType([AllowShared] ArrayBuffer).Annotation = %q, want \"AllowShared\"", got.Annotation)
	}
}

// ---------------------------------------------------------------------------
// CATH-48: adversary gap tests
// ---------------------------------------------------------------------------

// Gap 1: union returns exactly GoType{Name:"any"} — no Pointer, no Annotation
func TestMapTypeUnionReturnsExactGoType(t *testing.T) {
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
	want := GoType{Name: "any"}
	if got != want {
		t.Errorf("MapType(union) = %#v, want %#v", got, want)
	}
}

// Gap 2: union with a malformed member must not propagate an error — union→any
// discards member types entirely; it must not recurse into them.
func TestMapTypeUnionWithMalformedMemberStillReturnsAny(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	malformed := &webidl.IDLType{Union: true, Generic: "sequence"} // both set = malformed
	outer := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, malformed},
	}
	got, err := m.MapType(outer)
	if err != nil {
		t.Fatalf("MapType(union with malformed member) returned error: %v; union→any must not recurse into members", err)
	}
	if got.Name != "any" {
		t.Errorf("MapType(union with malformed member).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(union with malformed member).Unresolved = true; must be intentional (false)")
	}
}

// Gap 3a: [Clamp] on a signed type — tests annotation isn't keyed on a specific type name.
func TestMapTypeExtAttrClampOnSignedLong(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "long",
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrClamp}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([Clamp] long) returned error: %v", err)
	}
	if got.Name != "int32" {
		t.Errorf("MapType([Clamp] long).Name = %q, want \"int32\"", got.Name)
	}
	if got.Annotation != webidl.ExtAttrClamp {
		t.Errorf("MapType([Clamp] long).Annotation = %q, want \"Clamp\"", got.Annotation)
	}
}

// Gap 3b: [EnforceRange] on an unsigned type — tests annotation isn't keyed on specific names.
func TestMapTypeExtAttrEnforceRangeOnUnsignedLong(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "unsigned long",
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrEnforceRange}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([EnforceRange] unsigned long) returned error: %v", err)
	}
	if got.Name != "uint32" {
		t.Errorf("MapType([EnforceRange] unsigned long).Name = %q, want \"uint32\"", got.Name)
	}
	if got.Annotation != webidl.ExtAttrEnforceRange {
		t.Errorf("MapType([EnforceRange] unsigned long).Annotation = %q, want \"EnforceRange\"", got.Annotation)
	}
}

// Gap 4: [AllowShared] on a resolved base type (not just the unresolved fallback).
func TestMapTypeExtAttrAllowSharedOnResolvedType(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "octet",
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrAllowShared}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([AllowShared] octet) returned error: %v", err)
	}
	if got.Name != "uint8" {
		t.Errorf("MapType([AllowShared] octet).Name = %q, want \"uint8\"", got.Name)
	}
	if got.Annotation != webidl.ExtAttrAllowShared {
		t.Errorf("MapType([AllowShared] octet).Annotation = %q, want \"AllowShared\"", got.Annotation)
	}
}

// Gap 6: [Clamp] ExtAttr on a union node must not set Annotation — annotation
// post-processing must not bleed into the union→any dispatch path.
func TestMapTypeUnionWithClampExtAttrNoAnnotation(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}},
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrClamp}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([Clamp] (DOMString or long)) returned error: %v", err)
	}
	if got.Annotation != "" {
		t.Errorf("MapType([Clamp] union).Annotation = %q, want \"\"; annotation must not bleed onto union→any", got.Annotation)
	}
	if got.Unresolved {
		t.Error("MapType([Clamp] union).Unresolved = true; must be intentional (false)")
	}
}

// Gap 7: companion test for AllowShared verifying the base type name and Unresolved state.
func TestMapTypeExtAttrAllowSharedBaseTypeIsUnresolvedAny(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Base:     "ArrayBuffer",
		ExtAttrs: []*webidl.ExtAttr{{Name: webidl.ExtAttrAllowShared}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType([AllowShared] ArrayBuffer) returned error: %v", err)
	}
	if got.Name != "any" {
		t.Errorf("MapType([AllowShared] ArrayBuffer).Name = %q, want \"any\" (unresolved fallback)", got.Name)
	}
	if !got.Unresolved {
		t.Error("MapType([AllowShared] ArrayBuffer).Unresolved = false; ArrayBuffer has no known Go mapping, must be Unresolved")
	}
}

func TestMapTypeGenericRecordResolved(t *testing.T) {
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
	if got.Name != "map[string]int32" {
		t.Errorf("MapType(record<DOMString,long>).Name = %q, want \"map[string]int32\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(record<DOMString,long>).Unresolved = true; fully resolved record must not be marked Unresolved")
	}
}

func TestMapTypeGenericPromiseResolved(t *testing.T) {
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
	if got.Name != "any" {
		t.Errorf("MapType(Promise<long>).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(Promise<long>).Unresolved = true; Promise→any is intentional (not a stub)")
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
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(sequence<long>?) returned error: %v", err)
	}
	if got.Name != "[]int32" {
		t.Errorf("MapType(sequence<long>?).Name = %q, want \"[]int32\"", got.Name)
	}
	if got.Pointer {
		t.Error("MapType(sequence<long>?).Pointer = true; slices are reference types and must not gain an extra pointer layer")
	}
}

// ---------------------------------------------------------------------------
// Cross-feature: nested generics — must not panic, must not error
// ---------------------------------------------------------------------------

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

// A sequence IDLType with no Subtypes is a malformed node — MapType must return an error.
func TestMapTypeSequenceEmptySubtypesReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{Generic: "sequence", Subtypes: nil}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(sequence with nil Subtypes) expected error for malformed node, got nil")
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

// ---------------------------------------------------------------------------
// CATH-46: Generic sequence type mappings
// ---------------------------------------------------------------------------

// TestMapTypeSequenceExact verifies that sequence, FrozenArray, and ObservableArray
// each map to the correct []T Go slice type with Unresolved=false.
func TestMapTypeSequenceExact(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cases := []struct {
		generic  string
		elemBase string
		wantName string
	}{
		{"sequence", "long", "[]int32"},
		{"FrozenArray", "DOMString", "[]string"},
		{"ObservableArray", "boolean", "[]bool"},
	}
	for _, tc := range cases {
		t.Run(tc.generic+"<"+tc.elemBase+">", func(t *testing.T) {
			t.Parallel()
			idlType := &webidl.IDLType{
				Generic:  tc.generic,
				Subtypes: []*webidl.IDLType{{Base: tc.elemBase}},
			}
			got, err := m.MapType(idlType)
			if err != nil {
				t.Fatalf("MapType(%s<%s>) returned error: %v", tc.generic, tc.elemBase, err)
			}
			if got.Name != tc.wantName {
				t.Errorf("MapType(%s<%s>).Name = %q, want %q", tc.generic, tc.elemBase, got.Name, tc.wantName)
			}
			if got.Unresolved {
				t.Errorf("MapType(%s<%s>).Unresolved = true; resolved sequence must not be marked Unresolved", tc.generic, tc.elemBase)
			}
		})
	}
}

// TestMapTypeSequenceNested verifies that sequence<sequence<boolean>> maps to [][]bool.
func TestMapTypeSequenceNested(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	inner := &webidl.IDLType{Generic: "sequence", Subtypes: []*webidl.IDLType{{Base: "boolean"}}}
	outer := &webidl.IDLType{Generic: "sequence", Subtypes: []*webidl.IDLType{inner}}
	got, err := m.MapType(outer)
	if err != nil {
		t.Fatalf("MapType(sequence<sequence<boolean>>) returned error: %v", err)
	}
	if got.Name != "[][]bool" {
		t.Errorf("MapType(sequence<sequence<boolean>>).Name = %q, want \"[][]bool\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(sequence<sequence<boolean>>).Unresolved = true; resolved nested sequence must not be marked Unresolved")
	}
}

// TestMapTypeSequenceUnresolvedElement verifies that sequence<EventTarget> maps to
// []any with Unresolved=true, propagating the element's unresolved state.
func TestMapTypeSequenceUnresolvedElement(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "sequence",
		Subtypes: []*webidl.IDLType{{Base: "EventTarget"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(sequence<EventTarget>) returned error: %v", err)
	}
	if got.Name != "[]any" {
		t.Errorf("MapType(sequence<EventTarget>).Name = %q, want \"[]any\"", got.Name)
	}
	if !got.Unresolved {
		t.Error("MapType(sequence<EventTarget>).Unresolved = false; unresolved element must propagate Unresolved to the slice")
	}
}

// TestMapTypeSequenceNullableElement verifies that sequence<boolean?> maps to []*bool:
// the nullable element becomes *bool, and the slice wraps it as []*bool.
func TestMapTypeSequenceNullableElement(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "sequence",
		Subtypes: []*webidl.IDLType{{Base: "boolean", Nullable: true}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(sequence<boolean?>) returned error: %v", err)
	}
	if got.Name != "[]*bool" {
		t.Errorf("MapType(sequence<boolean?>).Name = %q, want \"[]*bool\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(sequence<boolean?>).Unresolved = true; resolved sequence must not be marked Unresolved")
	}
}

// TestMapTypeAsyncSequenceReturnsError verifies that async_sequence produces an error
// (it is IDL-to-JS only and should have been rejected by validate.go).
func TestMapTypeAsyncSequenceReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "async_sequence",
		Subtypes: []*webidl.IDLType{{Base: "long"}},
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(async_sequence<long>) expected error (IDL-to-JS only), got nil")
	}
}

// ---------------------------------------------------------------------------
// CATH-47: record<K,V> → map[string]V
// ---------------------------------------------------------------------------

// --- Edge / boundary ---

// TestMapTypeRecordNonStringKeyReturnsError verifies that a record with a non-string
// key type returns an error. WebIDL allows only DOMString, USVString, or ByteString
// as key types; any other base must be rejected.
func TestMapTypeRecordNonStringKeyReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "long"}, {Base: "boolean"}},
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(record<long,boolean>) expected error (non-string key type), got nil")
	}
}

// TestMapTypeRecordNullableNoPointer verifies that a nullable record does NOT gain an
// extra pointer. map[K]V is already a reference type and is nil-able without wrapping.
func TestMapTypeRecordNullableNoPointer(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Nullable: true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(record<DOMString,long>?) returned error: %v", err)
	}
	if got.Name != "map[string]int32" {
		t.Errorf("MapType(record<DOMString,long>?).Name = %q, want \"map[string]int32\"", got.Name)
	}
	if got.Pointer {
		t.Error("MapType(record<DOMString,long>?).Pointer = true; maps are reference types and must not gain an extra pointer")
	}
}

// TestMapTypeRecordNullableValueElement verifies that a nullable element type inside
// a record is promoted to a pointer: record<DOMString, boolean?> → map[string]*bool.
func TestMapTypeRecordNullableValueElement(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "boolean", Nullable: true}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(record<DOMString,boolean?>) returned error: %v", err)
	}
	if got.Name != "map[string]*bool" {
		t.Errorf("MapType(record<DOMString,boolean?>).Name = %q, want \"map[string]*bool\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(record<DOMString,boolean?>).Unresolved = true; resolved record must not be marked Unresolved")
	}
}

// --- Error / rejection ---

// TestMapTypeRecordNoSubtypesReturnsError verifies that a record with nil Subtypes
// returns an error (malformed node: no type parameters).
func TestMapTypeRecordNoSubtypesReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: nil,
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(record with nil Subtypes) expected error for malformed node, got nil")
	}
}

// TestMapTypeRecordOneSubtypeReturnsError verifies that a record with only one subtype
// (missing the value type) returns an error.
func TestMapTypeRecordOneSubtypeReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}},
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(record with one Subtype) expected error for malformed node (missing value type), got nil")
	}
}

// TestMapTypeRecordThreeSubtypesReturnsError verifies that a record with 3 subtypes
// (more than the required exactly-2) returns an error rather than silently ignoring the extra.
func TestMapTypeRecordThreeSubtypesReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "long"}, {Base: "boolean"}},
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(record with 3 Subtypes) expected error for malformed node (too many type parameters), got nil")
	}
}

// TestMapTypeRecordCSSOMStringKeyReturnsError verifies that CSSOMString is rejected as a
// record key type. The WebIDL spec (§3.2.26) permits only DOMString, USVString, and
// ByteString; CSSOMString is a string type in the broader mapper but is not a valid
// record key.
func TestMapTypeRecordCSSOMStringKeyReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "CSSOMString"}, {Base: "long"}},
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(record<CSSOMString,long>) expected error (CSSOMString not a valid record key per WebIDL §3.2.26), got nil")
	}
}

// TestMapTypePromiseInvalidSubtypeReturnsError verifies that an error in the Promise
// type parameter is propagated rather than silently swallowed. async_sequence is
// IDL-to-JS only and should produce an error even when nested inside Promise.
func TestMapTypePromiseInvalidSubtypeReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	asyncSeq := &webidl.IDLType{
		Generic:  "async_sequence",
		Subtypes: []*webidl.IDLType{{Base: "long"}},
	}
	idlType := &webidl.IDLType{
		Generic:  "Promise",
		Subtypes: []*webidl.IDLType{asyncSeq},
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(Promise<async_sequence<long>>) expected error (async_sequence is IDL-to-JS only), got nil")
	}
}

// --- Cross-feature ---

// TestMapTypeRecordNestedValue verifies recursive value type resolution:
// record<DOMString, sequence<long>> → map[string][]int32.
func TestMapTypeRecordNestedValue(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	seqLong := &webidl.IDLType{Generic: "sequence", Subtypes: []*webidl.IDLType{{Base: "long"}}}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, seqLong},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(record<DOMString,sequence<long>>) returned error: %v", err)
	}
	if got.Name != "map[string][]int32" {
		t.Errorf("MapType(record<DOMString,sequence<long>>).Name = %q, want \"map[string][]int32\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(record<DOMString,sequence<long>>).Unresolved = true; resolved record must not be marked Unresolved")
	}
}

// TestMapTypeRecordNestedRecord verifies doubly-nested record resolution:
// record<DOMString, record<USVString, long>> → map[string]map[string]int32.
func TestMapTypeRecordNestedRecord(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	inner := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "USVString"}, {Base: "long"}},
	}
	outer := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, inner},
	}
	got, err := m.MapType(outer)
	if err != nil {
		t.Fatalf("MapType(record<DOMString,record<USVString,long>>) returned error: %v", err)
	}
	if got.Name != "map[string]map[string]int32" {
		t.Errorf("MapType(record<DOMString,record<USVString,long>>).Name = %q, want \"map[string]map[string]int32\"", got.Name)
	}
}

// TestMapTypeRecordUnresolvedValue verifies that an unresolved value type propagates
// Unresolved=true to the record: record<DOMString, EventTarget> → map[string]any, Unresolved=true.
func TestMapTypeRecordUnresolvedValue(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "EventTarget"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(record<DOMString,EventTarget>) returned error: %v", err)
	}
	if got.Name != "map[string]any" {
		t.Errorf("MapType(record<DOMString,EventTarget>).Name = %q, want \"map[string]any\"", got.Name)
	}
	if !got.Unresolved {
		t.Error("MapType(record<DOMString,EventTarget>).Unresolved = false; unresolved value type must propagate Unresolved to the record")
	}
}

// --- Happy path ---

// TestMapTypeRecordExact verifies the spec example: record<DOMString, long> → map[string]int32.
func TestMapTypeRecordExact(t *testing.T) {
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
	if got.Name != "map[string]int32" {
		t.Errorf("MapType(record<DOMString,long>).Name = %q, want \"map[string]int32\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(record<DOMString,long>).Unresolved = true; fully resolved record must not be marked Unresolved")
	}
}

// TestMapTypeRecordUSVStringKey verifies that USVString is a valid record key.
func TestMapTypeRecordUSVStringKey(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "USVString"}, {Base: "boolean"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(record<USVString,boolean>) returned error: %v", err)
	}
	if got.Name != "map[string]bool" {
		t.Errorf("MapType(record<USVString,boolean>).Name = %q, want \"map[string]bool\"", got.Name)
	}
}

// TestMapTypeRecordByteStringKey verifies that ByteString is a valid record key.
func TestMapTypeRecordByteStringKey(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "record",
		Subtypes: []*webidl.IDLType{{Base: "ByteString"}, {Base: "DOMString"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(record<ByteString,DOMString>) returned error: %v", err)
	}
	if got.Name != "map[string]string" {
		t.Errorf("MapType(record<ByteString,DOMString>).Name = %q, want \"map[string]string\"", got.Name)
	}
}

// ---------------------------------------------------------------------------
// CATH-47: Promise<T> → any (design decision: punt, unblock codegen)
//
// Promise<T> maps to GoType{Name:"any", Unresolved:false}. This is an intentional
// mapping analogous to void/undefined → any. Promise semantics cannot be faithfully
// represented at the Go type level without a runtime package; callers that need
// typed resolution narrow with type assertions. This decision is revisable in a
// future ticket once the codegen layer has a clearer picture of Promise call sites.
// ---------------------------------------------------------------------------

// --- Edge / boundary ---

// TestMapTypePromiseNullableNoPointer verifies that Promise<T>? does NOT gain a pointer.
// The resolved type is any, which is already a reference type and nil-able.
func TestMapTypePromiseNullableNoPointer(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "Promise",
		Nullable: true,
		Subtypes: []*webidl.IDLType{{Base: "long"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(Promise<long>?) returned error: %v", err)
	}
	if got.Name != "any" {
		t.Errorf("MapType(Promise<long>?).Name = %q, want \"any\"", got.Name)
	}
	if got.Pointer {
		t.Error("MapType(Promise<long>?).Pointer = true; any is already reference-typed, must not gain extra pointer")
	}
}

// --- Error / rejection ---

// TestMapTypePromiseNoSubtypesReturnsError verifies that a malformed Promise with no
// type parameter returns an error (no valid WebIDL Promise exists without one).
func TestMapTypePromiseNoSubtypesReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "Promise",
		Subtypes: nil,
	}
	_, err := m.MapType(idlType)
	if err == nil {
		t.Error("MapType(Promise with nil Subtypes) expected error for malformed node, got nil")
	}
}

// --- Happy path ---

// TestMapTypePromisePuntsToAny verifies the design decision: Promise<T> maps to
// GoType{Name:"any", Unresolved:false}. Unresolved=false distinguishes this
// intentional mapping from interface-name fallbacks that codegen must flag.
func TestMapTypePromisePuntsToAny(t *testing.T) {
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
	if got.Name != "any" {
		t.Errorf("MapType(Promise<long>).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(Promise<long>).Unresolved = true; Promise→any is an intentional mapping, not an unresolved stub")
	}
}

// TestMapTypePromiseVoidSubtype verifies that Promise<void> also maps to any, not any?.
func TestMapTypePromiseVoidSubtype(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	idlType := &webidl.IDLType{
		Generic:  "Promise",
		Subtypes: []*webidl.IDLType{{Base: "void"}},
	}
	got, err := m.MapType(idlType)
	if err != nil {
		t.Fatalf("MapType(Promise<void>) returned error: %v", err)
	}
	if got.Name != "any" {
		t.Errorf("MapType(Promise<void>).Name = %q, want \"any\"", got.Name)
	}
	if got.Unresolved {
		t.Error("MapType(Promise<void>).Unresolved = true; Promise<void>→any must be intentional (Unresolved=false)")
	}
}
