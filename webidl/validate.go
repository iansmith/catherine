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

// ---------------------------------------------------------------------------
// Member-level validators
// ---------------------------------------------------------------------------

// validateMember implements the incomplete-op rule and async-sequence-idl-to-js:
//   - regular and static operations must have both a return type and an identifier.
//   - the return type must not be async_sequence.
//   - no argument type may be a nullable union containing a dictionary.
func (op *Operation) validateMember(defs *Definitions) []error {
	var errs []error

	if op.Name == "" && (op.Special == "" || op.Special == "static") {
		errs = append(errs, &ValidationError{
			Rule:    "incomplete-op",
			Message: "Regular or static operations must have both a return type and an identifier.",
		})
	}

	if op.ReturnType != nil {
		if op.ReturnType.Generic == "async_sequence" {
			errs = append(errs, &ValidationError{
				Rule:    "async-sequence-idl-to-js",
				Message: "async_sequence types cannot be returned by an operation.",
			})
		}
		errs = append(errs, validateNullableUnionDict(op.ReturnType, defs)...)
	}

	for i, arg := range op.Arguments {
		errs = append(errs, validateNullableUnionDict(arg.IDLType, defs)...)
		errs = append(errs, validateArgDictRules(arg, i, op.Arguments, defs)...)
	}

	return errs
}

// validateMember implements the attr-invalid-type rule for attributes:
//   - attributes may not have sequence, record, or async_sequence types.
//   - attributes may not have dictionary types (directly, via union, or via typedef).
//   - readonly attributes may not have [EnforceRange] on their type (directly or via typedef).
//
// Also checks no-nullable-union-dict for the attribute's IDLType.
func (attr *Attribute) validateMember(defs *Definitions) []error {
	var errs []error

	// Rule: sequence / record / async_sequence not allowed as attribute types.
	if g := attr.IDLType.Generic; g == "async_sequence" || g == "sequence" || g == "record" {
		errs = append(errs, &ValidationError{
			Rule:    "attr-invalid-type",
			Message: fmt.Sprintf("Attributes cannot accept %s types.", g),
		})
	}

	// Rule: dictionary types (or unions/typedefs containing one) not allowed.
	if idlTypeIncludesDictionary(attr.IDLType, defs) != nil {
		errs = append(errs, &ValidationError{
			Rule:    "attr-invalid-type",
			Message: "Attributes cannot accept dictionary types.",
		})
	}

	// Rule: readonly attributes may not use [EnforceRange].
	if attr.Readonly && idlTypeIncludesEnforceRange(attr.IDLType, defs) {
		errs = append(errs, &ValidationError{
			Rule:    "attr-invalid-type",
			Message: "Readonly attributes cannot accept [EnforceRange] extended attribute.",
		})
	}

	// no-nullable-union-dict applies to the attribute's IDLType as well.
	errs = append(errs, validateNullableUnionDict(attr.IDLType, defs)...)

	return errs
}

// ---------------------------------------------------------------------------
// Definition-level validators
// ---------------------------------------------------------------------------

// validate implements per-member rules for namespaces (currently: incomplete-op,
// async-sequence-idl-to-js).
func (ns *Namespace) validate(defs *Definitions) []error {
	var errs []error
	for _, m := range ns.Members {
		if v, ok := m.(memberValidator); ok {
			errs = append(errs, v.validateMember(defs)...)
		}
	}
	return errs
}

// validate implements constructor-member and no-cross-overload rules for interfaces.
// The member walk also seeds the operation-name maps forwarded to checkCrossOverload,
// so iface.Members is traversed only once.
func (iface *Interface) validate(defs *Definitions) []error {
	var errs []error
	statics := make(map[string]bool)
	nonstatics := make(map[string]bool)

	// Single pass: run per-member rules and seed the cross-overload maps.
	for _, m := range iface.Members {
		if v, ok := m.(memberValidator); ok {
			errs = append(errs, v.validateMember(defs)...)
		}
		if op, ok := m.(*Operation); ok {
			seedOp(op, statics, nonstatics)
		}
		// no-nullable-union-dict and dict-arg rules on constructor argument types.
		// *Constructor has no validateMember (it carries no rule of its own),
		// so we handle its argument types explicitly here.
		if con, ok := m.(*Constructor); ok {
			for i, arg := range con.Arguments {
				errs = append(errs, validateNullableUnionDict(arg.IDLType, defs)...)
				errs = append(errs, validateArgDictRules(arg, i, con.Arguments, defs)...)
			}
		}
		// no-nullable-union-dict and dict-arg rules on iterable/maplike/setlike
		// types and arguments.
		if il, ok := m.(*IterableLike); ok {
			for _, t := range il.Types {
				errs = append(errs, validateNullableUnionDict(t, defs)...)
			}
			// Also check async-iterable buffer-size / optional arguments.
			for i, arg := range il.Arguments {
				errs = append(errs, validateNullableUnionDict(arg.IDLType, defs)...)
				errs = append(errs, validateArgDictRules(arg, i, il.Arguments, defs)...)
			}
		}
	}

	// constructor-member: [Constructor] extended attribute is the legacy form.
	// Only applies to regular interfaces — webidl2.js rejects [Constructor] on
	// mixins and callback interfaces at parse time, so the validator should not
	// fire for those variants.
	if iface.Variant == IfaceRegular && hasExtAttr(iface.ExtAttrs, "Constructor") {
		errs = append(errs, &ValidationError{
			Rule: "constructor-member",
			Message: "Constructors should now be represented as a `constructor()` operation " +
				"on the interface instead of `[Constructor]` extended attribute.",
		})
	}

	// no-cross-overload: only applies to the canonical (non-partial) regular interface.
	// Mixin and callback interfaces have no equivalent rule in webidl2.js.
	if !iface.Partial && iface.Variant == IfaceRegular {
		errs = append(errs, checkCrossOverload(defs, iface, statics, nonstatics)...)
	}

	return errs
}

// validate checks typedef IDLTypes for the no-nullable-union-dict rule.
func (td *Typedef) validate(defs *Definitions) []error {
	return validateNullableUnionDict(td.IDLType, defs)
}

// validate checks dictionary field types for the no-nullable-union-dict rule.
func (dict *Dictionary) validate(defs *Definitions) []error {
	var errs []error
	for _, f := range dict.Members {
		errs = append(errs, validateNullableUnionDict(f.IDLType, defs)...)
	}
	return errs
}

// validate checks callback arguments for the async-sequence-idl-to-js rule and
// the return type + arguments for no-nullable-union-dict and dict-arg rules.
func (cb *CallbackFunction) validate(defs *Definitions) []error {
	var errs []error

	for i, arg := range cb.Arguments {
		if arg.IDLType.Generic == "async_sequence" {
			errs = append(errs, &ValidationError{
				Rule:    "async-sequence-idl-to-js",
				Message: "async_sequence types cannot be returned as a callback argument.",
			})
		}
		errs = append(errs, validateNullableUnionDict(arg.IDLType, defs)...)
		errs = append(errs, validateArgDictRules(arg, i, cb.Arguments, defs)...)
	}

	if cb.ReturnType != nil {
		errs = append(errs, validateNullableUnionDict(cb.ReturnType, defs)...)
	}

	return errs
}

// ---------------------------------------------------------------------------
// Type-level helpers
// ---------------------------------------------------------------------------

// idlTypeWalkDicts is the shared traversal kernel used by
// idlTypeIncludesDictionary and dictionaryFromIDLTypeHasRequiredField. It walks
// t following typedef chains (with a visited cycle guard) and union/generic
// subtypes. The nullable check fires at the *Dictionary leaf, so typedef aliases
// that are themselves nullable (e.g. TD?) are still traversed.
//
// For each non-nullable *Dictionary leaf found, f is called with the dictionary
// name. The walk halts and returns true the first time f returns true.
//
// When a named type is not found in defs, returnOnUnknown is returned (false for
// presence-only queries, true for conservative required-field queries).
func idlTypeWalkDicts(t *IDLType, defs *Definitions, visited map[string]bool, returnOnUnknown bool, f func(name string) bool) bool {
	if !t.Union {
		name := semanticName(t.Base)
		if name != "" {
			def, ok := defs.Unique[name]
			if ok {
				switch d := def.(type) {
				case *Typedef:
					// Cycle guard: if we've already started evaluating this typedef,
					// treat the result as indeterminate to break the cycle.
					if !visited[name] {
						visited[name] = true
						if idlTypeWalkDicts(d.IDLType, defs, visited, returnOnUnknown, f) {
							return true
						}
					}
				case *Dictionary:
					// Only non-nullable references count — Dict? is excluded.
					if !t.Nullable {
						return f(name)
					}
				}
			} else if returnOnUnknown {
				// Unknown name in conservative mode (required-field queries).
				return true
			}
			// Unknown name + returnOnUnknown=false: fall through to subtype walk so
			// generics like UnknownContainer<Dict> are still covered.
		}
	}
	// Walk subtypes (union members and generic type parameters).
	for _, sub := range t.Subtypes {
		if idlTypeWalkDicts(sub, defs, visited, returnOnUnknown, f) {
			return true
		}
	}
	return false
}

// idlTypeIncludesDictionary returns the first IDLType reference in the type
// tree that resolves to a non-nullable dictionary, or nil if none is found. It
// follows typedef chains (with a cycle guard) and walks union subtypes and
// generic parameters. Nullable bare-dictionary references (Dict?) are NOT
// counted — only non-nullable ones.
func idlTypeIncludesDictionary(idlType *IDLType, defs *Definitions) *IDLType {
	var found *IDLType
	idlTypeWalkDicts(idlType, defs, make(map[string]bool), false, func(_ string) bool {
		found = idlType
		return true // stop on first dict found
	})
	return found
}

// hasExtAttr reports whether any entry in attrs has the given name.
func hasExtAttr(attrs []*ExtAttr, name string) bool {
	for _, ea := range attrs {
		if ea.Name == name {
			return true
		}
	}
	return false
}

// idlTypeIncludesEnforceRange reports whether the IDLType carries an
// [EnforceRange] extended attribute, either directly or via a one-level typedef.
func idlTypeIncludesEnforceRange(idlType *IDLType, defs *Definitions) bool {
	if hasExtAttr(idlType.ExtAttrs, "EnforceRange") {
		return true
	}
	if !idlType.Union && idlType.Base != "" {
		if def, ok := defs.Unique[semanticName(idlType.Base)]; ok {
			if td, ok := def.(*Typedef); ok {
				return hasExtAttr(td.IDLType.ExtAttrs, "EnforceRange")
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Dictionary-argument rule helpers (CATH-8)
// ---------------------------------------------------------------------------

// idlTypeNullableInnerIncludesDictionary reports whether t is a nullable type
// whose non-nullable inner type includes or is a dictionary. This is the check
// used by the no-nullable-dict-arg rule: Dict? fires, but non-nullable Dict does
// not (that's covered by dict-arg-optional instead).
func idlTypeNullableInnerIncludesDictionary(t *IDLType, defs *Definitions) bool {
	if !t.Nullable {
		return false
	}
	if t.Union {
		// Nullable union (e.g. (boolean or Dict)?): use the standard helper which
		// correctly treats each member's own nullable flag independently.
		return idlTypeIncludesDictionary(t, defs) != nil
	}
	// Nullable non-union reference — e.g. Dict? or a nullable typedef alias.
	name := semanticName(t.Base)
	if name == "" {
		return false
	}
	def, ok := defs.Unique[name]
	if !ok {
		return false
	}
	switch d := def.(type) {
	case *Dictionary:
		return true // direct nullable dict reference (Dict?)
	case *Typedef:
		return idlTypeIncludesDictionary(d.IDLType, defs) != nil // e.g. Union?
	}
	return false
}

// dictionaryFromIDLTypeHasRequiredField returns true when any dictionary reached
// through t has at least one required field in itself or in an ancestor
// dictionary. Returns true (conservative) when a type in the chain is unknown —
// this prevents false-positive dict-arg-optional warnings on types whose full
// definition is not available in defs.
func dictionaryFromIDLTypeHasRequiredField(t *IDLType, defs *Definitions) bool {
	return idlTypeWalkDicts(t, defs, make(map[string]bool), true,
		func(name string) bool {
			return dictHasRequiredFieldRec(name, defs, make(map[string]bool))
		})
}

// dictHasRequiredFieldRec walks d's field list and inheritance chain for a required
// field. visited is a cycle guard for circular inheritance (e.g. A: B, B: A).
// Returns true (conservative) when a named parent dictionary is absent from defs.
func dictHasRequiredFieldRec(name string, defs *Definitions, visited map[string]bool) bool {
	if name == "" || visited[name] {
		return false
	}
	visited[name] = true
	def, ok := defs.Unique[name]
	if !ok {
		return true // unknown parent — treat conservatively
	}
	dict, ok := def.(*Dictionary)
	if !ok {
		return false
	}
	for _, f := range dict.Members {
		if f.Required {
			return true
		}
	}
	if dict.Inheritance != "" {
		return dictHasRequiredFieldRec(semanticName(dict.Inheritance), defs, visited)
	}
	return false
}

// isLastRequiredArgument returns true when no arg at an index > idx is both
// non-optional AND has a type that does not include a dictionary. Such an arg
// would be a "required non-dict argument after the current one", which exempts the
// current dict arg from the dict-arg-optional rule (because callers must provide
// that later required arg anyway, making the dict arg effectively required too).
func isLastRequiredArgument(idx int, args []*Argument, defs *Definitions) bool {
	for _, a := range args[idx+1:] {
		if !a.Optional && idlTypeIncludesDictionary(a.IDLType, defs) == nil {
			return false // a required non-dict arg follows → current is NOT the last required
		}
	}
	return true
}

// validateArgDictRules checks the three CATH-8 dictionary-argument rules for a
// single argument. idx is the argument's 0-based position in allArgs (needed for
// the isLastRequiredArgument check). Rules fire in the order specified by the JS
// reference implementation: no-nullable-dict-arg → dict-arg-default / dict-arg-optional.
//
// If no-nullable-dict-arg fires the function returns immediately. The corpus
// baseline confirms that dict-arg-default and dict-arg-optional never co-fire
// with it, and the early exit also prevents a false-positive dict-arg-optional
// on nullable typedef aliases (e.g. TD? where TD resolves to a dictionary) —
// idlTypeIncludesDictionary follows the typedef chain through the nullable,
// but the arg is already flagged invalid by no-nullable-dict-arg.
func validateArgDictRules(arg *Argument, idx int, allArgs []*Argument, defs *Definitions) []error {
	// no-nullable-dict-arg: nullable type whose inner type includes a dictionary.
	if idlTypeNullableInnerIncludesDictionary(arg.IDLType, defs) {
		return []error{&ValidationError{
			Rule:    "no-nullable-dict-arg",
			Message: "Dictionary arguments cannot be nullable.",
		}}
	}

	if idlTypeIncludesDictionary(arg.IDLType, defs) != nil {
		if arg.Optional {
			// dict-arg-default: optional dict argument must default to {}.
			if arg.Default == nil || arg.Default.Kind != CVDictionary {
				return []error{&ValidationError{
					Rule:    "dict-arg-default",
					Message: "Optional dictionary arguments must have a default value of `{}`.",
				}}
			}
		} else {
			// dict-arg-optional: non-optional dict arg with no required fields must
			// be optional (unless a required non-dict arg follows, making the position
			// effectively required anyway).
			if !dictionaryFromIDLTypeHasRequiredField(arg.IDLType, defs) &&
				isLastRequiredArgument(idx, allArgs, defs) {
				return []error{&ValidationError{
					Rule:    "dict-arg-optional",
					Message: "Dictionary argument must be optional if it has no required fields",
				}}
			}
		}
	}

	return nil
}

// validateNullableUnionDict mirrors the no-nullable-union-dict logic in
// webidl2.js type.js: a nullable union (or a nullable reference to a typedef
// whose type is a union) that contains a dictionary type is invalid.
//
// When a nullable union is detected, the function checks once for a dictionary
// member and emits at most one error. Otherwise it recurses into subtypes so
// that inner nullable unions are caught too.
func validateNullableUnionDict(idlType *IDLType, defs *Definitions) []error {
	// Determine the "target" union:
	//   • If idlType is itself a union, it is the target.
	//   • If idlType is a non-union reference that resolves to ANY typedef,
	//     that typedef's IDLType is the target.  We do NOT require
	//     td.IDLType.Union here — idlTypeIncludesDictionary already follows
	//     multi-level typedef chains, so passing a non-union typedef IDLType
	//     as the target is safe and necessary for two-hop chains like
	//     V? → typedef U → typedef (Dict or boolean).
	//   • Otherwise there is no target (no union in scope).
	var target *IDLType
	if idlType.Union {
		target = idlType
	} else if idlType.Base != "" {
		if def, ok := defs.Unique[semanticName(idlType.Base)]; ok {
			if td, ok := def.(*Typedef); ok {
				target = td.IDLType
			}
		}
	}

	if target != nil && idlType.Nullable {
		// Nullable union (or nullable typedef-to-union): disallow any dictionary member.
		if idlTypeIncludesDictionary(target, defs) != nil {
			return []error{&ValidationError{
				Rule:    "no-nullable-union-dict",
				Message: "Nullable union cannot include a dictionary type.",
			}}
		}
		return nil
	}

	// Not a nullable union / typedef-to-union: recurse into subtypes so that
	// inner nullable unions are still caught.
	var errs []error
	for _, sub := range idlType.Subtypes {
		errs = append(errs, validateNullableUnionDict(sub, defs)...)
	}
	return errs
}

// ---------------------------------------------------------------------------
// Cross-overload helpers (no-cross-overload rule)
// ---------------------------------------------------------------------------

// seedOp records op.Name in statics (for static operations) or nonstatics (for all
// others). Unnamed operations (getters, setters, …) are silently skipped.
func seedOp(op *Operation, statics, nonstatics map[string]bool) {
	if op.Name == "" {
		return
	}
	if op.Special == "static" {
		statics[op.Name] = true
	} else {
		nonstatics[op.Name] = true
	}
}

// checkCrossOverload detects operations re-defined across partials or included mixins.
// statics and nonstatics are the base interface's own operation names, pre-seeded by
// the caller during its member walk. Operations may be overloaded within the same scope,
// but not across scopes.
func checkCrossOverload(defs *Definitions, iface *Interface, statics, nonstatics map[string]bool) []error {
	name := semanticName(iface.Name)
	var errs []error

	checkExtension := func(members []Member) {
		// Pass 1: check each operation against base + already-accumulated names.
		for _, m := range members {
			op, ok := m.(*Operation)
			if !ok || op.Name == "" {
				continue
			}
			if op.Special == "static" {
				if statics[op.Name] {
					errs = append(errs, &ValidationError{
						Rule:    "no-cross-overload",
						Message: fmt.Sprintf("The static operation %q has already been defined for the base interface %q either in itself or in a mixin", op.Name, name),
					})
				}
			} else {
				if nonstatics[op.Name] {
					errs = append(errs, &ValidationError{
						Rule:    "no-cross-overload",
						Message: fmt.Sprintf("The operation %q has already been defined for the base interface %q either in itself or in a mixin", op.Name, name),
					})
				}
			}
		}
		// Pass 2: accumulate names so subsequent extensions see earlier ones.
		for _, m := range members {
			if op, ok := m.(*Operation); ok {
				seedOp(op, statics, nonstatics)
			}
		}
	}

	for _, p := range defs.Partials[name] {
		if pi, ok := p.(*Interface); ok {
			checkExtension(pi.Members)
		}
	}
	for _, mixin := range defs.MixinMap[name] {
		checkExtension(mixin.Members)
	}

	return errs
}

// ---------------------------------------------------------------------------
// Top-level entry point
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// groupDefinitions and related helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Definition introspection helpers
// ---------------------------------------------------------------------------

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
