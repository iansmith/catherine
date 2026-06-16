package typemap

import (
	"fmt"
	"path"

	"github.com/iansmith/webidl/webidl"
)

// GoType is a resolved Go type expression produced by the TypeMapper.
// PkgPath is the import path of the package that declares the type ("" for
// predeclared / built-in types). Name is the unqualified type name. Pointer
// indicates the type should be written as *Name in generated source.
type GoType struct {
	PkgPath string
	Name    string
	Pointer bool
}

// String returns the Go source representation of the type — the form that
// would appear directly in generated code. For predeclared types (PkgPath=="")
// this is just the name, optionally pointer-prefixed. For package-scoped types
// this uses the last path segment of PkgPath as the package qualifier, e.g.
// PkgPath="net/http", Name="Request" → "http.Request".
//
// A zero-value GoType (Name=="") returns "".
func (g GoType) String() string {
	if g.Name == "" {
		return ""
	}
	prefix := ""
	if g.Pointer {
		prefix = "*"
	}
	if g.PkgPath == "" {
		return prefix + g.Name
	}
	return prefix + path.Base(g.PkgPath) + "." + g.Name
}

// Mapper translates *webidl.IDLType values from the resolved IR into GoType
// expressions. All codegen sub-packages call into Mapper rather than producing
// Go type strings directly.
//
// The zero value (Mapper{}) is ready to use. Configuration fields — such as
// per-target overrides or alternative string-type representations — will be
// added by later tickets.
type Mapper struct{}

// MapType maps a single IDLType to a GoType. Returns an error if t is nil or
// if t carries no recognisable type information (Union=false, Generic="",
// Base=""). Stubs for the individual type families will be replaced in
// follow-on tickets (CATH-44 through CATH-48).
func (m *Mapper) MapType(t *webidl.IDLType) (GoType, error) {
	if t == nil {
		return GoType{}, fmt.Errorf("MapType: nil IDLType")
	}

	var got GoType
	switch {
	case t.Union:
		got = stubUnion(t)
	case t.Generic != "":
		got = stubGeneric(t)
	case t.Base != "":
		got = stubBase(t)
	default:
		return GoType{}, fmt.Errorf("MapType: IDLType has neither Union, Generic, nor Base set")
	}

	// Nullable post-processing: T? → *T for value types. Reference types are
	// already nil-able, so they are left unchanged.
	if t.Nullable && isValueType(got) {
		got.Pointer = true
	}

	return got, nil
}

// ---------------------------------------------------------------------------
// Stubs — replaced by real implementations in CATH-44 through CATH-48
// ---------------------------------------------------------------------------

func stubUnion(_ *webidl.IDLType) GoType   { return GoType{Name: "any"} }
func stubGeneric(_ *webidl.IDLType) GoType { return GoType{Name: "any"} }
func stubBase(_ *webidl.IDLType) GoType    { return GoType{Name: "any"} }

// valueTypeNames lists the predeclared Go types that are value types: a
// nullable IDL type maps to a pointer (*T) for these, since reference types
// (interfaces, slices, maps) are already nil-able and must not gain an extra
// pointer layer.
var valueTypeNames = map[string]bool{
	"bool":    true,
	"byte":    true,
	"rune":    true,
	"int":     true,
	"int8":    true,
	"int16":   true,
	"int32":   true,
	"int64":   true,
	"uint":    true,
	"uint8":   true,
	"uint16":  true,
	"uint32":  true,
	"uint64":  true,
	"uintptr": true,
	"float32": true,
	"float64": true,
	"string":  true,
}

func isValueType(g GoType) bool {
	return g.PkgPath == "" && valueTypeNames[g.Name]
}
