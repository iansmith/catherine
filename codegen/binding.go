package codegen

import (
	"github.com/iansmith/webidl/typemap"
	"github.com/iansmith/webidl/webidl"
)

// This file is the goja JS-binding backend (CATH-64, under CATH-62). For each
// regular interface it emits a Go type implementing goja's DynamicObject
// interface (Get/Set/Has/Delete/Keys) whose cases coerce JS values and dispatch
// into the already-generated layer-1 Go interface (see iface.go). The binding
// is the "second backend": it is emitted to its own output, not mixed into the
// layer-1 generated.go.
//
// Shape (interface I with parent P):
//
//	type IBinding struct {
//		ctx    *bindCtx   // runtime context (vm, identity cache) — defined by CATH-66
//		impl   I          // the engine object satisfying the layer-1 interface
//		parent *PBinding  // present only when I inherits; inherited keys delegate here
//	}
//
// Inheritance is handled by embed-and-delegate: IBinding handles its own
// members, and Get/Set/Has/Keys fall through to b.parent for everything else.
// Generated bodies reference the CATH-66 runtime shim (coercion helpers); the
// output is gofmt-valid but does not fully compile until that shim lands.

// NewBindingDecls emits the DynamicObject accessor decls for the interface in
// def. Returns nil for a nil def, a non-interface primary, or a mixin/callback
// interface (only regular interfaces get a binding accessor).
//
// STUB — CATH-64 Phase 0. Returns a placeholder decl so the Phase-0 tests
// render; the real emit lands in Phase 1.
func NewBindingDecls(def *webidl.MergedDef, tm typemap.Mapper, diag *Diagnostics) []Decl {
	if def == nil {
		return nil
	}
	iface, ok := def.Primary.(*webidl.Interface)
	if !ok || iface.Variant != webidl.IfaceRegular {
		return nil
	}
	return []Decl{&bindingDecl{typeName: IdentSanitize(iface.Name)}}
}

// bindingDecl is one generated DynamicObject accessor type.
type bindingDecl struct {
	typeName string // sanitized interface name, e.g. "Node"
}

func (d *bindingDecl) declName() string   { return d.typeName + "Binding" }
func (d *bindingDecl) declSource() string { return "// bindingDecl stub — CATH-64 not yet implemented\n" }
