package typemap

import (
	"testing"

	"github.com/iansmith/webidl/webidl"
)

// ===========================================================================
// CATH-63: binding-backend type-info accessors (red tests)
//
// These describe the expected behavior of UnionMembers /
// CallbackFunctionSignature / CallbackInterfaceSignature. They fail on the
// Phase-0 stub implementation and turn green once the accessors are wired.
// ===========================================================================

// ---------------------------------------------------------------------------
// UnionMembers — error / rejection
// ---------------------------------------------------------------------------

func TestUnionMembersNilReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	if _, err := m.UnionMembers(nil); err == nil {
		t.Error("UnionMembers(nil) expected error, got nil")
	}
}

func TestUnionMembersNonUnionReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	// A plain base type is not a union; the accessor must reject it rather than
	// silently returning a one-element slice.
	if _, err := m.UnionMembers(&webidl.IDLType{Base: "long"}); err == nil {
		t.Error("UnionMembers(non-union) expected error, got nil")
	}
}

func TestUnionMembersMalformedReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	// Union and Generic both set is a malformed node (same rule MapType enforces).
	malformed := &webidl.IDLType{Union: true, Generic: "sequence"}
	if _, err := m.UnionMembers(malformed); err == nil {
		t.Error("UnionMembers(malformed Union+Generic) expected error, got nil")
	}
}

func TestUnionMembersNilMemberReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	u := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, nil},
	}
	if _, err := m.UnionMembers(u); err == nil {
		t.Error("UnionMembers(union with nil member) expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// UnionMembers — edge / boundary
// ---------------------------------------------------------------------------

// Nested unions are flattened: a binding wants the leaf member types, not an
// inner `any`. (double or (long or DOMString)) → [float64, int32, string].
func TestUnionMembersNestedUnionFlattened(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	inner := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "long"}, {Base: "DOMString"}},
	}
	outer := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "double"}, inner},
	}
	got, err := m.UnionMembers(outer)
	if err != nil {
		t.Fatalf("UnionMembers(nested) returned error: %v", err)
	}
	want := []string{"float64", "int32", "string"}
	if len(got) != len(want) {
		t.Fatalf("UnionMembers(nested) len = %d (%v), want %d (%v)", len(got), goTypeNames(got), len(want), want)
	}
	for i, w := range want {
		if got[i].String() != w {
			t.Errorf("UnionMembers(nested)[%d] = %q, want %q", i, got[i].String(), w)
		}
	}
}

// Distinct IDL string types collapse to the same Go type; members are kept
// (not deduped) so the binding can still map each IDL alternative to its
// coercion. (DOMString or USVString) → [string, string].
func TestUnionMembersDuplicateGoTypesPreserved(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	u := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "USVString"}},
	}
	got, err := m.UnionMembers(u)
	if err != nil {
		t.Fatalf("UnionMembers returned error: %v", err)
	}
	if len(got) != 2 || got[0].String() != "string" || got[1].String() != "string" {
		t.Errorf("UnionMembers(DOMString|USVString) = %v, want [string string]", goTypeNames(got))
	}
}

// An unresolved interface member (e.g. Node) keeps Unresolved=true so the
// binding can tell apart a real `any` from an unmapped interface name.
func TestUnionMembersUnresolvedMemberPropagated(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	u := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "Node"}, {Base: "DOMString"}},
	}
	got, err := m.UnionMembers(u)
	if err != nil {
		t.Fatalf("UnionMembers returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("UnionMembers len = %d, want 2", len(got))
	}
	if !got[0].Unresolved {
		t.Errorf("UnionMembers[0] (Node) Unresolved = false, want true")
	}
	// An unmapped interface name resolves to `any` (not a half-mapping that keeps
	// the IDL name as the Go type name).
	if got[0].String() != "any" {
		t.Errorf("UnionMembers[0] (Node) = %q, want \"any\"", got[0].String())
	}
	if got[1].Unresolved {
		t.Errorf("UnionMembers[1] (DOMString) Unresolved = true, want false")
	}
}

// Nullability of the union itself does not change member enumeration.
func TestUnionMembersNullableUnion(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	u := &webidl.IDLType{
		Nullable: true,
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "Node"}, {Base: "DOMString"}},
	}
	got, err := m.UnionMembers(u)
	if err != nil {
		t.Fatalf("UnionMembers(nullable union) returned error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("UnionMembers(nullable union) len = %d, want 2", len(got))
	}
}

// ---------------------------------------------------------------------------
// UnionMembers — happy path
// ---------------------------------------------------------------------------

func TestUnionMembersResolvedScalars(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	u := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "double"}, {Base: "DOMString"}},
	}
	got, err := m.UnionMembers(u)
	if err != nil {
		t.Fatalf("UnionMembers returned error: %v", err)
	}
	want := []GoType{{Name: "float64"}, {Name: "string"}}
	if len(got) != len(want) {
		t.Fatalf("UnionMembers len = %d (%v), want %d", len(got), goTypeNames(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("UnionMembers[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// CallbackFunctionSignature — error / rejection
// ---------------------------------------------------------------------------

func TestCallbackFunctionSignatureNilReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	if _, err := m.CallbackFunctionSignature(nil); err == nil {
		t.Error("CallbackFunctionSignature(nil) expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// CallbackFunctionSignature — edge / boundary
// ---------------------------------------------------------------------------

// undefined/void return → zero GoType (no return), mirroring buildReturnType.
func TestCallbackFunctionSignatureVoidReturn(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cb := &webidl.CallbackFunction{
		Name:       "VoidCb",
		ReturnType: &webidl.IDLType{Base: "undefined"},
		Arguments:  []*webidl.Argument{{Name: "e", IDLType: &webidl.IDLType{Base: "DOMString"}}},
	}
	sig, err := m.CallbackFunctionSignature(cb)
	if err != nil {
		t.Fatalf("CallbackFunctionSignature returned error: %v", err)
	}
	if sig.Return.String() != "" {
		t.Errorf("void callback Return = %q, want \"\" (zero GoType)", sig.Return.String())
	}
	// Params must still be surfaced even when the return is void.
	if len(sig.Params) != 1 || sig.Params[0].GoType.String() != "string" {
		t.Errorf("void callback Params = %#v, want one string param", sig.Params)
	}
}

func TestCallbackFunctionSignatureVariadicParam(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cb := &webidl.CallbackFunction{
		Name:       "VarCb",
		ReturnType: &webidl.IDLType{Base: "undefined"},
		Arguments:  []*webidl.Argument{{Name: "nums", IDLType: &webidl.IDLType{Base: "double"}, Variadic: true}},
	}
	sig, err := m.CallbackFunctionSignature(cb)
	if err != nil {
		t.Fatalf("CallbackFunctionSignature returned error: %v", err)
	}
	if len(sig.Params) != 1 {
		t.Fatalf("Params len = %d, want 1", len(sig.Params))
	}
	if !sig.Params[0].Variadic {
		t.Error("variadic param Variadic = false, want true")
	}
	if sig.Params[0].GoType.String() != "float64" {
		t.Errorf("variadic param GoType = %q, want float64", sig.Params[0].GoType.String())
	}
}

func TestCallbackFunctionSignatureOptionalParam(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cb := &webidl.CallbackFunction{
		Name:       "OptCb",
		ReturnType: &webidl.IDLType{Base: "undefined"},
		Arguments:  []*webidl.Argument{{Name: "x", IDLType: &webidl.IDLType{Base: "long"}, Optional: true}},
	}
	sig, err := m.CallbackFunctionSignature(cb)
	if err != nil {
		t.Fatalf("CallbackFunctionSignature returned error: %v", err)
	}
	if len(sig.Params) != 1 || !sig.Params[0].Optional {
		t.Errorf("optional param not surfaced: %#v", sig.Params)
	}
}

func TestCallbackFunctionSignatureNoArgs(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cb := &webidl.CallbackFunction{
		Name:       "Thunk",
		ReturnType: &webidl.IDLType{Base: "boolean"},
		Arguments:  nil,
	}
	sig, err := m.CallbackFunctionSignature(cb)
	if err != nil {
		t.Fatalf("CallbackFunctionSignature returned error: %v", err)
	}
	if len(sig.Params) != 0 {
		t.Errorf("Params len = %d, want 0", len(sig.Params))
	}
	if sig.Return.String() != "bool" {
		t.Errorf("Return = %q, want bool", sig.Return.String())
	}
}

// ---------------------------------------------------------------------------
// CallbackFunctionSignature — happy path
// ---------------------------------------------------------------------------

func TestCallbackFunctionSignatureResolved(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cb := &webidl.CallbackFunction{
		Name:       "Predicate",
		ReturnType: &webidl.IDLType{Base: "boolean"},
		Arguments: []*webidl.Argument{
			{Name: "x", IDLType: &webidl.IDLType{Base: "double"}},
			{Name: "s", IDLType: &webidl.IDLType{Base: "DOMString"}},
		},
	}
	sig, err := m.CallbackFunctionSignature(cb)
	if err != nil {
		t.Fatalf("CallbackFunctionSignature returned error: %v", err)
	}
	if sig.Return.String() != "bool" {
		t.Errorf("Return = %q, want bool", sig.Return.String())
	}
	wantParams := []struct {
		name   string
		goType string
	}{{"x", "float64"}, {"s", "string"}}
	if len(sig.Params) != len(wantParams) {
		t.Fatalf("Params len = %d, want %d", len(sig.Params), len(wantParams))
	}
	for i, w := range wantParams {
		if sig.Params[i].Name != w.name || sig.Params[i].GoType.String() != w.goType {
			t.Errorf("Params[%d] = {%q, %q}, want {%q, %q}",
				i, sig.Params[i].Name, sig.Params[i].GoType.String(), w.name, w.goType)
		}
	}
}

// ---------------------------------------------------------------------------
// CallbackInterfaceSignature — error / rejection
// ---------------------------------------------------------------------------

func TestCallbackInterfaceSignatureNilReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	if _, err := m.CallbackInterfaceSignature(nil); err == nil {
		t.Error("CallbackInterfaceSignature(nil) expected error, got nil")
	}
}

func TestCallbackInterfaceSignatureNonCallbackVariantReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	iface := &webidl.Interface{
		Variant: webidl.IfaceRegular,
		Name:    "NotACallback",
		Members: []webidl.Member{
			&webidl.Operation{Name: "foo", ReturnType: &webidl.IDLType{Base: "undefined"}},
		},
	}
	if _, err := m.CallbackInterfaceSignature(iface); err == nil {
		t.Error("CallbackInterfaceSignature(regular interface) expected error, got nil")
	}
}

func TestCallbackInterfaceSignatureNoOperationReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	// A callback interface with only a constant has no operation to surface.
	iface := &webidl.Interface{
		Variant: webidl.IfaceCallback,
		Name:    "ConstOnly",
		Members: []webidl.Member{
			&webidl.Constant{Name: "VALUE", IDLType: &webidl.IDLType{Base: "long"}},
		},
	}
	if _, err := m.CallbackInterfaceSignature(iface); err == nil {
		t.Error("CallbackInterfaceSignature(no operation) expected error, got nil")
	}
}

func TestCallbackInterfaceSignatureMultipleOperationsReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	iface := &webidl.Interface{
		Variant: webidl.IfaceCallback,
		Name:    "TwoOps",
		Members: []webidl.Member{
			&webidl.Operation{Name: "a", ReturnType: &webidl.IDLType{Base: "undefined"}},
			&webidl.Operation{Name: "b", ReturnType: &webidl.IDLType{Base: "undefined"}},
		},
	}
	if _, err := m.CallbackInterfaceSignature(iface); err == nil {
		t.Error("CallbackInterfaceSignature(multiple operations) expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// CallbackInterfaceSignature — cross-feature / happy path
// ---------------------------------------------------------------------------

// Constants coexist with the single operation in a callback interface; they
// must be ignored when resolving the signature.
func TestCallbackInterfaceSignatureIgnoresConstants(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	iface := &webidl.Interface{
		Variant: webidl.IfaceCallback,
		Name:    "WithConst",
		Members: []webidl.Member{
			&webidl.Constant{Name: "K", IDLType: &webidl.IDLType{Base: "long"}},
			&webidl.Operation{
				Name:       "call",
				ReturnType: &webidl.IDLType{Base: "boolean"},
				Arguments:  []*webidl.Argument{{Name: "x", IDLType: &webidl.IDLType{Base: "double"}}},
			},
		},
	}
	sig, err := m.CallbackInterfaceSignature(iface)
	if err != nil {
		t.Fatalf("CallbackInterfaceSignature returned error: %v", err)
	}
	if sig.Return.String() != "bool" {
		t.Errorf("Return = %q, want bool", sig.Return.String())
	}
	if len(sig.Params) != 1 || sig.Params[0].GoType.String() != "float64" {
		t.Errorf("Params = %#v, want one float64 param", sig.Params)
	}
}

// EventListener-shaped callback interface: undefined handleEvent(Event event).
func TestCallbackInterfaceSignatureEventListener(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	iface := &webidl.Interface{
		Variant: webidl.IfaceCallback,
		Name:    "EventListener",
		Members: []webidl.Member{
			&webidl.Operation{
				Name:       "handleEvent",
				ReturnType: &webidl.IDLType{Base: "undefined"},
				Arguments:  []*webidl.Argument{{Name: "event", IDLType: &webidl.IDLType{Base: "Event"}}},
			},
		},
	}
	sig, err := m.CallbackInterfaceSignature(iface)
	if err != nil {
		t.Fatalf("CallbackInterfaceSignature returned error: %v", err)
	}
	if sig.Return.String() != "" {
		t.Errorf("handleEvent Return = %q, want \"\" (void)", sig.Return.String())
	}
	if len(sig.Params) != 1 || sig.Params[0].Name != "event" {
		t.Fatalf("Params = %#v, want one 'event' param", sig.Params)
	}
	// Event is an unmapped interface name → Unresolved any.
	if !sig.Params[0].GoType.Unresolved {
		t.Error("event param Unresolved = false, want true (Event is an unmapped interface)")
	}
	if sig.Params[0].GoType.String() != "any" {
		t.Errorf("event param GoType = %q, want \"any\"", sig.Params[0].GoType.String())
	}
}

// ===========================================================================
// CATH-63: adversary gap tests
// ===========================================================================

// --- UnionMembers: boundary -------------------------------------------------

// A one-member union must yield exactly that one member, not be mistaken for
// "not really a union" by a len>1 guard.
func TestUnionMembersSingleMember(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	u := &webidl.IDLType{Union: true, Subtypes: []*webidl.IDLType{{Base: "double"}}}
	got, err := m.UnionMembers(u)
	if err != nil {
		t.Fatalf("UnionMembers(single) returned error: %v", err)
	}
	if len(got) != 1 || got[0].String() != "float64" {
		t.Errorf("UnionMembers(single) = %v, want [float64]", goTypeNames(got))
	}
}

// A union with no members is malformed; the accessor must reject it rather than
// return an empty slice a binding would silently treat as "no overloads".
func TestUnionMembersEmptySubtypesReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	if _, err := m.UnionMembers(&webidl.IDLType{Union: true}); err == nil {
		t.Error("UnionMembers(union with no subtypes) expected error, got nil")
	}
}

// Flattening must be recursive to any depth, not a single pass.
// (double or (long or (DOMString or boolean))) → [float64, int32, string, bool].
func TestUnionMembersDoublyNestedFlattened(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	innermost := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, {Base: "boolean"}},
	}
	inner := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "long"}, innermost},
	}
	outer := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "double"}, inner},
	}
	got, err := m.UnionMembers(outer)
	if err != nil {
		t.Fatalf("UnionMembers(doubly nested) returned error: %v", err)
	}
	want := []string{"float64", "int32", "string", "bool"}
	if len(got) != len(want) {
		t.Fatalf("UnionMembers(doubly nested) = %v, want %v", goTypeNames(got), want)
	}
	for i, w := range want {
		if got[i].String() != w {
			t.Errorf("UnionMembers(doubly nested)[%d] = %q, want %q", i, got[i].String(), w)
		}
	}
}

// --- UnionMembers: nested error propagation ---------------------------------

// A nil leaf buried inside a nested union must still be rejected — validation
// is not a top-level-only check.
func TestUnionMembersNestedNilMemberReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	inner := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "DOMString"}, nil},
	}
	outer := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "double"}, inner},
	}
	if _, err := m.UnionMembers(outer); err == nil {
		t.Error("UnionMembers(nested nil member) expected error, got nil")
	}
}

// A member whose own type fails to map (async_sequence is IDL-to-JS only and
// MapType errors on it) must propagate, not be silently dropped.
func TestUnionMembersErroringMemberPropagated(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	u := &webidl.IDLType{
		Union:    true,
		Subtypes: []*webidl.IDLType{{Base: "double"}, {Generic: "async_sequence", Subtypes: []*webidl.IDLType{{Base: "long"}}}},
	}
	if _, err := m.UnionMembers(u); err == nil {
		t.Error("UnionMembers(erroring member) expected error, got nil")
	}
}

// --- CallbackFunctionSignature: nil-safety / error paths --------------------

// A nil ReturnType means void (same as Base=="undefined"), and must not panic.
func TestCallbackFunctionSignatureNilReturnType(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cb := &webidl.CallbackFunction{
		Name:       "NilRet",
		ReturnType: nil,
		Arguments:  []*webidl.Argument{{Name: "x", IDLType: &webidl.IDLType{Base: "long"}}},
	}
	sig, err := m.CallbackFunctionSignature(cb)
	if err != nil {
		t.Fatalf("CallbackFunctionSignature(nil return) returned error: %v", err)
	}
	if sig.Return.String() != "" {
		t.Errorf("nil-return callback Return = %q, want \"\"", sig.Return.String())
	}
	if len(sig.Params) != 1 || sig.Params[0].GoType.String() != "int32" {
		t.Errorf("nil-return callback Params = %#v, want one int32 param", sig.Params)
	}
}

// A parameter whose type fails to map must surface as an error, not a silent any.
func TestCallbackFunctionSignatureParamMapErrorPropagated(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cb := &webidl.CallbackFunction{
		Name:       "BadParam",
		ReturnType: &webidl.IDLType{Base: "undefined"},
		Arguments:  []*webidl.Argument{{Name: "bad", IDLType: &webidl.IDLType{Generic: "async_sequence", Subtypes: []*webidl.IDLType{{Base: "long"}}}}},
	}
	if _, err := m.CallbackFunctionSignature(cb); err == nil {
		t.Error("CallbackFunctionSignature(param map error) expected error, got nil")
	}
}

// A nil Argument element must be rejected, not dereferenced.
func TestCallbackFunctionSignatureNilArgumentReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	cb := &webidl.CallbackFunction{
		Name:       "NilArg",
		ReturnType: &webidl.IDLType{Base: "undefined"},
		Arguments:  []*webidl.Argument{{Name: "e", IDLType: &webidl.IDLType{Base: "DOMString"}}, nil},
	}
	if _, err := m.CallbackFunctionSignature(cb); err == nil {
		t.Error("CallbackFunctionSignature(nil argument) expected error, got nil")
	}
}

// --- CallbackInterfaceSignature: variant + Special handling -----------------

// A mixin is not a callback interface — reject it. Guards an impl that checks
// `Variant == IfaceRegular` instead of `Variant != IfaceCallback`.
func TestCallbackInterfaceSignatureMixinVariantReturnsError(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	iface := &webidl.Interface{
		Variant: webidl.IfaceMixin,
		Name:    "MixinNotCallback",
		Members: []webidl.Member{
			&webidl.Operation{Name: "op", ReturnType: &webidl.IDLType{Base: "undefined"}},
		},
	}
	if _, err := m.CallbackInterfaceSignature(iface); err == nil {
		t.Error("CallbackInterfaceSignature(mixin variant) expected error, got nil")
	}
}

// "The single operation" means the single *regular* (Special=="") operation;
// special operations (getter/setter/static/stringifier) are not it and must be
// skipped, so a callback interface with one special op + one regular op resolves
// the regular one.
func TestCallbackInterfaceSignatureSkipsSpecialOps(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	iface := &webidl.Interface{
		Variant: webidl.IfaceCallback,
		Name:    "WithSpecial",
		Members: []webidl.Member{
			&webidl.Operation{Name: "", Special: "getter", ReturnType: &webidl.IDLType{Base: "undefined"}},
			&webidl.Operation{
				Name:       "call",
				Special:    "",
				ReturnType: &webidl.IDLType{Base: "boolean"},
				Arguments:  []*webidl.Argument{{Name: "x", IDLType: &webidl.IDLType{Base: "double"}}},
			},
		},
	}
	sig, err := m.CallbackInterfaceSignature(iface)
	if err != nil {
		t.Fatalf("CallbackInterfaceSignature(special+regular) returned error: %v", err)
	}
	if sig.Return.String() != "bool" {
		t.Errorf("Return = %q, want bool", sig.Return.String())
	}
	if len(sig.Params) != 1 || sig.Params[0].GoType.String() != "float64" {
		t.Errorf("Params = %#v, want one float64 param", sig.Params)
	}
}

// An interface whose only operation is special has zero regular operations and
// must be rejected.
func TestCallbackInterfaceSignatureSingleSpecialOpRejected(t *testing.T) {
	t.Parallel()
	m := Mapper{}
	iface := &webidl.Interface{
		Variant: webidl.IfaceCallback,
		Name:    "OnlySpecial",
		Members: []webidl.Member{
			&webidl.Operation{Name: "s", Special: "static", ReturnType: &webidl.IDLType{Base: "undefined"}},
		},
	}
	if _, err := m.CallbackInterfaceSignature(iface); err == nil {
		t.Error("CallbackInterfaceSignature(only a special op) expected error, got nil")
	}
}

// goTypeNames is a small test helper for readable failure messages.
func goTypeNames(gts []GoType) []string {
	out := make([]string, len(gts))
	for i, g := range gts {
		out[i] = g.String()
	}
	return out
}
