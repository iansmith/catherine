package typemap

import (
	"fmt"
	"path"
	"strings"

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
	return prefix + pkgQualifier(g.PkgPath) + "." + g.Name
}

// Mapper translates *webidl.IDLType values from the resolved IR into GoType
// expressions. All codegen sub-packages call into Mapper rather than producing
// Go type strings directly.
//
// The zero value (Mapper{}) is ready to use. Configuration fields — such as
// per-target overrides or alternative string-type representations — will be
// added by later tickets.
type Mapper struct{}

// MapType maps a single IDLType to a GoType. Returns an error if t is nil,
// if t has both Union and Generic set (malformed node), or if t carries no
// recognisable type information (Union=false, Generic="", Base=""). Stubs for
// union and generic type families will be replaced in follow-on tickets
// (CATH-45 through CATH-48).
//
// Note: a nil error does not guarantee a fully-resolved type. Base types not
// yet handled by this mapper (string types, interface names, etc.) return
// GoType{Name:"any"} with no error until their respective tickets land.
func (m Mapper) MapType(t *webidl.IDLType) (GoType, error) {
	if t == nil {
		return GoType{}, fmt.Errorf("MapType: nil IDLType")
	}

	var got GoType
	switch {
	case t.Union && t.Generic != "":
		return GoType{}, fmt.Errorf("MapType: IDLType has both Union and Generic set (malformed node)")
	case t.Union:
		got = stubUnion(t)
	case t.Generic != "":
		got = stubGeneric(t)
	case t.Base != "":
		got = mapBase(t)
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
// Stubs — replaced by real implementations in CATH-45 through CATH-48
// ---------------------------------------------------------------------------

func stubUnion(_ *webidl.IDLType) GoType   { return GoType{Name: "any"} }
func stubGeneric(_ *webidl.IDLType) GoType { return GoType{Name: "any"} }

// scalarGoTypes maps IDL primitive scalar base names to their Go predeclared
// type names. "octet" is the WebIDL unsigned-byte primitive (the IDL keyword;
// not "unsigned byte" as sometimes written in prose).
var scalarGoTypes = map[string]string{
	"boolean":             "bool",
	"byte":                "int8",
	"octet":               "uint8",
	"short":               "int16",
	"unsigned short":      "uint16",
	"long":                "int32",
	"unsigned long":       "uint32",
	"long long":           "int64",
	"unsigned long long":  "uint64",
	"float":               "float32",
	"unrestricted float":  "float32",
	"double":              "float64",
	"unrestricted double": "float64",
}

// mapBase resolves an IDLType with a non-empty Base field to a GoType. Scalar
// primitive types are mapped via scalarGoTypes; all other base types (string
// types, interface names, etc.) fall back to GoType{Name: "any"} until
// CATH-45 implements them.
func mapBase(t *webidl.IDLType) GoType {
	if name, ok := scalarGoTypes[t.Base]; ok {
		return GoType{Name: name}
	}
	return GoType{Name: "any"}
}

// valueTypeNames lists the predeclared Go types that are value types: a
// nullable IDL type maps to a pointer (*T) for these, since reference types
// (interfaces, slices, maps) are already nil-able and must not gain an extra
// pointer layer.
var valueTypeNames = map[string]bool{
	"bool": true,
	// "byte" is intentionally absent: mapBase never emits Name="byte"
	// (IDL "byte"→int8, IDL "octet"→uint8). If you add a path that emits
	// Name="byte", add it here too or nullable promotion will be silently skipped.
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

// pkgQualifier derives the Go package qualifier (the short name written in
// source code) from a Go import path. It handles two versioned-module patterns
// that path.Base alone gets wrong:
//
//   - gopkg.in/yaml.v3  → last segment "yaml.v3"; strips after the first dot → "yaml"
//   - github.com/u/r/v2 → last segment "v2" (a bare major-version dir); uses the
//     preceding segment → "r"
//
// For unversioned paths (net/http, github.com/iansmith/webidl/webidl) the result
// is the same as path.Base.
func pkgQualifier(importPath string) string {
	seg := path.Base(importPath)
	if isMajorVersionSegment(seg) {
		seg = path.Base(path.Dir(importPath))
	}
	if i := strings.Index(seg, "."); i != -1 {
		seg = seg[:i]
	}
	return seg
}

// isMajorVersionSegment reports whether s is a bare Go major-version directory
// name ("v2", "v3", …). These are not valid package qualifiers on their own.
func isMajorVersionSegment(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
