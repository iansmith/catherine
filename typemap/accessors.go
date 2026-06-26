package typemap

import (
	"fmt"

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

// UnionMembers returns the resolved member GoTypes of a union IDLType, in
// declaration order. It is the per-member counterpart to MapType, which
// deliberately collapses every union to `any`: a binding backend needs the
// individual member types to emit runtime type-checks and overload dispatch.
//
// Nested unions are flattened to their leaf members (any depth), so a binding
// sees a flat list of alternatives. Duplicates are preserved — distinct IDL
// types that share a Go representation (e.g. DOMString and USVString both →
// string) are each kept, since the binding still maps each IDL alternative to
// its own coercion. Each member carries the Unresolved flag MapType produced,
// so an unmapped interface name (→ any, Unresolved:true) is distinguishable
// from a genuine `any`.
//
// Returns an error if t is nil, not a union (t.Union == false), malformed
// (Union and Generic both set), has no members, contains a nil member, or
// contains a member whose own type fails to map.
func (m Mapper) UnionMembers(t *webidl.IDLType) ([]GoType, error) {
	if t == nil {
		return nil, fmt.Errorf("UnionMembers: nil IDLType")
	}
	if !t.Union {
		return nil, fmt.Errorf("UnionMembers: IDLType is not a union (Base=%q, Generic=%q)", t.Base, t.Generic)
	}
	return m.appendUnionMembers(nil, t)
}

// appendUnionMembers validates the union node u and appends its resolved member
// GoTypes to out, recursing into nested unions. u is assumed to be a union node
// (u.Union == true).
func (m Mapper) appendUnionMembers(out []GoType, u *webidl.IDLType) ([]GoType, error) {
	if u.Generic != "" {
		return nil, fmt.Errorf("UnionMembers: union has both Union and Generic set (malformed node)")
	}
	if len(u.Subtypes) == 0 {
		return nil, fmt.Errorf("UnionMembers: union has no members")
	}
	for i, sub := range u.Subtypes {
		switch {
		case sub == nil:
			return nil, fmt.Errorf("UnionMembers: union member %d is nil", i)
		case sub.Union:
			// Nested union — inline its leaf members. The recursive call
			// re-validates the node (malformed Union+Generic, empty members).
			var err error
			out, err = m.appendUnionMembers(out, sub)
			if err != nil {
				return nil, err
			}
		default:
			gt, err := m.MapType(sub)
			if err != nil {
				return nil, fmt.Errorf("UnionMembers: member %d: %w", i, err)
			}
			out = append(out, gt)
		}
	}
	return out, nil
}

// CallbackFunctionSignature returns the resolved argument and return GoTypes of
// a WebIDL callback function (`callback Name = ReturnType (Args...)`). Returns
// an error if cb is nil, if any argument is nil, or if an argument/return type
// fails to map.
func (m Mapper) CallbackFunctionSignature(cb *webidl.CallbackFunction) (CallbackSignature, error) {
	if cb == nil {
		return CallbackSignature{}, fmt.Errorf("CallbackFunctionSignature: nil CallbackFunction")
	}
	return m.signatureFrom(cb.Arguments, cb.ReturnType)
}

// CallbackInterfaceSignature returns the resolved argument and return GoTypes of
// a callback interface's single regular operation (e.g. EventListener's
// `undefined handleEvent(Event event)`).
//
// "Regular operation" means an Operation with Special == "" — getter/setter/
// deleter/stringifier/static operations are not the callback entry point and
// are skipped, as are constants. A callback interface must have exactly one
// such operation. (Note: this filter intentionally diverges from
// codegen.buildCallbackDecls, which does not filter on Special; do not "align"
// them — a binding needs the one invocable operation, not every member.)
//
// Returns an error if iface is nil, is not a callback interface
// (Variant != IfaceCallback), or does not have exactly one regular operation.
func (m Mapper) CallbackInterfaceSignature(iface *webidl.Interface) (CallbackSignature, error) {
	if iface == nil {
		return CallbackSignature{}, fmt.Errorf("CallbackInterfaceSignature: nil Interface")
	}
	if iface.Variant != webidl.IfaceCallback {
		return CallbackSignature{}, fmt.Errorf("CallbackInterfaceSignature: interface %q is not a callback interface", iface.Name)
	}
	var op *webidl.Operation
	count := 0
	for _, member := range iface.Members {
		o, ok := member.(*webidl.Operation)
		if !ok || o.Special != "" {
			continue
		}
		count++
		op = o
	}
	if count != 1 {
		return CallbackSignature{}, fmt.Errorf("CallbackInterfaceSignature: callback interface %q has %d regular operations, want exactly 1", iface.Name, count)
	}
	return m.signatureFrom(op.Arguments, op.ReturnType)
}

// signatureFrom resolves a shared argument list + return type into a
// CallbackSignature. Used by both callback functions and callback interface
// operations. Returns an error on a nil argument or an unmappable type.
func (m Mapper) signatureFrom(args []*webidl.Argument, returnType *webidl.IDLType) (CallbackSignature, error) {
	params := make([]CallbackParam, 0, len(args))
	for i, a := range args {
		if a == nil {
			return CallbackSignature{}, fmt.Errorf("argument %d is nil", i)
		}
		gt, err := m.MapType(a.IDLType)
		if err != nil {
			return CallbackSignature{}, fmt.Errorf("argument %q: %w", a.Name, err)
		}
		params = append(params, CallbackParam{
			Name:     a.Name,
			GoType:   gt,
			Optional: a.Optional,
			Variadic: a.Variadic,
		})
	}
	ret, err := m.returnGoType(returnType)
	if err != nil {
		return CallbackSignature{}, fmt.Errorf("return type: %w", err)
	}
	return CallbackSignature{Params: params, Return: ret}, nil
}

// returnGoType resolves a return-position IDLType, mirroring
// codegen.buildReturnType: a nil type or an undefined/void base means "no
// return value" and yields the zero GoType (String() == ""). Any other type is
// mapped through MapType. (undefined in an argument position still maps to
// `any`; the void convention applies only to the return slot.)
func (m Mapper) returnGoType(t *webidl.IDLType) (GoType, error) {
	if t == nil || t.Base == "undefined" || t.Base == "void" {
		return GoType{}, nil
	}
	return m.MapType(t)
}
