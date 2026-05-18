package webidl

// ToJSONShape converts a list of definitions to the webidl2.js-compatible JSON
// shape. Each element is a map[string]any structurally equivalent to the JSON
// emitted by `JSON.stringify(parse(text))` in webidl2.js (without an EOF
// sentinel — callers should strip the trailing `{type:"eof",value:""}` from
// webidl2 baselines before comparing).
func ToJSONShape(defs []Definition) []any {
	out := make([]any, 0, len(defs))
	for _, d := range defs {
		out = append(out, defToJSON(d))
	}
	return out
}

func defToJSON(d Definition) any {
	switch x := d.(type) {
	case *Interface:
		t := "interface"
		switch x.Variant {
		case IfaceMixin:
			t = "interface mixin"
		case IfaceCallback:
			t = "callback interface"
		}
		return map[string]any{
			"type":        t,
			"name":        x.Name,
			"inheritance": stringOrNull(x.Inheritance),
			"members":     membersToJSON(x.Members),
			"extAttrs":    extAttrsToJSON(x.ExtAttrs),
			"partial":     x.Partial,
		}
	case *Dictionary:
		return map[string]any{
			"type":        "dictionary",
			"name":        x.Name,
			"inheritance": stringOrNull(x.Inheritance),
			"members":     fieldsToJSON(x.Members),
			"extAttrs":    extAttrsToJSON(x.ExtAttrs),
			"partial":     x.Partial,
		}
	case *Enum:
		values := make([]any, 0, len(x.Values))
		for _, v := range x.Values {
			values = append(values, map[string]any{
				"type":  "enum-value",
				"value": v,
			})
		}
		return map[string]any{
			"type":     "enum",
			"name":     x.Name,
			"values":   values,
			"extAttrs": extAttrsToJSON(x.ExtAttrs),
		}
	case *Typedef:
		return map[string]any{
			"type":     "typedef",
			"name":     x.Name,
			"idlType":  typeToJSON(x.IDLType),
			"extAttrs": extAttrsToJSON(x.ExtAttrs),
		}
	case *Includes:
		return map[string]any{
			"type":     "includes",
			"target":   x.Target,
			"includes": x.Includes,
			"extAttrs": extAttrsToJSON(x.ExtAttrs),
		}
	case *Namespace:
		return map[string]any{
			"type":        "namespace",
			"name":        x.Name,
			"inheritance": nil,
			"members":     membersToJSON(x.Members),
			"extAttrs":    extAttrsToJSON(x.ExtAttrs),
			"partial":     x.Partial,
		}
	case *CallbackFunction:
		return map[string]any{
			"type":      "callback",
			"name":      x.Name,
			"idlType":   typeToJSON(x.ReturnType),
			"arguments": argsToJSON(x.Arguments),
			"extAttrs":  extAttrsToJSON(x.ExtAttrs),
		}
	}
	return nil
}

// stringOrNull returns the string, or JSON null for empty.
func stringOrNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func membersToJSON(ms []Member) []any {
	out := make([]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, memberToJSON(m))
	}
	return out
}

func memberToJSON(m Member) any {
	switch x := m.(type) {
	case *Attribute:
		return map[string]any{
			"type":     "attribute",
			"name":     x.Name,
			"idlType":  typeToJSON(x.IDLType),
			"extAttrs": extAttrsToJSON(x.ExtAttrs),
			"special":  x.Special,
			"readonly": x.Readonly,
		}
	case *Operation:
		// Bodyless stringifier: omit idlType (and name has natural "").
		out := map[string]any{
			"type":      "operation",
			"name":      x.Name,
			"arguments": argsToJSON(x.Arguments),
			"extAttrs":  extAttrsToJSON(x.ExtAttrs),
			"special":   x.Special,
		}
		if x.ReturnType != nil {
			out["idlType"] = typeToJSON(x.ReturnType)
		}
		return out
	case *Constant:
		return map[string]any{
			"type":     "const",
			"name":     x.Name,
			"idlType":  typeToJSON(x.IDLType),
			"extAttrs": extAttrsToJSON(x.ExtAttrs),
			"value":    constValueToJSON(x.Value),
		}
	case *Constructor:
		return map[string]any{
			"type":      "constructor",
			"arguments": argsToJSON(x.Arguments),
			"extAttrs":  extAttrsToJSON(x.ExtAttrs),
		}
	case *IterableLike:
		out := map[string]any{
			"type":      x.Kind.String(),
			"idlType":   typesToJSONArray(x.Types),
			"arguments": argsToJSON(x.Arguments),
			"extAttrs":  extAttrsToJSON(x.ExtAttrs),
			"readonly":  x.Readonly,
			"async":     x.Async,
		}
		return out
	}
	return nil
}

func fieldsToJSON(fs []*Field) []any {
	out := make([]any, 0, len(fs))
	for _, f := range fs {
		entry := map[string]any{
			"type":     "field",
			"name":     f.Name,
			"extAttrs": extAttrsToJSON(f.ExtAttrs),
			"idlType":  typeToJSON(f.IDLType),
			"required": f.Required,
		}
		if f.Default != nil {
			entry["default"] = constValueToJSON(f.Default)
		} else {
			entry["default"] = nil
		}
		out = append(out, entry)
	}
	return out
}

func argsToJSON(args []*Argument) []any {
	out := make([]any, 0, len(args))
	for _, a := range args {
		entry := map[string]any{
			"type":     "argument",
			"name":     a.Name,
			"extAttrs": extAttrsToJSON(a.ExtAttrs),
			"idlType":  typeToJSON(a.IDLType),
			"optional": a.Optional,
			"variadic": a.Variadic,
		}
		if a.Default != nil {
			entry["default"] = constValueToJSON(a.Default)
		} else {
			entry["default"] = nil
		}
		out = append(out, entry)
	}
	return out
}

func typeToJSON(t *IDLType) any {
	if t == nil {
		return nil
	}
	out := map[string]any{
		"extAttrs": extAttrsToJSON(t.ExtAttrs),
		"generic":  t.Generic,
		"nullable": t.Nullable,
		"union":    t.Union,
	}
	if t.Context != "" {
		out["type"] = t.Context
	} else {
		out["type"] = nil
	}
	if len(t.Subtypes) > 0 {
		out["idlType"] = typesToJSONArray(t.Subtypes)
	} else {
		out["idlType"] = t.Base
	}
	return out
}

func typesToJSONArray(ts []*IDLType) []any {
	out := make([]any, 0, len(ts))
	for _, t := range ts {
		out = append(out, typeToJSON(t))
	}
	return out
}

func extAttrsToJSON(eas []*ExtAttr) []any {
	out := make([]any, 0, len(eas))
	for _, ea := range eas {
		entry := map[string]any{
			"type":      "extended-attribute",
			"name":      ea.Name,
			"rhs":       extAttrRHSToJSON(ea.RHS),
			"arguments": argsToJSON(ea.Arguments),
		}
		out = append(out, entry)
	}
	return out
}

func extAttrRHSToJSON(r *ExtAttrRHS) any {
	if r == nil {
		return nil
	}
	if r.Type == "*" {
		return map[string]any{"type": "*", "value": nil}
	}
	if r.IsList {
		items := make([]any, 0, len(r.Items))
		for _, it := range r.Items {
			val := it.Value
			if it.Type == "identifier" {
				val = unescape(val)
			}
			items = append(items, map[string]any{
				"type":  it.Type,
				"value": val,
			})
		}
		return map[string]any{
			"type":  r.Type,
			"value": items,
		}
	}
	value := r.Value
	if r.Type == "identifier" {
		// webidl2 unescapes the leading `_` on rhs identifier values.
		value = unescape(value)
	}
	return map[string]any{
		"type":  r.Type,
		"value": value,
	}
}

func constValueToJSON(v *ConstValue) any {
	if v == nil {
		return nil
	}
	switch v.Kind {
	case CVString:
		return map[string]any{"type": "string", "value": v.String}
	case CVNumber:
		return map[string]any{"type": "number", "value": v.Number}
	case CVBoolean:
		return map[string]any{"type": "boolean", "value": v.Bool}
	case CVNull:
		return map[string]any{"type": "null"}
	case CVInfinity:
		return map[string]any{"type": "Infinity", "negative": v.Negative}
	case CVNaN:
		return map[string]any{"type": "NaN"}
	case CVSequence:
		return map[string]any{"type": "sequence", "value": []any{}}
	case CVDictionary:
		return map[string]any{"type": "dictionary"}
	}
	return nil
}

