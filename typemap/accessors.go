package typemap

import (
	"github.com/iansmith/webidl/webidl"
)

// This file exposes richer type information for the WebIDL → goja binding
// backend (CATH-62). The default MapType deliberately collapses unions and
// callbacks to coarse Go types (`any`, plain func types); the accessors here
// recover the per-member / per-parameter detail a binding needs to emit
// runtime type-checks and goja.Callable wrappers, WITHOUT changing MapType's
// value-type output.
//
// Convention for "a JS function passed into Go" (callback / callback
// interface): the binding backend takes the CallbackSignature returned here and
// emits a Go implementation of the generated func type whose body wraps a
// goja.Callable — coercing each Go argument to a goja.Value, invoking the
// Callable, then coercing the result back to CallbackSignature.Return. The
// runtime shim that performs that coercion is CATH-62 subtask 4; this package
// only surfaces the types it needs.

// CallbackParam is one parameter of a resolved callback signature.
type CallbackParam struct {
	Name     string
	GoType   GoType
	Optional bool
	Variadic bool
}

// CallbackSignature is the resolved Go-type view of a callback function or
// callback interface operation. Return is the zero GoType (Name=="") when the
// callback returns undefined/void — mirroring codegen's buildReturnType, which
// emits no return for those. (undefined in an argument position still maps to
// `any`; the void convention applies only to the return slot.)
type CallbackSignature struct {
	Params []CallbackParam
	Return GoType
}

// UnionMembers returns the resolved member GoTypes of a union IDLType.
//
// STUB — see CATH-63 plan. Implemented in Phase 1.
func (m Mapper) UnionMembers(t *webidl.IDLType) ([]GoType, error) {
	return nil, nil
}

// CallbackFunctionSignature returns the resolved arg/return GoTypes of a
// WebIDL callback function.
//
// STUB — see CATH-63 plan. Implemented in Phase 1.
func (m Mapper) CallbackFunctionSignature(cb *webidl.CallbackFunction) (CallbackSignature, error) {
	return CallbackSignature{}, nil
}

// CallbackInterfaceSignature returns the resolved arg/return GoTypes of a
// callback interface's single regular operation (e.g. EventListener.handleEvent).
//
// STUB — see CATH-63 plan. Implemented in Phase 1.
func (m Mapper) CallbackInterfaceSignature(iface *webidl.Interface) (CallbackSignature, error) {
	return CallbackSignature{}, nil
}
