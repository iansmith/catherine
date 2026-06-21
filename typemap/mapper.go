package typemap

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/iansmith/webidl/webidl"
)

// GoType is a resolved Go type expression produced by the TypeMapper.
// PkgPath is the import path of the package that declares the type ("" for
// predeclared / built-in types). Name is the unqualified type name. Pointer
// indicates the type should be written as *Name in generated source.
//
// Unresolved is true when the mapper could not find an explicit Go type for
// the IDL base — either an unimplemented generic or an unrecognised interface
// name. Codegen layers should check Unresolved before
// emitting output to avoid silently producing any for unmapped interface names.
// Intentional mappings (scalars, string types, IDL any/object/undefined/void)
// always have Unresolved=false.
type GoType struct {
	PkgPath    string
	Name       string
	Pointer    bool
	Unresolved bool
	// Annotation carries IDL extended-attribute modifiers that affect codegen
	// semantics but not the Go type name (e.g. "Clamp", "EnforceRange",
	// "AllowShared"). Empty when no modifier applies.
	Annotation string
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
// recognisable type information (Union=false, Generic="", Base=""). Union types
// map to intentional any (Unresolved:false); all generic families are handled
// (sequences, record, Promise).
//
// Note: a nil error does not guarantee a fully-resolved type. Unrecognised base
// types and unrecognised generics return GoType{Name:"any", Unresolved:true}
// with no error. Intentional mappings (including union→any) return
// Unresolved:false. Check GoType.Unresolved before emitting code to avoid
// silently producing any for unmapped names.
func (m Mapper) MapType(t *webidl.IDLType) (GoType, error) {
	if t == nil {
		return GoType{}, fmt.Errorf("MapType: nil IDLType")
	}

	var got GoType
	switch {
	case t.Union && t.Generic != "":
		return GoType{}, fmt.Errorf("MapType: IDLType has both Union and Generic set (malformed node)")
	case t.Union:
		got = unionToAny(t)
	case t.Generic != "":
		var genErr error
		got, genErr = m.mapGeneric(t)
		if genErr != nil {
			return GoType{}, genErr
		}
	case t.Base != "":
		got = mapBase(t)
		got.Annotation = extAttrAnnotation(t.ExtAttrs)
	default:
		return GoType{}, fmt.Errorf("MapType: IDLType has neither Union, Generic, nor Base set")
	}

	// Nullable post-processing: T? → *T for value types. Reference types are
	// already nil-able, so they are left unchanged.
	if t.Nullable && !got.Unresolved && isValueType(got) {
		got.Pointer = true
	}

	return got, nil
}

// ---------------------------------------------------------------------------
// Generic and union resolution
// ---------------------------------------------------------------------------

// unionToAny maps all IDL union types to Go's any. This is an intentional
// design decision: union→any loses static type information but is the
// simplest mapping that unblocks codegen. Callers that need per-member
// typing narrow with type assertions. Unresolved is false because this is
// a deliberate choice, not an unimplemented stub.
func unionToAny(_ *webidl.IDLType) GoType { return GoType{Name: "any"} }

// mapGeneric resolves IDLType nodes with a non-empty Generic field. The three
// IDL sequence-like generics (sequence, FrozenArray, ObservableArray) map to Go
// slices; async_sequence is IDL-to-JS only and returns an error. record maps to
// map[string]V; Promise maps to any (see case "Promise" for the rationale).
// Other generics remain as Unresolved stubs until a follow-on ticket implements them.
//
// FrozenArray and ObservableArray are both mapped to plain []T. FrozenArray is
// immutable in WebIDL — callers must not mutate the returned slice. ObservableArray
// mutation side effects (platform observer hooks) are out of scope.
func (m Mapper) mapGeneric(t *webidl.IDLType) (GoType, error) {
	switch t.Generic {
	case "sequence", "FrozenArray", "ObservableArray":
		if len(t.Subtypes) == 0 {
			return GoType{}, fmt.Errorf("MapType: %s has no type parameter", t.Generic)
		}
		elem, err := m.MapType(t.Subtypes[0])
		if err != nil {
			return GoType{}, fmt.Errorf("%s element: %w", t.Generic, err)
		}
		return GoType{Name: "[]" + elem.String(), Unresolved: elem.Unresolved}, nil
	case "async_sequence":
		return GoType{}, fmt.Errorf("MapType: async_sequence is IDL-to-JS only and should have been rejected by validate.go")
	case "record":
		if len(t.Subtypes) != 2 {
			return GoType{}, fmt.Errorf("MapType: record requires exactly 2 type parameters, got %d", len(t.Subtypes))
		}
		// WebIDL §3.2.26 restricts record key types to DOMString, USVString, or
		// ByteString. Use webidl.StringTypes (the parser's authoritative list) rather
		// than nonScalarGoTypes, which also contains CSSOMString and would silently
		// accept an invalid key.
		if t.Subtypes[0] == nil {
			return GoType{}, fmt.Errorf("MapType: record key type is nil")
		}
		if !isRecordKeyType(t.Subtypes[0].Base) {
			return GoType{}, fmt.Errorf("MapType: record key type must be DOMString, USVString, or ByteString, got %q", t.Subtypes[0].Base)
		}
		val, err := m.MapType(t.Subtypes[1])
		if err != nil {
			return GoType{}, fmt.Errorf("record value: %w", err)
		}
		return GoType{Name: "map[string]" + val.String(), Unresolved: val.Unresolved}, nil
	case "Promise":
		// Promise<T> is mapped to any (intentional punt, not an unresolved stub).
		// Go cannot faithfully represent single-resolution Promise semantics at the
		// type level without a dedicated runtime type. Mapping to any unblocks codegen;
		// callers that need typed resolution narrow with type assertions. This is
		// revisable once the codegen layer has a clearer picture of Promise call sites.
		if len(t.Subtypes) != 1 {
			return GoType{}, fmt.Errorf("MapType: Promise requires exactly 1 type parameter, got %d", len(t.Subtypes))
		}
		// Validate the type parameter even though its resolved GoType is discarded.
		// This surfaces errors in T (e.g. async_sequence, which is IDL-to-JS only)
		// rather than silently accepting Promise<invalid>.
		if _, err := m.MapType(t.Subtypes[0]); err != nil {
			return GoType{}, fmt.Errorf("Promise type parameter: %w", err)
		}
		return GoType{Name: "any"}, nil
	default:
		// Other generics remain as stubs until a follow-on ticket implements them.
		return GoType{Name: "any", Unresolved: true}, nil
	}
}

// isRecordKeyType reports whether base is a valid WebIDL record key type.
// Only DOMString, USVString, and ByteString are permitted (WebIDL §3.2.26).
// Uses webidl.StringTypes as the single source of truth, matching the parser.
func isRecordKeyType(base string) bool {
	return slices.Contains(webidl.StringTypes, base)
}

// extAttrAnnotation scans a list of IDL extended attributes and returns the
// name of the first recognised type-modifier attribute ("Clamp",
// "EnforceRange", "AllowShared"). Returns "" when none is present or when
// all attributes are unrecognised — unknown attributes are silently ignored.
func extAttrAnnotation(attrs []*webidl.ExtAttr) string {
	for _, a := range attrs {
		switch a.Name {
		case webidl.ExtAttrClamp, webidl.ExtAttrEnforceRange, webidl.ExtAttrAllowShared:
			return a.Name
		}
	}
	return ""
}

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

// nonScalarGoTypes maps WebIDL string and special/sentinel type bases to Go
// predeclared type names.
//
// String types all collapse to bare string; semantic distinctions (UTF-16 vs.
// UTF-8, byte-range validity) are caller responsibility. ByteString maps to
// string rather than []byte — the IDL model treats it as a string, and
// callers who need byte access convert with []byte(s).
//
// undefined and void ("no value") map to any because the mapper cannot know
// return vs. argument position. The codegen layer (CATH-46+) decides whether
// to emit a return type.
var nonScalarGoTypes = map[string]string{
	// string types
	"ByteString":  "string",
	"CSSOMString": "string", // CSS Object Model string; recognized by webidl/validate.go
	"DOMString":   "string",
	"USVString":   "string",
	// special / sentinel types
	"any":       "any",
	"object":    "any",
	"undefined": "any",
	"void":      "any",
}

// mapBase resolves an IDLType with a non-empty Base field to a GoType. Scalar
// primitive types are mapped via scalarGoTypes; string and special/sentinel
// types via nonScalarGoTypes. All other base types (interface names, etc.)
// fall back to GoType{Name: "any", Unresolved: true}.
func mapBase(t *webidl.IDLType) GoType {
	if name, ok := scalarGoTypes[t.Base]; ok {
		return GoType{Name: name}
	}
	if name, ok := nonScalarGoTypes[t.Base]; ok {
		return GoType{Name: name}
	}
	return GoType{Name: "any", Unresolved: true}
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
