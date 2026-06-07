package webidl

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
type MergedDef struct {
	// Primary is the non-partial, canonical definition (the one without
	// Partial==true).
	Primary Definition

	// Members holds all own members: primary members followed by partial
	// members (in source order within each group), followed by mixin members
	// (in includes-statement order).
	Members []Member

	// InheritedMembers holds members contributed by parent interfaces,
	// closest ancestor first. Only populated for Interface definitions that
	// have a non-empty Inheritance field.
	InheritedMembers []Member
}

// Merge takes the flat definition list returned by Parse and produces a
// fully-resolved IR. Non-fatal merge warnings (unknown partial targets,
// missing mixin references, inheritance cycles, etc.) are returned as errors
// alongside a valid (possibly partial) IR.
//
// TODO: implement — stub returns empty IR.
func Merge(_ []Definition) (*IR, []error) {
	return &IR{defs: make(map[string]*MergedDef)}, nil
}

// MergeFiles merges across multiple parsed IDL files. Each element of files
// is the []Definition slice returned by one Parse call. Cross-file partials,
// mixin includes, and inheritance chains are resolved exactly as within a
// single file.
//
// TODO: implement — stub delegates to Merge on concatenated input.
func MergeFiles(files [][]Definition) (*IR, []error) {
	var all []Definition
	for _, f := range files {
		all = append(all, f...)
	}
	return Merge(all)
}
