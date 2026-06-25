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
	ReflectPresent bool
	ReflectAttr    string
	CEReactions       bool
	LegacyUnforgeable bool

	// UnknownAttrs contains names of unrecognized extended attributes (emitted as warnings).
	UnknownAttrs []string
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
		switch a.Name {
		case "Exposed":
			if s.ExposedScopes != nil {
				diag.Add("warning", fmt.Sprintf("extended attribute %q appears more than once — last value wins", a.Name))
			}
			s.ExposedScopes = extAttrExposedScopes(a.RHS)
		case "SecureContext":
			s.SecureContext = true
		case "RuntimeEnabled":
			if s.RuntimeEnabled != "" {
				diag.Add("warning", fmt.Sprintf("extended attribute %q appears more than once — last value wins", a.Name))
			}
			if a.RHS != nil && a.RHS.Type == "identifier" {
				s.RuntimeEnabled = a.RHS.Value
			} else if a.RHS != nil {
				diag.Add("warning", fmt.Sprintf("extended attribute %q: expected identifier RHS, got %q — ignored", a.Name, a.RHS.Type))
			}
		case "RaisesException":
			s.RaisesException = true
		case "Custom":
			s.Custom = true
		case "PutForwards":
			if s.PutForwards != "" {
				diag.Add("warning", fmt.Sprintf("extended attribute %q appears more than once — last value wins", a.Name))
			}
			if a.RHS != nil && a.RHS.Type == "identifier" {
				s.PutForwards = a.RHS.Value
			} else if a.RHS != nil {
				diag.Add("warning", fmt.Sprintf("extended attribute %q: expected identifier RHS, got %q — ignored", a.Name, a.RHS.Type))
			}
		case "Replaceable":
			s.Replaceable = true
		case "Clamp":
			s.Clamp = true
		case "EnforceRange":
			s.EnforceRange = true
		case "AllowShared":
			s.AllowShared = true
		case "NewObject":
			s.NewObject = true
		case "SameObject":
			s.SameObject = true
		case "Reflect":
			s.ReflectPresent = true
			if a.RHS != nil && a.RHS.Type == "identifier" {
				s.ReflectAttr = a.RHS.Value
			} else if a.RHS != nil {
				diag.Add("warning", fmt.Sprintf("extended attribute %q: expected identifier RHS, got %q — ignored", a.Name, a.RHS.Type))
			}
		case "CEReactions":
			s.CEReactions = true
		case "LegacyUnforgeable":
			s.LegacyUnforgeable = true
		default:
			s.UnknownAttrs = append(s.UnknownAttrs, a.Name)
			diag.Add("warning", fmt.Sprintf("unknown extended attribute %q — ignored", a.Name))
		}
	}
	return s
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
