package codegen_test

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
	"github.com/iansmith/webidl/typemap"
	"github.com/iansmith/webidl/webidl"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func idlType(base string) *webidl.IDLType {
	return &webidl.IDLType{Base: base, Context: webidl.CtxReturn}
}

func idlArgType(base string) *webidl.IDLType {
	return &webidl.IDLType{Base: base, Context: webidl.CtxArgument}
}

func idlAttrType(base string) *webidl.IDLType {
	return &webidl.IDLType{Base: base, Context: webidl.CtxAttribute}
}

func idlConstType(base string) *webidl.IDLType {
	return &webidl.IDLType{Base: base, Context: webidl.CtxConst}
}

func regularMergedDef(name, inheritance string, members ...webidl.Member) *webidl.MergedDef {
	iface := &webidl.Interface{
		Variant:     webidl.IfaceRegular,
		Name:        name,
		Inheritance: inheritance,
		Members:     membersSlice(members...),
	}
	return &webidl.MergedDef{Primary: iface, Members: membersSlice(members...)}
}

func callbackMergedDef(name string, members ...webidl.Member) *webidl.MergedDef {
	iface := &webidl.Interface{
		Variant: webidl.IfaceCallback,
		Name:    name,
		Members: membersSlice(members...),
	}
	return &webidl.MergedDef{Primary: iface, Members: membersSlice(members...)}
}

func mixinMergedDef(name string) *webidl.MergedDef {
	iface := &webidl.Interface{Variant: webidl.IfaceMixin, Name: name}
	return &webidl.MergedDef{Primary: iface}
}

func membersSlice(ms ...webidl.Member) []webidl.Member {
	return ms
}

func attr(name string, readonly bool, t *webidl.IDLType) *webidl.Attribute {
	return &webidl.Attribute{Name: name, IDLType: t, Readonly: readonly}
}

func staticAttr(name string, readonly bool, t *webidl.IDLType) *webidl.Attribute {
	return &webidl.Attribute{Name: name, IDLType: t, Readonly: readonly, Special: "static"}
}

func op(name string, ret *webidl.IDLType, args ...*webidl.Argument) *webidl.Operation {
	return &webidl.Operation{Name: name, ReturnType: ret, Arguments: args}
}

func staticOp(name string, ret *webidl.IDLType, args ...*webidl.Argument) *webidl.Operation {
	return &webidl.Operation{Name: name, ReturnType: ret, Arguments: args, Special: "static"}
}

func specialOp(special string, ret *webidl.IDLType, args ...*webidl.Argument) *webidl.Operation {
	return &webidl.Operation{ReturnType: ret, Special: special, Arguments: args}
}

func arg(name, base string) *webidl.Argument {
	return &webidl.Argument{Name: name, IDLType: idlArgType(base)}
}

func constMember(name string, t *webidl.IDLType, num string) *webidl.Constant {
	return &webidl.Constant{
		Name:    name,
		IDLType: t,
		Value:   &webidl.ConstValue{Kind: webidl.CVNumber, Number: num},
	}
}

func iterable(valBase string) *webidl.IterableLike {
	return &webidl.IterableLike{
		Kind:  webidl.IterIterable,
		Types: []*webidl.IDLType{idlType(valBase)},
	}
}

func pairIterable(keyBase, valBase string) *webidl.IterableLike {
	return &webidl.IterableLike{
		Kind:  webidl.IterIterable,
		Types: []*webidl.IDLType{idlType(keyBase), idlType(valBase)},
	}
}

func asyncIterable(valBase string) *webidl.IterableLike {
	return &webidl.IterableLike{
		Kind:  webidl.IterAsyncIterable,
		Types: []*webidl.IDLType{idlType(valBase)},
		Async: true,
	}
}

func asyncPairIterable(keyBase, valBase string) *webidl.IterableLike {
	return &webidl.IterableLike{
		Kind:  webidl.IterAsyncIterable,
		Types: []*webidl.IDLType{idlType(keyBase), idlType(valBase)},
		Async: true,
	}
}

func maplike(keyBase, valBase string, readonly bool) *webidl.IterableLike {
	return &webidl.IterableLike{
		Kind:     webidl.IterMaplike,
		Types:    []*webidl.IDLType{idlType(keyBase), idlType(valBase)},
		Readonly: readonly,
	}
}

func setlike(valBase string, readonly bool) *webidl.IterableLike {
	return &webidl.IterableLike{
		Kind:     webidl.IterSetlike,
		Types:    []*webidl.IDLType{idlType(valBase)},
		Readonly: readonly,
	}
}

var tm = typemap.Mapper{}

// firstDecl returns the first Decl from the result of NewInterfaceDecls.
// t.Fatal if result is empty.
func firstDecl(t *testing.T, decls []codegen.Decl) codegen.Decl {
	t.Helper()
	if len(decls) == 0 {
		t.Fatal("NewInterfaceDecls returned empty slice")
	}
	return decls[0]
}

// sourceOf renders the first Decl (usually InterfaceDecl) into a File and
// returns the source string. The file uses package "gen". Import paths needed
// by the source must be supplied in imports.
func sourceOf(t *testing.T, d codegen.Decl, imports ...string) string {
	t.Helper()
	f := codegen.NewFile("gen")
	if len(imports) > 0 {
		tr := codegen.NewImportTracker()
		for _, imp := range imports {
			tr.Add(imp)
		}
		f.SetImports(tr)
	}
	f.AddDecl(d)
	out, err := f.Render()
	if err != nil {
		t.Fatalf("File.Render() error: %v", err)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// nil / mixin
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_NilMergedDef(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.NewInterfaceDecls(nil, tm, diag)
	if got != nil {
		t.Errorf("NewInterfaceDecls(nil) = %v; want nil", got)
	}
}

func TestNewInterfaceDecls_MixinSkipped(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.NewInterfaceDecls(mixinMergedDef("EventHandlerMixin"), tm, diag)
	if got != nil {
		t.Errorf("IfaceMixin: NewInterfaceDecls returned %v; want nil", got)
	}
}

// ---------------------------------------------------------------------------
// Regular interface — empty
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_EmptyInterface(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(regularMergedDef("EventTarget", ""), tm, diag)
	if len(decls) != 1 {
		t.Fatalf("empty interface: expected 1 decl, got %d", len(decls))
	}
	src := sourceOf(t, decls[0])
	if !strings.Contains(src, "type EventTarget interface") {
		t.Errorf("empty interface source missing type declaration:\n%s", src)
	}
	if !diag.IsClean() {
		t.Errorf("empty interface produced diagnostics: %s", diag.Format())
	}
}

// ---------------------------------------------------------------------------
// Attributes
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_ReadonlyAttribute(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Node", "", attr("nodeType", true, idlAttrType("unsigned short"))),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "NodeTypeAttr() uint16") {
		t.Errorf("readonly attr: expected 'NodeTypeAttr() uint16' in source:\n%s", src)
	}
	if strings.Contains(src, "SetNodeTypeAttr") {
		t.Errorf("readonly attr: setter must not be present:\n%s", src)
	}
}

func TestNewInterfaceDecls_WritableAttribute(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Element", "", attr("className", false, idlAttrType("DOMString"))),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "ClassNameAttr() string") {
		t.Errorf("writable attr: expected getter 'ClassNameAttr() string':\n%s", src)
	}
	if !strings.Contains(src, "SetClassNameAttr(V string)") {
		t.Errorf("writable attr: expected setter 'SetClassNameAttr(V string)':\n%s", src)
	}
}

func TestNewInterfaceDecls_AttributeCollision(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	// Two attributes that sanitize to the same getter name: "my-attr" and "myAttr" → "MyAttrAttr"
	members := []webidl.Member{
		attr("my-attr", true, idlAttrType("long")),
		attr("myAttr", true, idlAttrType("long")),
	}
	def := regularMergedDef("Foo", "", members...)
	codegen.NewInterfaceDecls(def, tm, diag)
	if diag.IsClean() {
		t.Error("attribute collision: expected error diagnostic, got none")
	}
	errs := diag.Errors()
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "collision") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("attribute collision: no 'collision' in diagnostics: %s", diag.Format())
	}
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_RegularOperation(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("EventTarget", "",
			op("addEventListener", idlType("undefined"), arg("type", "DOMString"), arg("callback", "any")),
		),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "AddEventListener(") {
		t.Errorf("regular op: expected 'AddEventListener(' in source:\n%s", src)
	}
}

func TestNewInterfaceDecls_VoidOperation(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Foo", "", op("doThing", idlType("undefined"))),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	// void op: no return type on the same line as method name
	if !strings.Contains(src, "DoThing()") {
		t.Errorf("void op: expected 'DoThing()' (no return type):\n%s", src)
	}
}

func TestNewInterfaceDecls_OperationWithReturnType(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Foo", "",
			op("getLength", idlType("unsigned long")),
		),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "GetLength() uint32") {
		t.Errorf("op with return: expected 'GetLength() uint32':\n%s", src)
	}
}

func TestNewInterfaceDecls_OperationCollision(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	members := []webidl.Member{
		op("doThing", idlType("undefined")),
		op("doThing", idlType("long")), // same name → collision
	}
	codegen.NewInterfaceDecls(regularMergedDef("Foo", "", members...), tm, diag)
	if diag.IsClean() {
		t.Error("op collision: expected error, got none")
	}
	found := false
	for _, e := range diag.Errors() {
		if strings.Contains(e.Message, "collision") || strings.Contains(e.Message, "dropped") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("op collision: no collision/dropped error in diagnostics: %s", diag.Format())
	}
}

// ---------------------------------------------------------------------------
// Stringifier
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_StringifierOperation(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("URL", "",
			&webidl.Operation{Special: "stringifier", ReturnType: idlType("DOMString")},
		),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "String() string") {
		t.Errorf("stringifier: expected 'String() string' in interface:\n%s", src)
	}
}

// ---------------------------------------------------------------------------
// Special indexed operations
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_SpecialGetter(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("NodeList", "",
			specialOp("getter", idlType("any"), arg("index", "unsigned long")),
		),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "Index(I uint32)") {
		t.Errorf("getter special op: expected 'Index(I uint32)':\n%s", src)
	}
}

func TestNewInterfaceDecls_SpecialSetter(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Foo", "",
			specialOp("setter", idlType("undefined"), arg("index", "unsigned long"), arg("value", "DOMString")),
		),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "SetIndex(I uint32, V string)") {
		t.Errorf("setter special op: expected 'SetIndex(I uint32, V string)':\n%s", src)
	}
}

func TestNewInterfaceDecls_SpecialDeleter(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Foo", "",
			specialOp("deleter", idlType("undefined"), arg("index", "unsigned long")),
		),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "Delete(I uint32)") {
		t.Errorf("deleter special op: expected 'Delete(I uint32)':\n%s", src)
	}
}

// ---------------------------------------------------------------------------
// Inheritance
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_Inheritance(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("HTMLElement", "Element"),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls))
	if !strings.Contains(src, "type HTMLElement interface") {
		t.Errorf("inheritance: expected 'type HTMLElement interface':\n%s", src)
	}
	if !strings.Contains(src, "\tElement\n") {
		t.Errorf("inheritance: expected '\\tElement\\n' embedded in interface body:\n%s", src)
	}
}

// ---------------------------------------------------------------------------
// Callback interfaces
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_CallbackSingleMethod(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		callbackMergedDef("EventListener",
			op("handleEvent", idlType("undefined"), arg("event", "any")),
		),
		tm, diag,
	)
	if len(decls) != 1 {
		t.Fatalf("callback single-method: expected 1 decl, got %d", len(decls))
	}
	src := sourceOf(t, decls[0])
	// Should produce a func type, not an interface
	if !strings.Contains(src, "type EventListenerFunc func(") {
		t.Errorf("callback single-method: expected 'type EventListenerFunc func(' in:\n%s", src)
	}
	if strings.Contains(src, "interface") {
		t.Errorf("callback single-method: must not produce an interface:\n%s", src)
	}
}

func TestNewInterfaceDecls_CallbackMultiMethod(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		callbackMergedDef("Observer",
			op("observe", idlType("undefined"), arg("entry", "any")),
			op("disconnect", idlType("undefined")),
		),
		tm, diag,
	)
	if len(decls) != 1 {
		t.Fatalf("callback multi-method: expected 1 decl, got %d", len(decls))
	}
	src := sourceOf(t, decls[0])
	if !strings.Contains(src, "type Observer interface") {
		t.Errorf("callback multi-method: expected interface, got:\n%s", src)
	}
}

func TestNewInterfaceDecls_CallbackZeroMethod(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.NewInterfaceDecls(callbackMergedDef("Empty"), tm, diag)
	if got != nil {
		t.Errorf("callback zero-method: expected nil, got %v", got)
	}
	if diag.IsClean() {
		t.Error("callback zero-method: expected error diagnostic, got none")
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_Constants(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Node", "",
			constMember("ELEMENT_NODE", idlConstType("unsigned short"), "1"),
			constMember("TEXT_NODE", idlConstType("unsigned short"), "3"),
		),
		tm, diag,
	)
	// Expect InterfaceDecl + ConstBlockDecl
	if len(decls) < 2 {
		t.Fatalf("constants: expected ≥2 decls (interface + const block), got %d", len(decls))
	}
	var constSrc string
	for _, d := range decls[1:] {
		s := sourceOf(t, d)
		if strings.Contains(s, "const") {
			constSrc = s
			break
		}
	}
	if constSrc == "" {
		t.Fatal("constants: no ConstBlockDecl found in result decls")
	}
	if !strings.Contains(constSrc, "NodeELEMENTNODE") {
		t.Errorf("constants: expected 'NodeELEMENTNODE' in const block:\n%s", constSrc)
	}
	if !strings.Contains(constSrc, "NodeTEXTNODE") {
		t.Errorf("constants: expected 'NodeTEXTNODE' in const block:\n%s", constSrc)
	}
}

func TestNewInterfaceDecls_ConstantCollision(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	codegen.NewInterfaceDecls(
		regularMergedDef("Foo", "",
			constMember("VALUE", idlConstType("unsigned short"), "1"),
			constMember("VALUE", idlConstType("unsigned short"), "2"), // collision
		),
		tm, diag,
	)
	if diag.IsClean() {
		t.Error("constant collision: expected error diagnostic, got none")
	}
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_Constructor(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	ctor := &webidl.Constructor{
		Arguments: []*webidl.Argument{arg("nodeType", "unsigned short")},
	}
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Node", "", ctor),
		tm, diag,
	)
	if len(decls) < 2 {
		t.Fatalf("constructor: expected ≥2 decls, got %d", len(decls))
	}
	var ctorSrc string
	for _, d := range decls {
		s := sourceOf(t, d)
		if strings.Contains(s, "func NewNode(") {
			ctorSrc = s
			break
		}
	}
	if ctorSrc == "" {
		t.Fatalf("constructor: no ConstructorDecl (NewNode) found in:\n%v", decls)
	}
	if !strings.Contains(ctorSrc, ") Node ") {
		t.Errorf("constructor: expected return type 'Node' in:\n%s", ctorSrc)
	}
}

func TestNewInterfaceDecls_ConstructorOverloadFirstWins(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	ctor1 := &webidl.Constructor{Arguments: []*webidl.Argument{arg("x", "long")}}
	ctor2 := &webidl.Constructor{Arguments: []*webidl.Argument{arg("x", "long"), arg("y", "long")}}
	codegen.NewInterfaceDecls(regularMergedDef("Foo", "", ctor1, ctor2), tm, diag)
	if diag.IsClean() {
		t.Error("constructor overload: expected error diagnostic about dropped overload, got none")
	}
}

// ---------------------------------------------------------------------------
// Static operations
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_StaticOperation(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("URL", "",
			staticOp("createObjectURL", idlType("DOMString"), arg("obj", "any")),
		),
		tm, diag,
	)
	var staticSrc string
	for _, d := range decls {
		s := sourceOf(t, d)
		if strings.Contains(s, "func URLCreateObjectURL(") {
			staticSrc = s
			break
		}
	}
	if staticSrc == "" {
		t.Fatalf("static op: expected func URLCreateObjectURL in decls, got:\n%v", decls)
	}
	if !strings.Contains(staticSrc, ") string ") {
		t.Errorf("static op: expected string return type in:\n%s", staticSrc)
	}
}

func TestNewInterfaceDecls_StaticAttribute(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Document", "",
			staticAttr("activeElement", true, idlAttrType("any")),
		),
		tm, diag,
	)
	// Should produce a static getter func
	var found bool
	for _, d := range decls {
		s := sourceOf(t, d)
		if strings.Contains(s, "func DocumentGetActiveElement()") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("static attr: expected 'func DocumentGetActiveElement()' in decls")
	}
}

// ---------------------------------------------------------------------------
// Iterable / collection members
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_Iterable(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("NodeList", "", iterable("any")),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls), "iter")
	for _, want := range []string{"Values()", "Keys()", "Entries()", "ForEach("} {
		if !strings.Contains(src, want) {
			t.Errorf("iterable: expected %q in source:\n%s", want, src)
		}
	}
	if !strings.Contains(src, "iter.Seq") {
		t.Errorf("iterable: expected 'iter.Seq' in source:\n%s", src)
	}
}

func TestNewInterfaceDecls_PairIterable(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("FormData", "", pairIterable("DOMString", "any")),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls), "iter")
	if !strings.Contains(src, "iter.Seq2[string, any]") {
		t.Errorf("pair iterable: expected 'iter.Seq2[string, any]' in:\n%s", src)
	}
}

func TestNewInterfaceDecls_AsyncIterable(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("ReadableStream", "", asyncIterable("any")),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls), "iter", "context")
	if !strings.Contains(src, "AsyncValues(Ctx context.Context)") {
		t.Errorf("async iterable: expected 'AsyncValues(Ctx context.Context)' in:\n%s", src)
	}
	if !strings.Contains(src, "iter.Seq2[any, error]") {
		t.Errorf("async iterable: expected 'iter.Seq2[any, error]' in:\n%s", src)
	}
}

func TestNewInterfaceDecls_AsyncPairIterableHasEntry(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("AsyncMap", "", asyncPairIterable("DOMString", "long")),
		tm, diag,
	)
	// Should include an EntryTypeDecl
	var entryFound bool
	for _, d := range decls {
		s := sourceOf(t, d)
		if strings.Contains(s, "type Entry[K, V any] struct") {
			entryFound = true
			break
		}
	}
	if !entryFound {
		t.Error("async pair iterable: expected EntryTypeDecl in result decls")
	}
	// Interface should reference AsyncEntries with Entry type
	src := sourceOf(t, firstDecl(t, decls), "iter", "context")
	if !strings.Contains(src, "AsyncEntries(") {
		t.Errorf("async pair iterable: expected 'AsyncEntries(' in interface:\n%s", src)
	}
}

func TestNewInterfaceDecls_Maplike(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("StylePropertyMap", "", maplike("DOMString", "any", false)),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls), "iter")
	for _, want := range []string{"Get(K string)", "Has(K string) bool", "Keys()", "Values()", "Entries()", "Size() int", "Set(K string, V any)", "Delete(K string)", "Clear()"} {
		if !strings.Contains(src, want) {
			t.Errorf("maplike: expected %q in source:\n%s", want, src)
		}
	}
}

func TestNewInterfaceDecls_MaplikeReadonly(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("StylePropertyMapReadOnly", "", maplike("DOMString", "any", true)),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls), "iter")
	for _, absent := range []string{"Set(", "Delete(", "Clear()"} {
		if strings.Contains(src, absent) {
			t.Errorf("readonly maplike: must not contain %q:\n%s", absent, src)
		}
	}
}

func TestNewInterfaceDecls_Setlike(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("EventTargetSet", "", setlike("any", false)),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls), "iter")
	for _, want := range []string{"Has(V any) bool", "Keys()", "Values()", "Entries()", "Size() int", "Add(V any)", "Delete(V any)", "Clear()"} {
		if !strings.Contains(src, want) {
			t.Errorf("setlike: expected %q in source:\n%s", want, src)
		}
	}
}

func TestNewInterfaceDecls_SetlikeReadonly(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("ReadOnlySet", "", setlike("any", true)),
		tm, diag,
	)
	src := sourceOf(t, firstDecl(t, decls), "iter")
	for _, absent := range []string{"Add(", "Delete(", "Clear()"} {
		if strings.Contains(src, absent) {
			t.Errorf("readonly setlike: must not contain %q:\n%s", absent, src)
		}
	}
}

// ---------------------------------------------------------------------------
// Multi-decl result — N decls for one interface
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_MultipleDecls(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	ctor := &webidl.Constructor{}
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("Node", "",
			constMember("ELEMENT_NODE", idlConstType("unsigned short"), "1"),
			ctor,
		),
		tm, diag,
	)
	// Expect InterfaceDecl + ConstBlockDecl + ConstructorDecl = 3
	if len(decls) < 3 {
		t.Errorf("multi-decl: expected ≥3 decls (interface + consts + ctor), got %d", len(decls))
	}
}

// ---------------------------------------------------------------------------
// Source is valid Go (round-trip through gofmt)
// ---------------------------------------------------------------------------

func TestNewInterfaceDecls_SourceIsValidGo(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	decls := codegen.NewInterfaceDecls(
		regularMergedDef("EventTarget", "",
			attr("type", true, idlAttrType("DOMString")),
			op("addEventListener", idlType("undefined"), arg("type", "DOMString")),
		),
		tm, diag,
	)
	for i, d := range decls {
		f := codegen.NewFile("gen")
		f.AddDecl(d)
		if _, err := f.Render(); err != nil {
			t.Errorf("decl[%d]: File.Render() error: %v", i, err)
		}
	}
}

func TestNewInterfaceDecls_IterableCollisionEmitsDiagnostic(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	// "Values" operation plus an iterable — iterable's Values method should
	// be skipped with a warning, not silently duplicated.
	def := regularMergedDef("Foo", "",
		op("Values", idlType("long")),
		&webidl.IterableLike{Kind: webidl.IterIterable, Types: []*webidl.IDLType{idlType("long")}},
	)
	decls := codegen.NewInterfaceDecls(def, tm, diag)
	if len(decls) == 0 {
		t.Fatal("expected at least one Decl")
	}
	// Diagnostic must contain a warning about the collision.
	if !strings.Contains(diag.Format(), "warning") {
		t.Errorf("expected a warning diagnostic for iterable/method collision, got:\n%s", diag.Format())
	}
	// The interface source must contain "Values" exactly once.
	src := sourceOf(t, decls[0], "iter")
	count := strings.Count(src, "Values(")
	if count != 1 {
		t.Errorf("expected exactly 1 Values method in interface, got %d:\n%s", count, src)
	}
}

func TestDedupeDecls_RemovesDuplicateEntry(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	// Two interfaces each with a pair async_iterable both emit EntryTypeDecl.
	def1 := regularMergedDef("AsyncA", "",
		&webidl.IterableLike{
			Kind:  webidl.IterAsyncIterable,
			Async: true,
			Types: []*webidl.IDLType{idlType("DOMString"), idlType("long")},
		},
	)
	def2 := regularMergedDef("AsyncB", "",
		&webidl.IterableLike{
			Kind:  webidl.IterAsyncIterable,
			Async: true,
			Types: []*webidl.IDLType{idlType("DOMString"), idlType("long")},
		},
	)
	all := append(
		codegen.NewInterfaceDecls(def1, tm, diag),
		codegen.NewInterfaceDecls(def2, tm, diag)...,
	)
	deduped := codegen.DedupeDecls(all)

	// Build a single File — must not error with duplicate "Entry".
	f := codegen.NewFile("gen")
	imp := codegen.NewImportTracker()
	imp.Add("iter")
	imp.Add("context")
	f.SetImports(imp)
	for _, d := range deduped {
		f.AddDecl(d)
	}
	if _, err := f.Render(); err != nil {
		t.Errorf("File.Render after DedupeDecls: %v", err)
	}
}
