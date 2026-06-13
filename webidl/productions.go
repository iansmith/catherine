package webidl

// ============================================================================
// Extended attributes
// ============================================================================

var extAttrValueKinds = []TokenKind{TokIdentifier, TokDecimal, TokInteger, TokString}

// parseExtAttrs parses `[ExtAttr, ExtAttr, ...]` if present; returns an
// empty slice if not.
func (p *parser) parseExtAttrs() []*ExtAttr {
	if p.consume("[") == nil {
		return nil
	}
	items := []*ExtAttr{}
	first := p.parseExtAttr()
	if first != nil {
		items = append(items, first)
		for p.consume(",") != nil {
			next := p.parseExtAttr()
			if next == nil {
				p.errorf("Trailing comma in extended attribute")
			}
			items = append(items, next)
		}
	}
	if p.consume("]") == nil {
		p.errorf("Expected a closing token for the extended attribute list")
	}
	if len(items) == 0 {
		p.errorf("An extended attribute list must not be empty")
	}
	if p.probe("[") {
		p.errorf("Illegal double extended attribute lists, consider merging them")
	}
	return items
}

func (p *parser) parseExtAttr() *ExtAttr {
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		return nil
	}
	ea := &ExtAttr{Name: name.Value}
	assign := p.consume("=")
	if assign != nil {
		if p.consume("*") != nil {
			ea.RHS = &ExtAttrRHS{Type: "*"}
			return ea
		}
		// Secondary name: identifier|decimal|integer|string
		if sec := p.consumeKind(extAttrValueKinds...); sec != nil {
			ea.RHS = &ExtAttrRHS{Type: sec.Kind.String(), Value: sec.Value}
			// fallthrough: still might have `(`
		}
	}
	if p.consume("(") != nil {
		if assign != nil && ea.RHS == nil {
			// `Name=(...)` — RHS is a list of identifiers/strings/integers/decimals
			items := p.parseExtAttrItems()
			ea.RHS = &ExtAttrRHS{
				Type:   items[0].Type + "-list",
				IsList: true,
				Items:  items,
			}
		} else {
			// `Name(arg list)` or `Name=Sec(arg list)`
			ea.Arguments = p.parseArgumentList()
		}
		if p.consume(")") == nil {
			p.errorf("Unexpected token in extended attribute argument list")
		}
	} else if assign != nil && ea.RHS == nil {
		p.errorf("No right hand side to extended attribute assignment")
	}
	return ea
}

// parseExtAttrItems parses a comma-separated list of identifier|string|integer|decimal.
// All items must be the same kind.
func (p *parser) parseExtAttrItems() []*ExtAttrItem {
	for _, k := range extAttrValueKinds {
		var items []*ExtAttrItem
		for {
			t := p.consumeKind(k)
			if t == nil {
				if len(items) == 0 {
					break
				}
				p.errorf("Trailing comma in %s list", k.String())
			}
			items = append(items, &ExtAttrItem{Type: k.String(), Value: t.Value})
			if p.consume(",") == nil {
				return items
			}
		}
	}
	p.errorf("Expected identifiers, strings, decimals, or integers but none found")
	return nil
}

// stringAndTypeNameKeywords is StringTypes + TypeNameKeywords pre-concatenated.
// Used by single-type parsing to look for either kind of type-name keyword.
var stringAndTypeNameKeywords = append(append([]string{}, StringTypes...), TypeNameKeywords...)

// ============================================================================
// Types
// ============================================================================

// parseTypeWithExtAttrs parses optional `[ExtAttrs]` then a type. The attrs are
// attached to the resulting type. Returns nil if no type was found.
func (p *parser) parseTypeWithExtAttrs(context string) *IDLType {
	mark := p.pos
	ea := p.parseExtAttrs()
	t := p.parseType(context)
	if t == nil {
		// rewind so the ext-attr-only parse doesn't consume
		if len(ea) > 0 {
			p.unconsume(mark)
		}
		return nil
	}
	if len(ea) > 0 {
		t.ExtAttrs = ea
	}
	return t
}

// parseType parses a single-type or union-type. Returns nil if neither matches.
func (p *parser) parseType(context string) *IDLType {
	if t := p.parseSingleType(context); t != nil {
		return t
	}
	return p.parseUnionType(context)
}

// parseSingleType: generic-type | primitive-type | identifier/string-type/typename-keyword, with optional `?`.
func (p *parser) parseSingleType(context string) *IDLType {
	t := p.parseGenericType(context)
	if t == nil {
		t = p.parsePrimitiveType()
	}
	if t == nil {
		var base *Token
		base = p.consumeKind(TokIdentifier)
		if base == nil {
			base = p.consume(stringAndTypeNameKeywords...)
		}
		if base == nil {
			return nil
		}
		if p.probe("<") {
			p.errorf("Unsupported generic type %s", base.Value)
		}
		t = &IDLType{Base: unescape(base.Value)}
	}
	if t.Generic == "Promise" && p.probe("?") {
		p.errorf("Promise type cannot be nullable")
	}
	t.Context = context
	p.consumeNullable(t)
	if t.Nullable && t.Base == "any" && t.Generic == "" && !t.Union {
		p.errorf("Type `any` cannot be made nullable")
	}
	return t
}

func (p *parser) consumeNullable(t *IDLType) {
	if p.consume("?") != nil {
		t.Nullable = true
	}
	if p.probe("?") {
		p.errorf("Can't nullable more than once")
	}
}

// parseGenericType: sequence<T>, FrozenArray<T>, ObservableArray<T>, Promise<T>, record<K,V>, async_sequence<T>.
func (p *parser) parseGenericType(context string) *IDLType {
	base := p.consume("FrozenArray", "ObservableArray", "Promise", "async_sequence", "sequence", "record")
	if base == nil {
		return nil
	}
	t := &IDLType{Generic: base.Value, Context: context}
	if p.consume("<") == nil {
		p.errorf("No opening bracket after %s", base.Value)
	}
	switch base.Value {
	case "Promise":
		if p.probe("[") {
			p.errorf("Promise type cannot have extended attribute")
		}
		sub := p.parseReturnTypeIn(context)
		if sub == nil {
			p.errorf("Missing Promise subtype")
		}
		t.Subtypes = []*IDLType{sub}
	case "async_sequence", "sequence", "FrozenArray", "ObservableArray":
		sub := p.parseTypeWithExtAttrs(context)
		if sub == nil {
			p.errorf("Missing %s subtype", base.Value)
		}
		t.Subtypes = []*IDLType{sub}
	case "record":
		if p.probe("[") {
			p.errorf("Record key cannot have extended attribute")
		}
		key := p.consume(StringTypes...)
		if key == nil {
			p.errorf("Record key must be one of: ByteString, DOMString, USVString")
		}
		keyType := &IDLType{Base: key.Value, Context: context}
		if p.consume(",") == nil {
			p.errorf("Missing comma after record key type")
		}
		valueType := p.parseTypeWithExtAttrs(context)
		if valueType == nil {
			p.errorf("Error parsing generic type record")
		}
		t.Subtypes = []*IDLType{keyType, valueType}
	}
	if p.consume(">") == nil {
		p.errorf("Missing closing bracket after %s", base.Value)
	}
	return t
}

// parsePrimitiveType: short, long, long long (signed/unsigned), float/double (restricted/unrestricted), bigint, boolean, byte, octet, undefined.
func (p *parser) parsePrimitiveType() *IDLType {
	// integer type
	pos := p.pos
	prefix := p.consume("unsigned")
	if base := p.consume("short", "long"); base != nil {
		name := base.Value
		if base.Value == "long" {
			if post := p.consume("long"); post != nil {
				name = "long long"
			}
		}
		if prefix != nil {
			name = "unsigned " + name
		}
		return &IDLType{Base: name}
	}
	if prefix != nil {
		p.errorf("Failed to parse integer type")
	}
	p.unconsume(pos)
	// decimal type
	prefix = p.consume("unrestricted")
	if base := p.consume("float", "double"); base != nil {
		name := base.Value
		if prefix != nil {
			name = "unrestricted " + name
		}
		return &IDLType{Base: name}
	}
	if prefix != nil {
		p.errorf("Failed to parse float type")
	}
	// other primitives
	if base := p.consume("bigint", "boolean", "byte", "octet", "undefined"); base != nil {
		return &IDLType{Base: base.Value}
	}
	return nil
}

// parseUnionType: `( T or T or T )` with optional nullable.
func (p *parser) parseUnionType(context string) *IDLType {
	if p.consume("(") == nil {
		return nil
	}
	t := &IDLType{Union: true, Context: context}
	for {
		sub := p.parseTypeWithExtAttrs(context)
		if sub == nil {
			p.errorf("No type after open parenthesis or 'or' in union type")
		}
		if sub.Base == "any" && sub.Generic == "" {
			p.errorf("Type `any` cannot be included in a union type")
		}
		if sub.Generic == "Promise" {
			p.errorf("Type `Promise` cannot be included in a union type")
		}
		t.Subtypes = append(t.Subtypes, sub)
		if p.consume("or") == nil {
			break
		}
	}
	if len(t.Subtypes) < 2 {
		p.errorf("At least two types are expected in a union type but found less")
	}
	if p.consume(")") == nil {
		p.errorf("Unterminated union type")
	}
	p.consumeNullable(t)
	return t
}

// parseReturnType is type-parse with the additional fallback that `void` is
// accepted (as a deprecated synonym). Uses "return-type" as the context.
func (p *parser) parseReturnType() *IDLType {
	return p.parseReturnTypeIn(CtxReturn)
}

// parseReturnTypeIn is like parseReturnType but lets the caller propagate
// the enclosing context (used so e.g. `attribute Promise<T>` records T with
// context "attribute-type" rather than "return-type"). The bare-`void`
// fallback is always recorded with context "return-type" regardless of the
// passed-in context — this matches webidl2.js, where the void Type node is
// constructed with a hard-coded `ret.type = "return-type"`.
func (p *parser) parseReturnTypeIn(context string) *IDLType {
	if context == "" {
		context = CtxReturn
	}
	if t := p.parseType(context); t != nil {
		return t
	}
	if v := p.consume("void"); v != nil {
		return &IDLType{Base: "void", Context: CtxReturn}
	}
	return nil
}

// ============================================================================
// Top-level definitions
// ============================================================================

func (p *parser) parseCallback() Definition {
	cb := p.consume("callback")
	if cb == nil {
		return nil
	}
	if p.probe("interface") {
		p.consume("interface")
		return p.parseContainerBody(IfaceCallback, false)
	}
	return p.parseCallbackFunctionRest()
}

func (p *parser) parseCallbackFunctionRest() *CallbackFunction {
	cf := &CallbackFunction{}
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		p.errorf("Callback lacks a name")
	}
	cf.Name = unescape(name.Value)
	if p.consume("=") == nil {
		p.errorf("Callback lacks an assignment")
	}
	rt := p.parseReturnType()
	if rt == nil {
		p.errorf("Callback lacks a return type")
	}
	cf.ReturnType = rt
	if p.consume("(") == nil {
		p.errorf("Callback lacks parentheses for arguments")
	}
	cf.Arguments = p.parseArgumentList()
	if p.consume(")") == nil {
		p.errorf("Unterminated callback")
	}
	if p.consume(";") == nil {
		p.errorf("Unterminated callback, expected `;`")
	}
	return cf
}

// parseInterfaceLike handles `interface Name {...}` or `interface mixin Name {...}`.
// `partial` is the consumed `partial` token if the caller saw one.
func (p *parser) parseInterfaceLike(partial *Token) Definition {
	base := p.consume("interface")
	if base == nil {
		return nil
	}
	if p.consume("mixin") != nil {
		return p.parseContainerBody(IfaceMixin, partial != nil)
	}
	return p.parseContainerBody(IfaceRegular, partial != nil)
}

// parseContainerBody implements the shared interface/mixin/callback-iface
// body: Name (`:` Inheritance)? `{` Members `}` `;`.
func (p *parser) parseContainerBody(v InterfaceVariant, partial bool) *Interface {
	iface := &Interface{Variant: v, Partial: partial}
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		p.errorf("Missing name in interface")
	}
	iface.Name = unescape(name.Value)
	inheritable := !partial && v != IfaceCallback && v != IfaceMixin
	if inheritable {
		if p.consume(":") != nil {
			inh := p.consumeKind(TokIdentifier)
			if inh == nil {
				p.errorf("Inheritance lacks a type")
			}
			iface.Inheritance = unescape(inh.Value)
		}
	}
	if p.consume("{") == nil {
		p.errorf("Bodyless interface")
	}
	for {
		if p.consume("}") != nil {
			if p.consume(";") == nil {
				p.errorf("Missing semicolon after interface")
			}
			return iface
		}
		firstTok := p.current()
		ea := p.parseExtAttrs()
		m := p.parseInterfaceMember(v)
		if m == nil {
			p.errorf("Unknown member")
		}
		attachMemberExtAttrs(m, ea)
		m.setSpan(spanFrom(firstTok))
		iface.Members = append(iface.Members, m)
	}
}

// parseInterfaceMember tries the production list appropriate for v.
func (p *parser) parseInterfaceMember(v InterfaceVariant) Member {
	switch v {
	case IfaceRegular:
		if m := p.parseConstant(); m != nil {
			return m
		}
		if m := p.parseConstructor(); m != nil {
			return m
		}
		if m := p.parseStaticMember(); m != nil {
			return m
		}
		if m := p.parseStringifier(); m != nil {
			return m
		}
		if m := p.parseIterableLike(); m != nil {
			return m
		}
		if m := p.parseAttribute(nil, false, false); m != nil {
			return m
		}
		if m := p.parseOperation(nil, false); m != nil {
			return m
		}
	case IfaceMixin:
		if m := p.parseConstant(); m != nil {
			return m
		}
		if m := p.parseStringifier(); m != nil {
			return m
		}
		if m := p.parseAttribute(nil, true, false); m != nil {
			return m
		}
		if m := p.parseOperation(nil, true); m != nil {
			return m
		}
	case IfaceCallback:
		if m := p.parseConstant(); m != nil {
			return m
		}
		if m := p.parseOperation(nil, true); m != nil {
			return m
		}
	}
	return nil
}

// parseStaticMember handles `static (Attribute | Operation)`.
func (p *parser) parseStaticMember() Member {
	t := p.consume("static")
	if t == nil {
		return nil
	}
	if a := p.parseAttribute(t, false, false); a != nil {
		return a
	}
	if o := p.parseOperation(t, false); o != nil {
		return o
	}
	p.errorf("No body in static member")
	return nil
}

// parseStringifier handles `stringifier (Attribute | Operation | ;)`.
func (p *parser) parseStringifier() Member {
	t := p.consume("stringifier")
	if t == nil {
		return nil
	}
	if a := p.parseAttribute(t, false, false); a != nil {
		return a
	}
	if o := p.parseOperation(t, false); o != nil {
		return o
	}
	p.errorf("Unterminated stringifier")
	return nil
}

// attachMemberExtAttrs assigns ext-attrs to a member, normalizing nil → [].
// All member types implement extAttrSetter (defined in ast.go).
func attachMemberExtAttrs(m Member, ea []*ExtAttr) {
	if ea == nil {
		ea = []*ExtAttr{}
	}
	m.(extAttrSetter).setExtAttrs(ea)
}

// parseDictionary handles `dictionary Name (: Parent)? { fields }` (and partial).
func (p *parser) parseDictionary(partial *Token) *Dictionary {
	if p.consume("dictionary") == nil {
		return nil
	}
	d := &Dictionary{Partial: partial != nil}
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		p.errorf("Missing name in dictionary")
	}
	d.Name = unescape(name.Value)
	if partial == nil {
		if p.consume(":") != nil {
			inh := p.consumeKind(TokIdentifier)
			if inh == nil {
				p.errorf("Inheritance lacks a type")
			}
			d.Inheritance = unescape(inh.Value)
		}
	}
	if p.consume("{") == nil {
		p.errorf("Bodyless dictionary")
	}
	for {
		if p.consume("}") != nil {
			if p.consume(";") == nil {
				p.errorf("Missing semicolon after dictionary")
			}
			return d
		}
		firstTok := p.current()
		ea := p.parseExtAttrs()
		f := p.parseField()
		if f == nil {
			p.errorf("Unknown member")
		}
		if ea == nil {
			ea = []*ExtAttr{}
		}
		f.ExtAttrs = ea
		f.Span = spanFrom(firstTok)
		d.Members = append(d.Members, f)
	}
}

// parseField parses one dictionary field.
func (p *parser) parseField() *Field {
	// Field-level ext attrs are handled by the caller; here we just match
	// `required? Type Name (= default)? ;`.
	required := p.consume("required")
	t := p.parseTypeWithExtAttrs(CtxDictionary)
	if t == nil {
		if required != nil {
			p.errorf("Dictionary member lacks a type")
		}
		return nil
	}
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		p.errorf("Dictionary member lacks a name")
	}
	f := &Field{Name: unescape(name.Value), IDLType: t, Required: required != nil}
	f.Default = p.parseDefault()
	if required != nil && f.Default != nil {
		p.errorf("Required member must not have a default")
	}
	if p.consume(";") == nil {
		p.errorf("Unterminated dictionary member, expected `;`")
	}
	return f
}

// parseEnum handles `enum Name { "v", "v", ... };`.
func (p *parser) parseEnum() *Enum {
	if p.consume("enum") == nil {
		return nil
	}
	e := &Enum{}
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		p.errorf("No name for enum")
	}
	e.Name = unescape(name.Value)
	if p.consume("{") == nil {
		p.errorf("Bodyless enum")
	}
	first := p.consumeKind(TokString)
	if first == nil && !p.probe("}") {
		p.errorf("No value in enum")
	}
	if first != nil {
		e.Values = append(e.Values, stripQuotes(first.Value))
		for p.consume(",") != nil {
			t := p.consumeKind(TokString)
			if t == nil {
				break // trailing comma allowed
			}
			e.Values = append(e.Values, stripQuotes(t.Value))
		}
	}
	if p.probeKind(TokString) {
		p.errorf("No comma between enum values")
	}
	if p.consume("}") == nil {
		p.errorf("Unexpected value in enum")
	}
	if len(e.Values) == 0 {
		p.errorf("No value in enum")
	}
	if p.consume(";") == nil {
		p.errorf("No semicolon after enum")
	}
	return e
}

func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// parseTypedef handles `typedef T Name;`.
func (p *parser) parseTypedef() *Typedef {
	if p.consume("typedef") == nil {
		return nil
	}
	td := &Typedef{}
	t := p.parseTypeWithExtAttrs(CtxTypedef)
	if t == nil {
		p.errorf("Typedef lacks a type")
	}
	td.IDLType = t
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		p.errorf("Typedef lacks a name")
	}
	td.Name = unescape(name.Value)
	if p.consume(";") == nil {
		p.errorf("Unterminated typedef, expected `;`")
	}
	return td
}

// parseIncludes handles `Target includes Mixin;`.  Speculatively consumes the
// leading identifier; rewinds if no `includes` keyword follows.
func (p *parser) parseIncludes() *Includes {
	mark := p.pos
	target := p.consumeKind(TokIdentifier)
	if target == nil {
		return nil
	}
	if p.consume("includes") == nil {
		p.unconsume(mark)
		return nil
	}
	mixin := p.consumeKind(TokIdentifier)
	if mixin == nil {
		p.errorf("Incomplete includes statement")
	}
	if p.consume(";") == nil {
		p.errorf("No terminating ; for includes statement")
	}
	return &Includes{Target: unescape(target.Value), Includes: unescape(mixin.Value)}
}

// parseNamespace handles `namespace Name { members } ;`.
func (p *parser) parseNamespace(partial *Token) *Namespace {
	if p.consume("namespace") == nil {
		return nil
	}
	ns := &Namespace{Partial: partial != nil}
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		p.errorf("Missing name in namespace")
	}
	ns.Name = unescape(name.Value)
	if p.consume("{") == nil {
		p.errorf("Bodyless namespace")
	}
	for {
		if p.consume("}") != nil {
			if p.consume(";") == nil {
				p.errorf("Missing semicolon after namespace")
			}
			return ns
		}
		firstTok := p.current()
		ea := p.parseExtAttrs()
		var m Member
		if x := p.parseAttribute(nil, true, true); x != nil {
			m = x
		} else if x := p.parseConstant(); x != nil {
			m = x
		} else if x := p.parseOperation(nil, true); x != nil {
			m = x
		} else {
			p.errorf("Unknown member")
		}
		attachMemberExtAttrs(m, ea)
		m.setSpan(spanFrom(firstTok))
		ns.Members = append(ns.Members, m)
	}
}

// ============================================================================
// Members
// ============================================================================

// parseAttribute: `[inherit] [readonly] attribute T name;`.
// `special` (if non-nil) is a pre-consumed `static`/`stringifier`/`inherit` token.
// noInherit=true skips trying to consume `inherit`.
// readonlyRequired=true errors if `attribute` is reached without `readonly`.
func (p *parser) parseAttribute(special *Token, noInherit, readonlyRequired bool) *Attribute {
	start := p.pos
	a := &Attribute{}
	if special != nil {
		a.Special = special.Value
	}
	if special == nil && !noInherit {
		if t := p.consume("inherit"); t != nil {
			a.Special = "inherit"
			if p.probe("readonly") {
				p.errorf("Inherited attributes cannot be read-only")
			}
		}
	}
	if t := p.consume("readonly"); t != nil {
		a.Readonly = true
	}
	if readonlyRequired && !a.Readonly && p.probe("attribute") {
		p.errorf("Attributes must be readonly in this context")
	}
	if p.consume("attribute") == nil {
		p.unconsume(start)
		return nil
	}
	t := p.parseTypeWithExtAttrs(CtxAttribute)
	if t == nil {
		p.errorf("Attribute lacks a type")
	}
	a.IDLType = t
	var name *Token
	if name = p.consumeKind(TokIdentifier); name == nil {
		name = p.consume("async", "required")
	}
	if name == nil {
		p.errorf("Attribute lacks a name")
	}
	a.Name = unescape(name.Value)
	if p.consume(";") == nil {
		p.errorf("Unterminated attribute, expected `;`")
	}
	return a
}

// parseOperation: getter/setter/deleter prefix (when !regular), return type,
// optional name, `(` args `)` `;`.  If special is "stringifier" the bare
// form `stringifier;` returns an Operation with no name/return type.
func (p *parser) parseOperation(special *Token, regular bool) *Operation {
	op := &Operation{}
	if special != nil {
		op.Special = special.Value
	}
	if special != nil && special.Value == "stringifier" {
		if p.consume(";") != nil {
			op.Arguments = []*Argument{}
			return op
		}
	}
	if special == nil && !regular {
		if t := p.consume("getter", "setter", "deleter"); t != nil {
			op.Special = t.Value
		}
	}
	rt := p.parseReturnType()
	if rt == nil {
		// no return type — only valid if we haven't committed to anything
		if special == nil && op.Special == "" {
			return nil
		}
		p.errorf("Missing return type")
	}
	op.ReturnType = rt
	if name := p.consumeKind(TokIdentifier); name != nil {
		op.Name = unescape(name.Value)
	} else if name := p.consume("includes"); name != nil {
		op.Name = name.Value
	}
	if p.consume("(") == nil {
		p.errorf("Invalid operation")
	}
	op.Arguments = p.parseArgumentList()
	if p.consume(")") == nil {
		p.errorf("Unterminated operation")
	}
	if p.consume(";") == nil {
		p.errorf("Unterminated operation, expected `;`")
	}
	return op
}

// parseConstant: `const Type NAME = value;`.
func (p *parser) parseConstant() *Constant {
	if p.consume("const") == nil {
		return nil
	}
	c := &Constant{}
	t := p.parsePrimitiveType()
	if t == nil {
		base := p.consumeKind(TokIdentifier)
		if base == nil {
			p.errorf("Const lacks a type")
		}
		t = &IDLType{Base: base.Value}
	}
	if p.probe("?") {
		p.errorf("Unexpected nullable constant type")
	}
	t.Context = CtxConst
	c.IDLType = t
	name := p.consumeKind(TokIdentifier)
	if name == nil {
		p.errorf("Const lacks a name")
	}
	c.Name = unescape(name.Value)
	if p.consume("=") == nil {
		p.errorf("Const lacks value assignment")
	}
	cv := p.parseConstValue()
	if cv == nil {
		p.errorf("Const lacks a value")
	}
	c.Value = cv
	if p.consume(";") == nil {
		p.errorf("Unterminated const, expected `;`")
	}
	return c
}

// parseConstValue: decimal | integer | true | false | Infinity | -Infinity | NaN.
func (p *parser) parseConstValue() *ConstValue {
	if t := p.consumeKind(TokDecimal, TokInteger); t != nil {
		return &ConstValue{Kind: CVNumber, Number: t.Value}
	}
	if t := p.consume("true", "false"); t != nil {
		return &ConstValue{Kind: CVBoolean, Bool: t.Value == "true"}
	}
	if t := p.consume("Infinity"); t != nil {
		return &ConstValue{Kind: CVInfinity, Negative: false}
	}
	if t := p.consume("-Infinity"); t != nil {
		return &ConstValue{Kind: CVInfinity, Negative: true}
	}
	if t := p.consume("NaN"); t != nil {
		return &ConstValue{Kind: CVNaN}
	}
	return nil
}

// parseDefault: `= <value>`.
// Returns nil if no `=` present.
func (p *parser) parseDefault() *ConstValue {
	if p.consume("=") == nil {
		return nil
	}
	if cv := p.parseConstValue(); cv != nil {
		return cv
	}
	if t := p.consumeKind(TokString); t != nil {
		return &ConstValue{Kind: CVString, String: stripQuotes(t.Value)}
	}
	if t := p.consume("null"); t != nil {
		return &ConstValue{Kind: CVNull}
	}
	if t := p.consume("["); t != nil {
		if p.consume("]") == nil {
			p.errorf("Default sequence value must be empty")
		}
		return &ConstValue{Kind: CVSequence}
	}
	if t := p.consume("{"); t != nil {
		if p.consume("}") == nil {
			p.errorf("Default dictionary value must be empty")
		}
		return &ConstValue{Kind: CVDictionary}
	}
	p.errorf("No value for default")
	return nil
}

// parseConstructor: `constructor(arglist);`.
func (p *parser) parseConstructor() *Constructor {
	if p.consume("constructor") == nil {
		return nil
	}
	c := &Constructor{}
	if p.consume("(") == nil {
		p.errorf("No argument list in constructor")
	}
	c.Arguments = p.parseArgumentList()
	if p.consume(")") == nil {
		p.errorf("Unterminated constructor")
	}
	if p.consume(";") == nil {
		p.errorf("No semicolon after constructor")
	}
	return c
}

// parseIterableLike handles iterable/async_iterable/maplike/setlike.
func (p *parser) parseIterableLike() *IterableLike {
	start := p.pos
	il := &IterableLike{}
	if t := p.consume("readonly"); t != nil {
		il.Readonly = true
	}
	if !il.Readonly {
		if t := p.consume("async"); t != nil {
			il.Async = true
		}
	}
	var base *Token
	if il.Readonly {
		base = p.consume("maplike", "setlike")
	} else if il.Async {
		base = p.consume("iterable")
	} else {
		base = p.consume("iterable", "async_iterable", "maplike", "setlike")
	}
	if base == nil {
		p.unconsume(start)
		return nil
	}
	switch base.Value {
	case "iterable":
		il.Kind = IterIterable
	case "async_iterable":
		il.Kind = IterAsyncIterable
	case "maplike":
		il.Kind = IterMaplike
	case "setlike":
		il.Kind = IterSetlike
	}
	secondTypeRequired := il.Kind == IterMaplike
	secondTypeAllowed := secondTypeRequired || il.Kind == IterIterable || il.Kind == IterAsyncIterable
	argumentAllowed := il.Kind == IterAsyncIterable || (il.Async && il.Kind == IterIterable)

	if p.consume("<") == nil {
		p.errorf("Missing less-than sign `<` in %s declaration", base.Value)
	}
	first := p.parseTypeWithExtAttrs("")
	if first == nil {
		p.errorf("Missing a type argument in %s declaration", base.Value)
	}
	il.Types = []*IDLType{first}
	if secondTypeAllowed {
		if p.consume(",") != nil {
			second := p.parseTypeWithExtAttrs("")
			if second == nil {
				p.errorf("Missing second type argument in %s declaration", base.Value)
			}
			il.Types = append(il.Types, second)
		} else if secondTypeRequired {
			p.errorf("Missing second type argument in %s declaration", base.Value)
		}
	}
	if p.consume(">") == nil {
		p.errorf("Missing greater-than sign `>` in %s declaration", base.Value)
	}
	il.Arguments = []*Argument{}
	if p.probe("(") {
		if !argumentAllowed {
			p.errorf("Arguments are only allowed for `async iterable`")
		}
		p.consume("(")
		il.Arguments = p.parseArgumentList()
		if p.consume(")") == nil {
			p.errorf("Unterminated async iterable argument list")
		}
	}
	if p.consume(";") == nil {
		p.errorf("Missing semicolon after %s declaration", base.Value)
	}
	return il
}

// ============================================================================
// Arguments
// ============================================================================

// parseArgumentList returns the (possibly empty) argument list inside `()`.
func (p *parser) parseArgumentList() []*Argument {
	first := p.parseArgument()
	if first == nil {
		return []*Argument{}
	}
	items := []*Argument{first}
	for p.consume(",") != nil {
		next := p.parseArgument()
		if next == nil {
			p.errorf("Trailing comma in arguments list")
		}
		items = append(items, next)
	}
	return items
}

// parseArgument parses one argument.
func (p *parser) parseArgument() *Argument {
	start := p.pos
	a := &Argument{}
	a.ExtAttrs = p.parseExtAttrs()
	if a.ExtAttrs == nil {
		a.ExtAttrs = []*ExtAttr{}
	}
	if p.consume("optional") != nil {
		a.Optional = true
	}
	t := p.parseTypeWithExtAttrs(CtxArgument)
	if t == nil {
		p.unconsume(start)
		return nil
	}
	a.IDLType = t
	if !a.Optional {
		if p.consume("...") != nil {
			a.Variadic = true
		}
	}
	var name *Token
	if name = p.consumeKind(TokIdentifier); name == nil {
		name = p.consume(ArgumentNameKeywords...)
	}
	if name == nil {
		p.errorf("Argument lacks a name")
	}
	a.Name = unescape(name.Value)
	if a.Optional {
		a.Default = p.parseDefault()
	}
	return a
}
