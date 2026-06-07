package webidl

import "fmt"

// IR is the fully-resolved view of a set of Web IDL definitions produced by
// Merge. It is distinct from the raw []Definition slice returned by Parse,
// which leaves partials, mixins, and inheritance chains unresolved.
type IR struct {
	defs map[string]*MergedDef
}

// Len returns the number of named definitions in the IR.
func (ir *IR) Len() int {
	if ir == nil {
		return 0
	}
	return len(ir.defs)
}

// Lookup returns the MergedDef for the given name, or nil if not present.
func (ir *IR) Lookup(name string) *MergedDef {
	if ir == nil {
		return nil
	}
	return ir.defs[semanticName(name)]
}

// MergedDef is the resolved representation of a single named definition in the
// IR. Members from the primary definition, its partials, and any included
// mixins are all accumulated into Members. Inherited members (from parent
// interfaces via the Inheritance field) are kept separately in
// InheritedMembers, closest ancestor first.
//
// For Interface and Namespace primaries, Members holds *Attribute, *Operation,
// *Constant, *Constructor, and *IterableLike values. For Dictionary primaries,
// Members holds *Field values (which satisfy Member via the memberNode marker).
type MergedDef struct {
	// Primary is the non-partial, canonical definition.
	Primary Definition

	// Members holds all own members: primary members followed by partial
	// members (in source order within each group), followed by mixin members
	// (in includes-statement order).
	Members []Member

	// InheritedMembers holds members contributed by parent interfaces,
	// closest ancestor first. Only populated for Interface and Dictionary
	// definitions that have a non-empty Inheritance field.
	InheritedMembers []Member
}

// Merge takes the flat definition list returned by Parse and produces a
// fully-resolved IR.  Non-fatal merge errors (unknown partial targets, missing
// mixin references, inheritance cycles, etc.) are returned alongside a valid
// (possibly partial) IR.
//
// After groupDefinitions partitions the input into primaries and partials, the
// four resolution stages run in order:
//  1. Report orphan partials (partials with no matching primary).
//  2. Fold each primary together with its partials → MergedDef.Members.
//  3. Apply mixin includes → graft mixin members onto target interfaces.
//  4. Resolve inheritance chains → MergedDef.InheritedMembers (closest-first).
func Merge(defs []Definition) (*IR, []error) {
	ir := &IR{defs: make(map[string]*MergedDef)}
	if len(defs) == 0 {
		return ir, nil
	}

	grouped := groupDefinitions(defs)

	var errs []error
	errs = append(errs, reportOrphanPartials(grouped)...) // Stage 1
	foldPartials(ir, grouped)                             // Stage 2
	errs = append(errs, applyMixins(ir, defs)...)         // Stage 3
	errs = append(errs, resolveInheritance(ir)...)        // Stage 4

	return ir, errs
}

// MergeFiles merges across multiple parsed IDL files. Each element of files is
// the []Definition slice returned by one Parse call. Cross-file partials, mixin
// includes, and inheritance chains are resolved exactly as within a single file.
func MergeFiles(files [][]Definition) (*IR, []error) {
	var all []Definition
	for _, f := range files {
		all = append(all, f...)
	}
	return Merge(all)
}

// ---------------------------------------------------------------------------
// Stage helpers
// ---------------------------------------------------------------------------

// reportOrphanPartials returns one error per partial definition that has no
// matching primary in grouped.Unique.
func reportOrphanPartials(grouped Definitions) []error {
	var errs []error
	for name, partialList := range grouped.Partials {
		if _, hasPrimary := grouped.Unique[name]; !hasPrimary {
			for range partialList {
				errs = append(errs, fmt.Errorf("partial %q has no primary definition", name))
			}
		}
	}
	return errs
}

// foldPartials creates a MergedDef for each primary, accumulating the primary's
// own members followed by each partial's members (in source order) into
// MergedDef.Members, and stores the result in ir.defs.
func foldPartials(ir *IR, grouped Definitions) {
	for name, primary := range grouped.Unique {
		md := &MergedDef{Primary: primary}
		collectMembers(md, primary)
		for _, partial := range grouped.Partials[name] {
			collectMembers(md, partial)
		}
		ir.defs[name] = md
	}
}

// applyMixins walks every Includes statement in defs and grafts the referenced
// mixin's members onto the target's MergedDef. Errors are returned for unknown
// targets, unknown mixins, and includes whose referenced name is not a mixin
// variant. The same target/mixin pair is never applied twice.
func applyMixins(ir *IR, defs []Definition) []error {
	var errs []error
	applied := make(map[string]bool) // "target\x00mixin" dedup key

	for _, def := range defs {
		inc, ok := def.(*Includes)
		if !ok {
			continue
		}
		targetName := semanticName(inc.Target)
		mixinName := semanticName(inc.Includes)

		targetMD, targetOK := ir.defs[targetName]
		mixinMD, mixinOK := ir.defs[mixinName]

		if !targetOK {
			errs = append(errs, fmt.Errorf("includes target %q is not a defined interface", targetName))
		}
		if !mixinOK {
			errs = append(errs, fmt.Errorf("included mixin %q is not defined", mixinName))
		}
		if !targetOK || !mixinOK {
			continue
		}

		iface, isIface := mixinMD.Primary.(*Interface)
		if !isIface || iface.Variant != IfaceMixin {
			errs = append(errs, fmt.Errorf("included %q is not an interface mixin", mixinName))
			continue
		}

		key := targetName + "\x00" + mixinName
		if !applied[key] {
			applied[key] = true
			targetMD.Members = append(targetMD.Members, mixinMD.Members...)
		}
	}
	return errs
}

// resolveInheritance walks the inheritance chain of each Interface and
// Dictionary in the IR, accumulating ancestor own-members into
// InheritedMembers (closest ancestor first). Errors are returned for
// undefined parents and for cycles.
func resolveInheritance(ir *IR) []error {
	var errs []error
	for name, md := range ir.defs {
		parent := inheritanceOf(md.Primary)
		if parent == "" {
			continue
		}
		visited := map[string]bool{name: true}
		current := parent
		for current != "" {
			if visited[current] {
				errs = append(errs, fmt.Errorf("inheritance cycle detected involving %q", name))
				break
			}
			visited[current] = true
			parentMD, ok := ir.defs[current]
			if !ok {
				errs = append(errs, fmt.Errorf("%q inherits from %q which is not defined", name, current))
				break
			}
			md.InheritedMembers = append(md.InheritedMembers, parentMD.Members...)
			current = inheritanceOf(parentMD.Primary)
		}
	}
	return errs
}

// ---------------------------------------------------------------------------
// Low-level helpers
// ---------------------------------------------------------------------------

// collectMembers appends the own members of def into md.Members.
// Handles the three definition types that can carry members: Interface,
// Namespace, and Dictionary (whose []*Field members are cast to []Member via
// the memberNode marker added to *Field).
func collectMembers(md *MergedDef, def Definition) {
	switch d := def.(type) {
	case *Interface:
		md.Members = append(md.Members, d.Members...)
	case *Namespace:
		md.Members = append(md.Members, d.Members...)
	case *Dictionary:
		for _, f := range d.Members {
			md.Members = append(md.Members, f)
		}
	}
}

// inheritanceOf returns the semantic parent name for definitions that support
// inheritance (Interface, Dictionary), or "" if none / not applicable.
func inheritanceOf(def Definition) string {
	switch d := def.(type) {
	case *Interface:
		return semanticName(d.Inheritance)
	case *Dictionary:
		return semanticName(d.Inheritance)
	}
	return ""
}
