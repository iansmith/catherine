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
// The four resolution stages run in order:
//  1. Partition into primaries, partials, and mixin-map (via groupDefinitions).
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

	// ------------------------------------------------------------------ //
	// Stage 1: report orphan partials (no matching primary).             //
	// ------------------------------------------------------------------ //
	for name, partialList := range grouped.Partials {
		if _, hasPrimary := grouped.Unique[name]; !hasPrimary {
			for range partialList {
				errs = append(errs, fmt.Errorf("partial %q has no primary definition", name))
			}
		}
	}

	// ------------------------------------------------------------------ //
	// Stage 2: fold primary + partials into MergedDef.Members.           //
	// ------------------------------------------------------------------ //
	for name, primary := range grouped.Unique {
		md := &MergedDef{Primary: primary}

		// Collect primary's own members.
		collectMembers(md, primary)

		// Append members from each partial in source order.
		for _, partial := range grouped.Partials[name] {
			collectMembers(md, partial)
		}

		ir.defs[name] = md
	}

	// ------------------------------------------------------------------ //
	// Stage 3: apply mixin includes.                                      //
	// Walk all Includes statements so we can produce errors for unknown  //
	// targets and unknown/invalid mixins (buildMixinMap silently drops   //
	// those; we must check them explicitly).                              //
	// ------------------------------------------------------------------ //
	applied := make(map[string]bool) // "target\x00mixin" — dedup same pair
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

		// The referenced definition must actually be a mixin variant.
		iface, isIface := mixinMD.Primary.(*Interface)
		if !isIface || iface.Variant != IfaceMixin {
			errs = append(errs, fmt.Errorf("included %q is not an interface mixin", mixinName))
			continue
		}

		// Graft mixin's own members onto the target (dedup same pair).
		key := targetName + "\x00" + mixinName
		if !applied[key] {
			applied[key] = true
			targetMD.Members = append(targetMD.Members, mixinMD.Members...)
		}
	}

	// ------------------------------------------------------------------ //
	// Stage 4: resolve inheritance chains → InheritedMembers.            //
	// ------------------------------------------------------------------ //
	for name, md := range ir.defs {
		parent := inheritanceOf(md.Primary)
		if parent == "" {
			continue
		}

		// Walk upward, collecting each ancestor's own Members (closest first).
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
// helpers
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

// inheritanceOf returns the semantic parent name for any definition that
// supports inheritance (Interface, Dictionary), or "" if none.
func inheritanceOf(def Definition) string {
	switch d := def.(type) {
	case *Interface:
		return semanticName(d.Inheritance)
	case *Dictionary:
		return semanticName(d.Inheritance)
	}
	return ""
}
