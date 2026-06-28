package codegen

import (
	"fmt"

	"github.com/iansmith/webidl/webidl"
)

// ExtAttrSet holds codegen directives parsed from WebIDL extended attributes.
type ExtAttrSet struct {
	// ExposedScopes is nil when [Exposed] is absent (generate for all scopes).
	// Non-nil lists scope names: ["Window"], ["Window","Worker"], or ["*"].
	ExposedScopes []string

	SecureContext  bool
	RuntimeEnabled string // "" = absent; non-empty = feature name from [RuntimeEnabled=F]

	RaisesException bool
	Custom          bool
	PutForwards     string // "" = absent
	Replaceable     bool

	Clamp        bool
	EnforceRange bool
	AllowShared  bool

	NewObject  bool
	SameObject bool
	// ReflectPresent distinguishes absent (false) from [Reflect] with no name (true, ReflectAttr=="").
	ReflectPresent    bool
	ReflectAttr       string
	CEReactions       bool
	LegacyUnforgeable bool
	Unscopable        bool

	// UnknownAttrs contains names of unrecognized extended attributes (emitted as warnings).
	UnknownAttrs []string
}

// flagSetters maps each boolean (no-RHS) extended attribute to the field it
// sets. Value-bearing attributes (Exposed/RuntimeEnabled/PutForwards/Reflect)
// are handled explicitly in ParseExtAttrs; everything here is a plain flag, so
// table-driving them keeps ParseExtAttrs from being one giant switch.
var flagSetters = map[string]func(*ExtAttrSet){
	"SecureContext":     func(s *ExtAttrSet) { s.SecureContext = true },
	"RaisesException":   func(s *ExtAttrSet) { s.RaisesException = true },
	"Custom":            func(s *ExtAttrSet) { s.Custom = true },
	"Replaceable":       func(s *ExtAttrSet) { s.Replaceable = true },
	"Clamp":             func(s *ExtAttrSet) { s.Clamp = true },
	"EnforceRange":      func(s *ExtAttrSet) { s.EnforceRange = true },
	"AllowShared":       func(s *ExtAttrSet) { s.AllowShared = true },
	"NewObject":         func(s *ExtAttrSet) { s.NewObject = true },
	"SameObject":        func(s *ExtAttrSet) { s.SameObject = true },
	"CEReactions":       func(s *ExtAttrSet) { s.CEReactions = true },
	"LegacyUnforgeable": func(s *ExtAttrSet) { s.LegacyUnforgeable = true },
	// Unscopable (CATH-65): recognized so it does not surface as an unknown attr.
	// No binding behavior (a Symbol.unscopables concern).
	"Unscopable": func(s *ExtAttrSet) { s.Unscopable = true },
}

// ParseExtAttrs parses a slice of WebIDL extended attributes into codegen directives.
// Unknown names emit a warning into diag; they are never a fatal error.
// diag may be nil; a fresh Diagnostics is used in that case.
func ParseExtAttrs(attrs []*webidl.ExtAttr, diag *Diagnostics) ExtAttrSet {
	if diag == nil {
		diag = NewDiagnostics()
	}
	var s ExtAttrSet
	for _, a := range attrs {
		if set, ok := flagSetters[a.Name]; ok {
			set(&s)
			continue
		}
		switch a.Name {
		case "Exposed":
			if s.ExposedScopes != nil {
				diag.Add("warning", fmt.Sprintf("extended attribute %q appears more than once — last value wins", a.Name))
			}
			s.ExposedScopes = extAttrExposedScopes(a.RHS)
		case "RuntimeEnabled":
			if s.RuntimeEnabled != "" {
				diag.Add("warning", fmt.Sprintf("extended attribute %q appears more than once — last value wins", a.Name))
			}
			if v := identRHS(a, diag); v != "" {
				s.RuntimeEnabled = v
			}
		case "PutForwards":
			if s.PutForwards != "" {
				diag.Add("warning", fmt.Sprintf("extended attribute %q appears more than once — last value wins", a.Name))
			}
			if v := identRHS(a, diag); v != "" {
				s.PutForwards = v
			}
		case "Reflect":
			s.ReflectPresent = true
			// Mirror RuntimeEnabled/PutForwards: a no-RHS (or wrong-RHS) occurrence
			// leaves a previously-set name intact rather than clobbering it to "".
			if v := identRHS(a, diag); v != "" {
				s.ReflectAttr = v
			}
		default:
			s.UnknownAttrs = append(s.UnknownAttrs, a.Name)
			diag.Add("warning", fmt.Sprintf("unknown extended attribute %q — ignored", a.Name))
		}
	}
	return s
}

// identRHS returns an identifier-typed RHS value, warning on a non-identifier
// RHS and returning "" when there is no RHS.
func identRHS(a *webidl.ExtAttr, diag *Diagnostics) string {
	if a.RHS == nil {
		return ""
	}
	if a.RHS.Type == "identifier" {
		return a.RHS.Value
	}
	diag.Add("warning", fmt.Sprintf("extended attribute %q: expected identifier RHS, got %q — ignored", a.Name, a.RHS.Type))
	return ""
}

// extAttrExposedScopes extracts scope names from an [Exposed=...] RHS.
func extAttrExposedScopes(rhs *webidl.ExtAttrRHS) []string {
	if rhs == nil {
		return nil
	}
	switch rhs.Type {
	case "identifier":
		return []string{rhs.Value}
	case "*":
		return []string{"*"}
	case "identifier-list":
		out := make([]string, 0, len(rhs.Items))
		for _, item := range rhs.Items {
			if item.Type == "identifier" {
				out = append(out, item.Value)
			}
		}
		return out
	default:
		return nil
	}
}
