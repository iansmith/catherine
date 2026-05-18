package webidl

// Definition is any top-level IDL definition.
//
// Concrete types: *Interface, *Dictionary, *Enum, *Typedef, *Includes,
// *Namespace, *CallbackFunction. All definitions implement extAttrSetter so
// the parser can attach extended attributes generically.
type Definition interface {
	definitionNode()
	extAttrSetter
}

type extAttrSetter interface {
	setExtAttrs([]*ExtAttr)
}

func (*Interface) definitionNode()        {}
func (*Dictionary) definitionNode()       {}
func (*Enum) definitionNode()             {}
func (*Typedef) definitionNode()          {}
func (*Includes) definitionNode()         {}
func (*Namespace) definitionNode()        {}
func (*CallbackFunction) definitionNode() {}

func (x *Interface) setExtAttrs(ea []*ExtAttr)        { x.ExtAttrs = ea }
func (x *Dictionary) setExtAttrs(ea []*ExtAttr)       { x.ExtAttrs = ea }
func (x *Enum) setExtAttrs(ea []*ExtAttr)             { x.ExtAttrs = ea }
func (x *Typedef) setExtAttrs(ea []*ExtAttr)          { x.ExtAttrs = ea }
func (x *Includes) setExtAttrs(ea []*ExtAttr)         { x.ExtAttrs = ea }
func (x *Namespace) setExtAttrs(ea []*ExtAttr)        { x.ExtAttrs = ea }
func (x *CallbackFunction) setExtAttrs(ea []*ExtAttr) { x.ExtAttrs = ea }

// Interface covers interface, interface mixin, and callback interface.
// The Variant field distinguishes them.
type Interface struct {
	Variant     InterfaceVariant
	Name        string
	Inheritance string // "" if no `: Base`
	Members     []Member
	ExtAttrs    []*ExtAttr
	Partial     bool
}

type InterfaceVariant int

const (
	IfaceRegular InterfaceVariant = iota
	IfaceMixin
	IfaceCallback
)

// Dictionary is `dictionary Name { ... }`.
type Dictionary struct {
	Name        string
	Inheritance string
	Members     []*Field
	ExtAttrs    []*ExtAttr
	Partial     bool
}

// Enum is `enum Name { "v1", "v2", ... }`.
type Enum struct {
	Name     string
	Values   []string
	ExtAttrs []*ExtAttr
}

// Typedef is `typedef T Name;`.
type Typedef struct {
	Name     string
	IDLType  *IDLType
	ExtAttrs []*ExtAttr
}

// Includes is `Target includes Mixin;`.
type Includes struct {
	Target   string
	Includes string
	ExtAttrs []*ExtAttr
}

// Namespace is `namespace Name { ... }`.
type Namespace struct {
	Name     string
	Members  []Member
	ExtAttrs []*ExtAttr
	Partial  bool
}

// CallbackFunction is `callback Name = ReturnType (Args...);`.
type CallbackFunction struct {
	Name       string
	ReturnType *IDLType
	Arguments  []*Argument
	ExtAttrs   []*ExtAttr
}

// Member is anything that may appear inside an interface / namespace /
// callback-interface body.
type Member interface {
	memberNode()
}

// Member is any value that may appear inside an interface/namespace body.
// All members implement extAttrSetter.
func (*Attribute) memberNode()    {}
func (*Operation) memberNode()    {}
func (*Constant) memberNode()     {}
func (*Constructor) memberNode()  {}
func (*IterableLike) memberNode() {}

func (x *Attribute) setExtAttrs(ea []*ExtAttr)    { x.ExtAttrs = ea }
func (x *Operation) setExtAttrs(ea []*ExtAttr)    { x.ExtAttrs = ea }
func (x *Constant) setExtAttrs(ea []*ExtAttr)     { x.ExtAttrs = ea }
func (x *Constructor) setExtAttrs(ea []*ExtAttr)  { x.ExtAttrs = ea }
func (x *IterableLike) setExtAttrs(ea []*ExtAttr) { x.ExtAttrs = ea }

// Attribute is `[readonly|inherit|static] attribute T name;`.
type Attribute struct {
	Name     string
	IDLType  *IDLType
	ExtAttrs []*ExtAttr
	Special  string // "", "static", "stringifier", "inherit"
	Readonly bool
}

// Operation covers regular, static, getter/setter/deleter, and stringifier ops.
type Operation struct {
	Name       string   // "" for unnamed getter/setter/deleter or anonymous stringifier
	ReturnType *IDLType // nil for body-less stringifier (`stringifier;`)
	Arguments  []*Argument
	ExtAttrs   []*ExtAttr
	Special    string // "", "static", "stringifier", "getter", "setter", "deleter"
}

// Constant is `const T NAME = value;`.
type Constant struct {
	Name     string
	IDLType  *IDLType
	Value    *ConstValue
	ExtAttrs []*ExtAttr
}

// Constructor is `constructor(args);`.
type Constructor struct {
	Arguments []*Argument
	ExtAttrs  []*ExtAttr
}

// IterableLike covers iterable, async_iterable, maplike, and setlike.
type IterableLike struct {
	Kind      IterableKind
	Types     []*IDLType  // 1 entry (iterable/setlike) or 2 (maplike/value+key iterable)
	Arguments []*Argument // only for async iterable; [] otherwise
	ExtAttrs  []*ExtAttr
	Readonly  bool
	Async     bool
}

type IterableKind int

const (
	IterIterable IterableKind = iota
	IterAsyncIterable
	IterMaplike
	IterSetlike
)

func (k IterableKind) String() string {
	switch k {
	case IterIterable:
		return "iterable"
	case IterAsyncIterable:
		return "async_iterable"
	case IterMaplike:
		return "maplike"
	case IterSetlike:
		return "setlike"
	}
	return ""
}

// Field is a dictionary member: `required? T name (= default)?;`.
type Field struct {
	Name     string
	IDLType  *IDLType
	Default  *ConstValue
	ExtAttrs []*ExtAttr
	Required bool
}

// Argument is one item in an operation/callback argument list.
type Argument struct {
	Name     string
	IDLType  *IDLType
	Default  *ConstValue // non-nil only when Optional
	ExtAttrs []*ExtAttr
	Optional bool
	Variadic bool
}

// IDLType is a type expression. It can be a simple base ("long", "DOMString"),
// a parametric generic ("sequence<T>", "Promise<T>", "record<K,V>"),
// a union ("(A or B or C)"), or any of those marked nullable.
type IDLType struct {
	// Context is a tag describing where this type appears: one of the
	// `Ctx*` constants. Empty string means "no context" — the type is an
	// inner type of an iterable/maplike/setlike, where webidl2.js emits
	// `type: null`.
	Context  string
	Base     string     // single-type base name (e.g. "unsigned long", "DOMString"). "" for union or generic.
	Generic  string     // "" for non-generic; otherwise one of the generic keywords ("sequence", "FrozenArray", "Promise", "record", "async_sequence", "ObservableArray")
	Nullable bool
	Union    bool       // true iff subtypes form a `(A or B)` union (Base/Generic both empty)
	Subtypes []*IDLType // present for generic or union types
	ExtAttrs []*ExtAttr // type-level extended attributes (e.g. [Clamp] long)
}

// Type context tags. These mirror the `type` field webidl2.js writes onto
// each Type node depending on where in the grammar the type appeared.
const (
	CtxAttribute  = "attribute-type"
	CtxArgument   = "argument-type"
	CtxReturn     = "return-type"
	CtxDictionary = "dictionary-type"
	CtxTypedef    = "typedef-type"
	CtxConst      = "const-type"
)

// ExtAttr is one entry in `[ExtAttr1, ExtAttr2=Foo, ...]`.
type ExtAttr struct {
	Name      string
	RHS       *ExtAttrRHS // nil if no `=` part
	Arguments []*Argument // non-empty for `[Name(arg list)]`
}

// ExtAttrRHS represents the right-hand side of an extended attribute.
//
// Forms it can take:
//   - identifier value:           `Name=Foo`        Type="identifier",  Value="Foo"
//   - string value:               `Name="abc"`      Type="string",      Value="\"abc\""
//   - integer value:              `Name=0`          Type="integer",     Value="0"
//   - decimal value:              `Name=3.14`       Type="decimal",     Value="3.14"
//   - asterisk:                   `Name=*`          Type="*"            (no value)
//   - identifier list:            `Name=(a, b)`     Type="identifier-list", Items
//   - same for string/integer/decimal lists
type ExtAttrRHS struct {
	Type   string
	Value  string
	Items  []*ExtAttrItem
	IsList bool
}

type ExtAttrItem struct {
	Type  string // "identifier", "string", "integer", "decimal"
	Value string
}

// ConstValueKind enumerates the value shapes used for constant initializers
// and dictionary/argument default values. Mirrors webidl2.js's const_data.
type ConstValueKind int

const (
	CVString     ConstValueKind = iota // "string"
	CVNumber                           // "number" (integer or decimal as written)
	CVBoolean                          // "boolean"
	CVNull                             // "null"
	CVInfinity                         // "Infinity" (with Negative)
	CVNaN                              // "NaN"
	CVSequence                         // "sequence" (always [])
	CVDictionary                       // "dictionary"
)

// ConstValue is a constant initializer or default value.
type ConstValue struct {
	Kind     ConstValueKind
	String   string // for CVString — text WITHOUT surrounding quotes
	Number   string // for CVNumber — text as-written
	Bool     bool   // for CVBoolean
	Negative bool   // for CVInfinity
	// For any value where const_data falls through to `{type: value}`,
	// such as `null` or other bare identifiers used as defaults,
	// the Kind is set accordingly (CVNull) and no other field is used.
}
