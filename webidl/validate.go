package webidl

import "fmt"

// ValidationError is a semantic error produced by Validate.
type ValidationError struct {
	Rule    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("(%s) Validation error: %s", e.Rule, e.Message)
}

// Definitions is the grouped view of a parsed AST shared by all validation rules.
type Definitions struct {
	All        []Definition
	Unique     map[string]Definition    // semantic name → first non-partial def
	Partials   map[string][]Definition // semantic name → partial defs
	Duplicates []Definition            // non-partial defs with a repeated semantic name
	MixinMap   map[string][]*Interface // target name → included mixin interfaces
}

// definitionValidator is implemented by top-level AST nodes that validate themselves.
type definitionValidator interface {
	validate(defs *Definitions) []error
}

// memberValidator is implemented by interface/namespace members that have per-member rules.
type memberValidator interface {
	validateMember(defs *Definitions) []error
}

// validateMember implements the incomplete-op rule:
// regular and static operations must have both a return type and an identifier.
func (op *Operation) validateMember(_ *Definitions) []error {
	if op.Name == "" && (op.Special == "" || op.Special == "static") {
		return []error{&ValidationError{
			Rule:    "incomplete-op",
			Message: "Regular or static operations must have both a return type and an identifier.",
		}}
	}
	return nil
}

// validate implements constructor-member and no-cross-overload rules for interfaces.
func (iface *Interface) validate(defs *Definitions) []error {
	var errs []error

	// Walk members for per-member rules (e.g. incomplete-op on operations).
	for _, m := range iface.Members {
		if v, ok := m.(memberValidator); ok {
			errs = append(errs, v.validateMember(defs)...)
		}
	}

	// constructor-member: [Constructor] extended attribute is the legacy form.
	for _, ea := range iface.ExtAttrs {
		if ea.Name == "Constructor" {
			errs = append(errs, &ValidationError{
				Rule: "constructor-member",
				Message: "Constructors should now be represented as a `constructor()` operation " +
					"on the interface instead of `[Constructor]` extended attribute.",
			})
		}
	}

	// no-cross-overload: only applies to the canonical (non-partial) interface.
	if !iface.Partial {
		errs = append(errs, checkCrossOverload(defs, iface)...)
	}

	return errs
}

// checkCrossOverload detects operations re-defined across partials or included mixins.
// Operations may be overloaded within the same scope, but not across scopes.
func checkCrossOverload(defs *Definitions, iface *Interface) []error {
	statics := make(map[string]bool)
	nonstatics := make(map[string]bool)

	for _, m := range iface.Members {
		op, ok := m.(*Operation)
		if !ok || op.Name == "" {
			continue
		}
		if op.Special == "static" {
			statics[op.Name] = true
		} else {
			nonstatics[op.Name] = true
		}
	}

	name := semanticName(iface.Name)
	var errs []error

	var extensions [][]Member
	for _, p := range defs.Partials[name] {
		if pi, ok := p.(*Interface); ok {
			extensions = append(extensions, pi.Members)
		}
	}
	for _, mixin := range defs.MixinMap[name] {
		extensions = append(extensions, mixin.Members)
	}

	for _, members := range extensions {
		var extStatics, extNonstatics []*Operation
		for _, m := range members {
			op, ok := m.(*Operation)
			if !ok {
				continue
			}
			if op.Special == "static" {
				extStatics = append(extStatics, op)
			} else {
				extNonstatics = append(extNonstatics, op)
			}
		}

		for _, op := range extStatics {
			if op.Name != "" && statics[op.Name] {
				errs = append(errs, &ValidationError{
					Rule:    "no-cross-overload",
					Message: fmt.Sprintf("The static operation %q has already been defined for the base interface %q either in itself or in a mixin", op.Name, name),
				})
			}
		}
		for _, op := range extNonstatics {
			if op.Name != "" && nonstatics[op.Name] {
				errs = append(errs, &ValidationError{
					Rule:    "no-cross-overload",
					Message: fmt.Sprintf("The operation %q has already been defined for the base interface %q either in itself or in a mixin", op.Name, name),
				})
			}
		}

		// Accumulate names so subsequent extensions see earlier ones.
		for _, op := range extStatics {
			if op.Name != "" {
				statics[op.Name] = true
			}
		}
		for _, op := range extNonstatics {
			if op.Name != "" {
				nonstatics[op.Name] = true
			}
		}
	}

	return errs
}

// Validate runs semantic validation over a parsed AST and returns all errors found.
func Validate(ast []Definition) []error {
	defs := groupDefinitions(ast)
	var errs []error
	for _, def := range defs.All {
		if v, ok := def.(definitionValidator); ok {
			errs = append(errs, v.validate(&defs)...)
		}
	}
	errs = append(errs, checkDuplicateNames(&defs)...)
	return errs
}

// groupDefinitions builds the Definitions view used by all validation rules.
func groupDefinitions(all []Definition) Definitions {
	unique := make(map[string]Definition)
	var duplicates []Definition
	partials := make(map[string][]Definition)

	for _, def := range all {
		if defIsPartial(def) {
			name := defSemanticName(def)
			partials[name] = append(partials[name], def)
			continue
		}
		name := defSemanticName(def)
		if name == "" {
			continue // Includes — no single name
		}
		if _, exists := unique[name]; !exists {
			unique[name] = def
		} else {
			duplicates = append(duplicates, def)
		}
	}

	return Definitions{
		All:        all,
		Unique:     unique,
		Partials:   partials,
		Duplicates: duplicates,
		MixinMap:   buildMixinMap(all, unique),
	}
}

func buildMixinMap(all []Definition, unique map[string]Definition) map[string][]*Interface {
	mm := make(map[string][]*Interface)
	for _, def := range all {
		inc, ok := def.(*Includes)
		if !ok {
			continue
		}
		mixinDef, ok := unique[semanticName(inc.Includes)]
		if !ok {
			continue
		}
		mixin, ok := mixinDef.(*Interface)
		if !ok {
			continue
		}
		mm[semanticName(inc.Target)] = append(mm[semanticName(inc.Target)], mixin)
	}
	return mm
}

func checkDuplicateNames(defs *Definitions) []error {
	var errs []error
	for _, dup := range defs.Duplicates {
		name := defSemanticName(dup)
		first := defs.Unique[name]
		msg := fmt.Sprintf("The name %q of type %q was already seen", name, defTypeName(first))
		errs = append(errs, &ValidationError{Rule: "no-duplicate", Message: msg})
	}
	return errs
}

// semanticName strips the leading underscore from a Web IDL escaped identifier.
// Per the spec, `_Foo` and `Foo` denote the same name.
func semanticName(name string) string {
	if len(name) > 0 && name[0] == '_' {
		return name[1:]
	}
	return name
}

// defAttrs holds the three per-Definition properties used by validation rules.
// All three are extracted in a single type switch so that adding a new
// Definition type to ast.go requires updating only defAttrsOf.
type defAttrs struct {
	name      string // semantic name (underscore-stripped); "" for Includes
	isPartial bool
	typeName  string // IDL keyword used in error messages
}

// defAttrsOf extracts name, partial flag, and IDL keyword for d in one pass.
func defAttrsOf(d Definition) defAttrs {
	switch v := d.(type) {
	case *Interface:
		keyword := "interface"
		switch v.Variant {
		case IfaceMixin:
			keyword = "interface mixin"
		case IfaceCallback:
			keyword = "callback interface"
		}
		return defAttrs{name: semanticName(v.Name), isPartial: v.Partial, typeName: keyword}
	case *Dictionary:
		return defAttrs{name: semanticName(v.Name), isPartial: v.Partial, typeName: "dictionary"}
	case *Enum:
		return defAttrs{name: semanticName(v.Name), typeName: "enum"}
	case *Typedef:
		return defAttrs{name: semanticName(v.Name), typeName: "typedef"}
	case *Namespace:
		return defAttrs{name: semanticName(v.Name), isPartial: v.Partial, typeName: "namespace"}
	case *CallbackFunction:
		return defAttrs{name: semanticName(v.Name), typeName: "callback"}
	case *Includes:
		return defAttrs{typeName: "includes"}
	}
	return defAttrs{typeName: "unknown"}
}

func defSemanticName(d Definition) string { return defAttrsOf(d).name }
func defIsPartial(d Definition) bool      { return defAttrsOf(d).isPartial }
func defTypeName(d Definition) string     { return defAttrsOf(d).typeName }
