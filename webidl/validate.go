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

// definitionValidator is implemented by AST nodes that validate themselves.
// CATH-6, CATH-7, CATH-8, CATH-9 add validate methods to the relevant types.
type definitionValidator interface {
	validate(defs *Definitions) []error
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
		mixinDef, ok := unique[inc.Includes]
		if !ok {
			continue
		}
		mixin, ok := mixinDef.(*Interface)
		if !ok {
			continue
		}
		mm[inc.Target] = append(mm[inc.Target], mixin)
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

// defSemanticName returns the semantic name of a Definition, or "" for Includes.
func defSemanticName(d Definition) string {
	switch v := d.(type) {
	case *Interface:
		return semanticName(v.Name)
	case *Dictionary:
		return semanticName(v.Name)
	case *Enum:
		return semanticName(v.Name)
	case *Typedef:
		return semanticName(v.Name)
	case *Namespace:
		return semanticName(v.Name)
	case *CallbackFunction:
		return semanticName(v.Name)
	}
	return ""
}

// defIsPartial reports whether d is a partial definition.
func defIsPartial(d Definition) bool {
	switch v := d.(type) {
	case *Interface:
		return v.Partial
	case *Dictionary:
		return v.Partial
	case *Namespace:
		return v.Partial
	}
	return false
}

// defTypeName returns the IDL keyword name for d, used in error messages.
func defTypeName(d Definition) string {
	switch v := d.(type) {
	case *Interface:
		switch v.Variant {
		case IfaceMixin:
			return "interface mixin"
		case IfaceCallback:
			return "callback interface"
		}
		return "interface"
	case *Dictionary:
		return "dictionary"
	case *Enum:
		return "enum"
	case *Typedef:
		return "typedef"
	case *Namespace:
		return "namespace"
	case *CallbackFunction:
		return "callback"
	case *Includes:
		return "includes"
	}
	return "unknown"
}
