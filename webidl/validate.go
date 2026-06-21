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
//   - no argument type may be async_sequence.
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

		// replace-void: return type must not be void.
		if op.ReturnType.Base == "void" {
			errs = append(errs, &ValidationError{
				Rule:    "replace-void",
				Message: "`void` is now replaced by `undefined`.",
			})
		}
		errs = append(errs, checkLegacyIDLType(op.ReturnType)...)
	}

	// renamed-legacy: check operation's own extended attributes.
	errs = append(errs, checkLegacyExtAttrs(op.ExtAttrs)...)

	for i, arg := range op.Arguments {
		if arg.IDLType.Generic == "async_sequence" {
			errs = append(errs, &ValidationError{
				Rule:    "async-sequence-idl-to-js",
				Message: "async_sequence types cannot be used as an operation argument.",
			})
		}
		errs = append(errs, validateNullableUnionDict(arg.IDLType, defs)...)
		errs = append(errs, validateArgDictRules(arg, i, op.Arguments, defs)...)
		errs = append(errs, checkLegacyExtAttrs(arg.ExtAttrs)...)
		errs = append(errs, checkLegacyIDLType(arg.IDLType)...)
		// migrate-allowshared: [AllowShared] is placed on the *argument* (not the
		// IDLType) when the caller writes `[AllowShared] BufferSource param`.
		if arg.IDLType.Base == "BufferSource" && hasExtAttr(arg.ExtAttrs, ExtAttrAllowShared) {
			errs = append(errs, &ValidationError{
				Rule:    "migrate-allowshared",
				Message: "[AllowShared] BufferSource is now replaced with AllowSharedBufferSource.",
			})
		}
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

	// renamed-legacy: check attribute's own ExtAttrs and its IDLType (covers
	// type-level attrs like [TreatNullAs] on the attribute's type expression).
	errs = append(errs, checkLegacyExtAttrs(attr.ExtAttrs)...)
	errs = append(errs, checkLegacyIDLType(attr.IDLType)...)

	// migrate-allowshared: [AllowShared] on a BufferSource attribute is deprecated.
	// Note: [AllowShared] is an attribute-level ExtAttr, not a type-level one.
	if attr.IDLType.Base == "BufferSource" && hasExtAttr(attr.ExtAttrs, ExtAttrAllowShared) {
		errs = append(errs, &ValidationError{
			Rule:    "migrate-allowshared",
			Message: "[AllowShared] BufferSource is now replaced with AllowSharedBufferSource.",
		})
	}

	return errs
}

// ---------------------------------------------------------------------------
// Definition-level validators
// ---------------------------------------------------------------------------

// validate implements per-member rules for namespaces.
func (ns *Namespace) validate(defs *Definitions) []error {
	var errs []error

	// require-exposed: non-partial namespaces must carry [Exposed].
	if !ns.Partial && !hasExtAttr(ns.ExtAttrs, "Exposed") {
		errs = append(errs, &ValidationError{
			Rule:    "require-exposed",
			Message: "Namespaces must have [Exposed] extended attribute.",
		})
	}

	// renamed-legacy: check namespace's own extended attributes.
	errs = append(errs, checkLegacyExtAttrs(ns.ExtAttrs)...)

	for _, m := range ns.Members {
		if v, ok := m.(memberValidator); ok {
			errs = append(errs, v.validateMember(defs)...)
		}
	}

	// overload-not-distinguishable: check within this namespace body's own members.
	errs = append(errs, checkOverloadDistinguishability(ns.Members, defs)...)

	return errs
}

// validate implements constructor-member, no-cross-overload, require-exposed,
// no-constructible-global, and related CATH-9 rules for interfaces.
// The member walk also seeds the operation-name maps forwarded to checkCrossOverload,
// so iface.Members is traversed only once.
func (iface *Interface) validate(defs *Definitions) []error {
	var errs []error
	statics := make(map[string]bool)
	nonstatics := make(map[string]bool)
	hasConstructorMember := false // tracked for no-constructible-global

	// require-exposed: regular (non-partial, non-mixin, non-callback) interfaces
	// must carry [Exposed].
	if iface.Variant == IfaceRegular && !iface.Partial && !hasExtAttr(iface.ExtAttrs, "Exposed") {
		errs = append(errs, &ValidationError{
			Rule:    "require-exposed",
			Message: "Interfaces must have [Exposed] extended attribute.",
		})
	}

	// renamed-legacy: check interface's own extended attributes.
	errs = append(errs, checkLegacyExtAttrs(iface.ExtAttrs)...)

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
			hasConstructorMember = true
			for i, arg := range con.Arguments {
				errs = append(errs, validateNullableUnionDict(arg.IDLType, defs)...)
				errs = append(errs, validateArgDictRules(arg, i, con.Arguments, defs)...)
				errs = append(errs, checkLegacyExtAttrs(arg.ExtAttrs)...)
				errs = append(errs, checkLegacyIDLType(arg.IDLType)...)
				if arg.IDLType.Base == "BufferSource" && hasExtAttr(arg.ExtAttrs, ExtAttrAllowShared) {
					errs = append(errs, &ValidationError{
						Rule:    "migrate-allowshared",
						Message: "[AllowShared] BufferSource is now replaced with AllowSharedBufferSource.",
					})
				}
			}
		}
		// Rules on iterable/maplike/setlike types and arguments.
		if il, ok := m.(*IterableLike); ok {
			// obsolete-async-iterable-syntax: `async iterable` (space) is the old form.
			if il.Async && il.Kind == IterIterable {
				errs = append(errs, &ValidationError{
					Rule:    "obsolete-async-iterable-syntax",
					Message: "`async iterable` is now changed to `async_iterable`.",
				})
			}
			for _, t := range il.Types {
				errs = append(errs, validateNullableUnionDict(t, defs)...)
				errs = append(errs, checkAllowSharedIDLType(t)...)
				errs = append(errs, checkLegacyIDLType(t)...)
			}
			// Also check async-iterable buffer-size / optional arguments.
			for i, arg := range il.Arguments {
				errs = append(errs, validateNullableUnionDict(arg.IDLType, defs)...)
				errs = append(errs, validateArgDictRules(arg, i, il.Arguments, defs)...)
				errs = append(errs, checkLegacyExtAttrs(arg.ExtAttrs)...)
				errs = append(errs, checkLegacyIDLType(arg.IDLType)...)
				if arg.IDLType.Base == "BufferSource" && hasExtAttr(arg.ExtAttrs, ExtAttrAllowShared) {
					errs = append(errs, &ValidationError{
						Rule:    "migrate-allowshared",
						Message: "[AllowShared] BufferSource is now replaced with AllowSharedBufferSource.",
					})
				}
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

	// no-constructible-global: [Global] regular interfaces cannot have constructors
	// or [LegacyFactoryFunction] factory functions.
	if iface.Variant == IfaceRegular && hasExtAttr(iface.ExtAttrs, "Global") {
		if hasExtAttr(iface.ExtAttrs, "LegacyFactoryFunction") {
			errs = append(errs, &ValidationError{
				Rule:    "no-constructible-global",
				Message: "Interfaces marked as [Global] cannot have factory functions.",
			})
		}
		if hasConstructorMember {
			errs = append(errs, &ValidationError{
				Rule:    "no-constructible-global",
				Message: "Interfaces marked as [Global] cannot have constructors.",
			})
		}
	}

	// no-cross-overload: only applies to the canonical (non-partial) regular interface.
	// Mixin and callback interfaces have no equivalent rule in webidl2.js.
	if !iface.Partial && iface.Variant == IfaceRegular {
		errs = append(errs, checkCrossOverload(defs, iface, statics, nonstatics)...)
	}

	// overload-not-distinguishable: check within this definition body's own members.
	// Applies to regular and mixin interfaces (base and partials); not to callback
	// interfaces, which may not have overloaded operations.
	if iface.Variant != IfaceCallback {
		errs = append(errs, checkOverloadDistinguishability(iface.Members, defs)...)
	}

	return errs
}

// validate checks typedef IDLTypes for the no-nullable-union-dict and renamed-legacy rules.
func (td *Typedef) validate(defs *Definitions) []error {
	errs := checkLegacyIDLType(td.IDLType)
	errs = append(errs, validateNullableUnionDict(td.IDLType, defs)...)
	return errs
}

// validate checks dictionary field types for the no-nullable-union-dict and renamed-legacy rules.
func (dict *Dictionary) validate(defs *Definitions) []error {
	errs := checkLegacyExtAttrs(dict.ExtAttrs)
	for _, f := range dict.Members {
		errs = append(errs, validateNullableUnionDict(f.IDLType, defs)...)
		errs = append(errs, checkLegacyExtAttrs(f.ExtAttrs)...)
		errs = append(errs, checkLegacyIDLType(f.IDLType)...)
	}
	return errs
}

// validate checks callback arguments and return type for multiple rules.
func (cb *CallbackFunction) validate(defs *Definitions) []error {
	var errs []error

	// renamed-legacy: check callback's own extended attributes.
	errs = append(errs, checkLegacyExtAttrs(cb.ExtAttrs)...)

	for i, arg := range cb.Arguments {
		if arg.IDLType.Generic == "async_sequence" {
			errs = append(errs, &ValidationError{
				Rule:    "async-sequence-idl-to-js",
				Message: "async_sequence types cannot be returned as a callback argument.",
			})
		}
		errs = append(errs, validateNullableUnionDict(arg.IDLType, defs)...)
		errs = append(errs, validateArgDictRules(arg, i, cb.Arguments, defs)...)
		errs = append(errs, checkLegacyExtAttrs(arg.ExtAttrs)...)
		errs = append(errs, checkLegacyIDLType(arg.IDLType)...)
		if arg.IDLType.Base == "BufferSource" && hasExtAttr(arg.ExtAttrs, ExtAttrAllowShared) {
			errs = append(errs, &ValidationError{
				Rule:    "migrate-allowshared",
				Message: "[AllowShared] BufferSource is now replaced with AllowSharedBufferSource.",
			})
		}
	}

	if cb.ReturnType != nil {
		errs = append(errs, validateNullableUnionDict(cb.ReturnType, defs)...)
		// replace-void: callback return type must not be void.
		if cb.ReturnType.Base == "void" {
			errs = append(errs, &ValidationError{
				Rule:    "replace-void",
				Message: "`void` is now replaced by `undefined`.",
			})
		}
		errs = append(errs, checkLegacyIDLType(cb.ReturnType)...)
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

// ---------------------------------------------------------------------------
// CATH-9 helpers: renamed-legacy, migrate-allowshared
// ---------------------------------------------------------------------------

// legacyAttrRenames maps each deprecated extended-attribute name to its
// [Legacy…] replacement, as codified by the renamed-legacy rule.
var legacyAttrRenames = map[string]string{
	"NamedConstructor":    "LegacyFactoryFunction",
	"NoInterfaceObject":   "LegacyNoInterfaceObject",
	"OverrideBuiltins":    "LegacyOverrideBuiltIns",
	"LenientSetter":       "LegacyLenientSetter",
	"LenientThis":         "LegacyLenientThis",
	"TreatNullAs":         "LegacyNullToEmptyString",
	"Unforgeable":         "LegacyUnforgeable",
	"TreatNonObjectAsNull": "LegacyTreatNonObjectAsNull",
}

// checkLegacyExtAttrs returns a renamed-legacy ValidationError for each
// deprecated extended attribute found in attrs.
func checkLegacyExtAttrs(attrs []*ExtAttr) []error {
	var errs []error
	for _, ea := range attrs {
		if newName, deprecated := legacyAttrRenames[ea.Name]; deprecated {
			errs = append(errs, &ValidationError{
				Rule: "renamed-legacy",
				Message: fmt.Sprintf(
					"[%s] is a legacy extended attribute; use [%s] instead.",
					ea.Name, newName,
				),
			})
		}
	}
	return errs
}

// checkLegacyIDLType checks an IDLType and all its subtypes for deprecated
// type-level extended attributes (e.g. [TreatNullAs]) caught by renamed-legacy.
func checkLegacyIDLType(t *IDLType) []error {
	if t == nil {
		return nil
	}
	errs := checkLegacyExtAttrs(t.ExtAttrs)
	for _, sub := range t.Subtypes {
		errs = append(errs, checkLegacyIDLType(sub)...)
	}
	return errs
}

// checkAllowSharedIDLType fires migrate-allowshared when an IDLType carries
// [AllowShared] on a BufferSource base type. Recurses into subtypes.
func checkAllowSharedIDLType(t *IDLType) []error {
	if t == nil {
		return nil
	}
	var errs []error
	if t.Base == "BufferSource" && hasExtAttr(t.ExtAttrs, ExtAttrAllowShared) {
		errs = append(errs, &ValidationError{
			Rule:    "migrate-allowshared",
			Message: "[AllowShared] BufferSource is now replaced with AllowSharedBufferSource.",
		})
	}
	for _, sub := range t.Subtypes {
		errs = append(errs, checkAllowSharedIDLType(sub)...)
	}
	return errs
}

// idlTypeIncludesEnforceRange reports whether the IDLType carries an
// [EnforceRange] extended attribute, either directly or via a one-level typedef.
func idlTypeIncludesEnforceRange(idlType *IDLType, defs *Definitions) bool {
	if hasExtAttr(idlType.ExtAttrs, ExtAttrEnforceRange) {
		return true
	}
	if !idlType.Union && idlType.Base != "" {
		if def, ok := defs.Unique[semanticName(idlType.Base)]; ok {
			if td, ok := def.(*Typedef); ok {
				return hasExtAttr(td.IDLType.ExtAttrs, ExtAttrEnforceRange)
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
// Overload distinguishability (overload-not-distinguishable rule, §3.2.11)
// ---------------------------------------------------------------------------

// resolveIDLType follows typedef chains to reach the concrete underlying type,
// guarding against cycles with a visited set. Non-typedef named types (Interface,
// Dictionary, …) and union/generic types are returned as-is.
func resolveIDLType(t *IDLType, defs *Definitions, visited map[string]bool) *IDLType {
	if t == nil || t.Union || t.Generic != "" {
		return t
	}
	name := semanticName(t.Base)
	if name == "" {
		return t
	}
	if visited[name] {
		return t
	}
	def, ok := defs.Unique[name]
	if !ok {
		return t
	}
	td, ok := def.(*Typedef)
	if !ok {
		return t
	}
	visited[name] = true
	return resolveIDLType(td.IDLType, defs, visited)
}

// numericIDLTypes is the set of IDL numeric scalar type names (all share one bucket).
var numericIDLTypes = map[string]bool{
	"byte": true, "octet": true,
	"short": true, "unsigned short": true,
	"long": true, "unsigned long": true,
	"long long": true, "unsigned long long": true,
	"float": true, "unrestricted float": true,
	"double": true, "unrestricted double": true,
}

// stringIDLTypes is the set of IDL string type names (all share one bucket).
var stringIDLTypes = map[string]bool{
	"DOMString": true, "ByteString": true, "USVString": true, "CSSOMString": true,
}

// ifaceBucketPrefix is the prefix used for named-interface buckets so that
// distinct interface types sort into distinct buckets without an extra import.
const ifaceBucketPrefix = "interface:"

func isIfaceBucket(b string) bool {
	return len(b) > len(ifaceBucketPrefix) && b[:len(ifaceBucketPrefix)] == ifaceBucketPrefix
}

// idlTypeBucket returns the §3.2.11 distinguishability bucket string for a
// typedef-resolved, non-nullable IDLType. Named interface types use the prefix
// "interface:<name>" so two different interfaces map to different buckets while
// still being subject to the object-vs-interface rule.
func idlTypeBucket(t *IDLType, defs *Definitions) string {
	if t.Base == "any" {
		return "any"
	}
	if t.Union {
		return "union" // caller handles union members individually
	}
	if t.Generic != "" {
		// record has its own bucket; every other generic (sequence, FrozenArray,
		// ObservableArray, …) is treated as a sequence.
		if t.Generic == "record" {
			return "record"
		}
		return "sequence"
	}
	name := semanticName(t.Base)
	switch {
	case name == "boolean":
		return "boolean"
	case name == "undefined" || name == "void":
		return "undefined"
	case name == "bigint":
		return "bigint"
	case name == "object":
		return "object"
	case name == "symbol":
		return "symbol"
	case numericIDLTypes[name]:
		return "numeric"
	case stringIDLTypes[name]:
		return "string"
	}
	if defs != nil {
		if def, ok := defs.Unique[name]; ok {
			switch def.(type) {
			case *Dictionary:
				return "dictionary"
			case *CallbackFunction:
				return "callback"
			case *Enum:
				return "string" // enums are string-typed in WebIDL
			case *Interface:
				return ifaceBucketPrefix + name
			}
		}
	}
	return ifaceBucketPrefix + name // unknown named type treated as interface-like
}

// bucketsDistinguishable reports whether two §3.2.11 bucket strings represent
// types that are distinguishable from each other.
func bucketsDistinguishable(b1, b2 string) bool {
	if b1 == "any" || b2 == "any" {
		return false
	}
	if b1 == b2 {
		return false // same bucket (numeric×numeric, string×string, sequence×sequence, …)
	}
	// Two distinct named interface types are distinguishable from each other.
	if isIfaceBucket(b1) && isIfaceBucket(b2) {
		return true
	}
	// object is not distinguishable from interface, callback, dictionary, sequence, or record
	// because all are JavaScript objects and the engine cannot tell them apart at the
	// call site without deeper type inspection.
	isObjectLike := func(b string) bool {
		return isIfaceBucket(b) || b == "callback" || b == "dictionary" || b == "sequence" || b == "record"
	}
	if b1 == "object" && isObjectLike(b2) {
		return false
	}
	if b2 == "object" && isObjectLike(b1) {
		return false
	}
	return true
}

// typesDistinguishable reports whether IDL types t1 and t2 are distinguishable
// per spec §3.2.11. It resolves typedef chains, handles nullable, and for
// union types checks all cross-product pairs of member types.
func typesDistinguishable(t1, t2 *IDLType, defs *Definitions) bool {
	if t1 == nil || t2 == nil {
		return false
	}
	r1 := resolveIDLType(t1, defs, make(map[string]bool))
	r2 := resolveIDLType(t2, defs, make(map[string]bool))

	// Track nullable from both the original reference and the resolved form.
	n1 := t1.Nullable || r1.Nullable
	n2 := t2.Nullable || r2.Nullable

	// §3.2.11.1 step 1: a nullable type vs. a plain dictionary is not distinguishable
	// because passing null satisfies both — null is an accepted dictionary value.
	isDictBucket := func(r *IDLType) bool {
		return !r.Union && r.Generic == "" && idlTypeBucket(r, defs) == "dictionary"
	}
	if (n1 && isDictBucket(r2)) || (n2 && isDictBucket(r1)) {
		return false
	}

	// Build the flat member sets. The parser already flattens nested unions, so
	// Subtypes contains the leaf members for union types.
	members := func(r *IDLType) []*IDLType {
		if r.Union {
			return r.Subtypes
		}
		return []*IDLType{r}
	}

	// Resolve and strip nullable from each member, then map to a bucket string.
	toBuckets := func(ms []*IDLType) []string {
		out := make([]string, len(ms))
		for i, m := range ms {
			rm := resolveIDLType(m, defs, make(map[string]bool))
			if rm.Nullable {
				cp := *rm
				cp.Nullable = false
				rm = &cp
			}
			out[i] = idlTypeBucket(rm, defs)
		}
		return out
	}
	buckets1 := toBuckets(members(r1))
	buckets2 := toBuckets(members(r2))

	// §3.2.11 step 4: if any cross-pair (u1, u2) is not distinguishable, the
	// whole pair t1/t2 is not distinguishable.
	for _, b1 := range buckets1 {
		for _, b2 := range buckets2 {
			if !bucketsDistinguishable(b1, b2) {
				return false
			}
		}
	}
	return true
}

// opArgCounts returns the effective argument count bounds for op.
// min = count of required (non-optional, non-variadic) args.
// max = total declared arg count.
// variadic = true if the last arg uses the ... spread syntax.
func opArgCounts(op *Operation) (min, max int, variadic bool) {
	max = len(op.Arguments)
	for _, arg := range op.Arguments {
		if arg.Variadic {
			variadic = true
			break
		}
		if !arg.Optional {
			min++
		}
	}
	return
}

// effectiveArgIDLType returns the IDLType at the given position in op's
// effective argument list. For variadic operations the last arg's type is
// returned for any position beyond the declared arg count.
func effectiveArgIDLType(op *Operation, pos int) *IDLType {
	if pos < len(op.Arguments) {
		return op.Arguments[pos].IDLType
	}
	if len(op.Arguments) > 0 && op.Arguments[len(op.Arguments)-1].Variadic {
		return op.Arguments[len(op.Arguments)-1].IDLType
	}
	return nil
}

// overloadPairDistinguishable reports whether two operations of the same name
// are distinguishable per §3.2.11. The pair is distinguishable if there exists
// some effective argument count N and position i < N where the types differ in
// a spec-distinguishable way. If any effective count N where both operations
// appear yields no distinguishing position, the pair is not distinguishable.
func overloadPairDistinguishable(a, b *Operation, defs *Definitions) bool {
	minA, maxA, variadicA := opArgCounts(a)
	minB, maxB, variadicB := opArgCounts(b)

	// Scan up to the larger of the two declared arg counts. For variadic ops,
	// positions beyond their declared count repeat the variadic type; checking up
	// to the other op's declared count is always sufficient.
	upperN := maxA
	if maxB > upperN {
		upperN = maxB
	}

	for n := 0; n <= upperN; n++ {
		inA := n >= minA && (variadicA || n <= maxA)
		inB := n >= minB && (variadicB || n <= maxB)
		if !inA || !inB {
			continue
		}
		// Both operations appear in the effective overload set at size n.
		// Look for a distinguishing argument position.
		distinguishedAtN := false
		for pos := 0; pos < n; pos++ {
			tA := effectiveArgIDLType(a, pos)
			tB := effectiveArgIDLType(b, pos)
			if tA == nil || tB == nil {
				continue
			}
			if typesDistinguishable(tA, tB, defs) {
				distinguishedAtN = true
				break
			}
		}
		if !distinguishedAtN {
			return false
		}
	}
	return true
}

// checkOverloadDistinguishability implements the overload-not-distinguishable
// rule for a single definition body (the Members slice of one interface or
// namespace declaration). It groups named operations by (name, isStatic) and
// reports a ValidationError for every pair within a group that is not
// distinguishable at any effective argument position.
func checkOverloadDistinguishability(members []Member, defs *Definitions) []error {
	type opKey struct {
		name     string
		isStatic bool
	}
	groups := make(map[opKey][]*Operation)
	for _, m := range members {
		op, ok := m.(*Operation)
		if !ok || op.Name == "" {
			continue
		}
		// Only regular and static operations form overload sets per §3.2.11.
		// Named special ops (getter, setter, deleter, stringifier, legacy caller)
		// are resolved by separate mechanisms and must not be grouped here.
		if op.Special != "" && op.Special != "static" {
			continue
		}
		key := opKey{name: op.Name, isStatic: op.Special == "static"}
		groups[key] = append(groups[key], op)
	}

	var errs []error
	for key, ops := range groups {
		if len(ops) < 2 {
			continue
		}
		for i := 0; i < len(ops); i++ {
			for j := i + 1; j < len(ops); j++ {
				if !overloadPairDistinguishable(ops[i], ops[j], defs) {
					errs = append(errs, &ValidationError{
						Rule:    "overload-not-distinguishable",
						Message: fmt.Sprintf("The overloads of operation %q are not distinguishable.", key.name),
					})
				}
			}
		}
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
