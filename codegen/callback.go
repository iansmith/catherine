package codegen

import "fmt"

// callbackParam is one parameter in a callback function type.
// It carries only the type information; names are not emitted in Go func type literals.
type callbackParam struct {
	goType   string
	variadic bool
}

// NewCallbackFuncDecl creates a CallbackFuncDecl for a WebIDL callback function.
// idlName is the WebIDL callback name; params is a slice of (goType, variadic) pairs;
// returnType is "" for void; raisesException controls whether an error return is added.
// diag may be nil; a fresh Diagnostics is used in that case.
func NewCallbackFuncDecl(idlName string, params []callbackParam, returnType string, raisesException bool, diag *Diagnostics) *CallbackFuncDecl {
	if diag == nil {
		diag = NewDiagnostics()
	}
	if !hasAlnum(idlName) {
		diag.Add("error", fmt.Sprintf("callback name %q has no letter or digit content", idlName))
	}
	// Convert callbackParam to ifaceParam (goName is unused in func type literals).
	ifaceParams := make([]ifaceParam, len(params))
	for i, p := range params {
		ifaceParams[i] = ifaceParam{goType: p.goType, variadic: p.variadic}
	}
	return &CallbackFuncDecl{
		typeName:        IdentSanitize(idlName),
		params:          ifaceParams,
		returnType:      returnType,
		raisesException: raisesException,
	}
}
