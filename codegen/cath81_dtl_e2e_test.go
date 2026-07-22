package codegen_test

// CATH-81 Phase 0: red test — end-to-end verify a generated DOMTokenList binding
// runs from JS (via the CATH-77 runGeneratedJS harness), backed by a fake impl.
//
// This fails on current code because the binding generator drops all but the
// FIRST argument of a variadic operation: `add('b','c')` emits
// `b.impl.Add(rt.Coerce[string](b.ctx, call.Argument(0)))`, so 'c' is lost. The
// fix spreads the JS arguments into the Go variadic. Everything else the runner
// exercises (contains / toggle / value get+set / indexed getter / forEach /
// toString) already works — the assertion below is red solely on the variadic
// arg-drop.

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
)

const cath81DTLIDL = `
[Exposed=Window]
interface DOMTokenList {
  readonly attribute unsigned long length;
  getter DOMString? item(unsigned long index);
  boolean contains(DOMString token);
  undefined add(DOMString... tokens);
  undefined remove(DOMString... tokens);
  boolean toggle(DOMString token, optional boolean force);
  boolean replace(DOMString token, DOMString newToken);
  boolean supports(DOMString token);
  stringifier attribute DOMString value;
  iterable<DOMString>;
};
`

// cath81DTLRunner is a package-gen runner: a fake DOMTokenList impl (ordered
// slice → deterministic iteration) seeded with {"a"}, driven from JS.
const cath81DTLRunner = `package gen

import (
	"iter"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/iansmith/webidl/jsbinding"
)

type e2eEnv struct{}

func (e2eEnv) Root() any                   { return nil }
func (e2eEnv) Construct(string, []any) any { return nil }

type fakeDTL struct{ toks []string }

func (d *fakeDTL) idx(t string) int {
	for i, x := range d.toks {
		if x == t {
			return i
		}
	}
	return -1
}
func (d *fakeDTL) LengthAttr() uint32 { return uint32(len(d.toks)) }
func (d *fakeDTL) Index(i uint32) *string {
	if int(i) < len(d.toks) {
		s := d.toks[i]
		return &s
	}
	return nil
}
func (d *fakeDTL) Contains(t string) bool { return d.idx(t) >= 0 }
func (d *fakeDTL) Add(ts ...string) {
	for _, t := range ts {
		if d.idx(t) < 0 {
			d.toks = append(d.toks, t)
		}
	}
}
func (d *fakeDTL) Remove(ts ...string) {
	for _, t := range ts {
		if i := d.idx(t); i >= 0 {
			d.toks = append(d.toks[:i], d.toks[i+1:]...)
		}
	}
}
func (d *fakeDTL) Toggle(t string, force bool) bool {
	if d.idx(t) >= 0 {
		d.Remove(t)
		return false
	}
	d.Add(t)
	return true
}
func (d *fakeDTL) Replace(t, n string) bool {
	if i := d.idx(t); i >= 0 {
		d.toks[i] = n
		return true
	}
	return false
}
func (d *fakeDTL) Supports(string) bool  { return true }
func (d *fakeDTL) ValueAttr() string     { return strings.Join(d.toks, " ") }
func (d *fakeDTL) String() string        { return strings.Join(d.toks, " ") }
func (d *fakeDTL) SetValueAttr(v string) { d.toks = strings.Fields(v) }
func (d *fakeDTL) Values() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, t := range d.toks {
			if !yield(t) {
				return
			}
		}
	}
}
func (d *fakeDTL) Keys() iter.Seq[uint32] {
	return func(yield func(uint32) bool) {
		for i := range d.toks {
			if !yield(uint32(i)) {
				return
			}
		}
	}
}
func (d *fakeDTL) Entries() iter.Seq2[uint32, string] {
	return func(yield func(uint32, string) bool) {
		for i, t := range d.toks {
			if !yield(uint32(i), t) {
				return
			}
		}
	}
}
func (d *fakeDTL) ForEach(fn func(string, uint32)) {
	for i, t := range d.toks {
		fn(t, uint32(i))
	}
}

func TestE2E(t *testing.T) {
	vm := goja.New()
	ctx := jsbinding.NewCtx(vm, e2eEnv{})
	impl := &fakeDTL{toks: []string{"a"}}
	m := ctx.Wrap(impl, func() goja.DynamicObject { return &DOMTokenListBinding{ctx: ctx, impl: impl} })
	vm.Set("m", m)

	js := "var log=[];" +
		"log.push('has-a:'+m.contains('a'));" +
		"log.push('has-z:'+m.contains('z'));" +
		"m.add('b','c');" + // variadic — both must land
		"log.push('add:'+m.value);" +
		"log.push('len:'+m.length);" +
		"m.remove('b');" +
		"log.push('rm:'+m.value);" +
		"log.push('tog:'+m.toggle('a'));" +
		"log.push('aftertog:'+m.value);" +
		"m.value='x y';" + // stringifier attribute setter
		"log.push('set:'+m.value+':'+m.length);" +
		"log.push('idx0:'+m[0]);" + // indexed getter
		"var fe=[];m.forEach(function(v){fe.push(v);});" +
		"log.push('fe:'+fe.join(','));" +
		"log.push('str:'+m.toString());" +
		"log.join('|')"
	res, err := vm.RunString(js)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "has-a:true|has-z:false|add:a b c|len:3|rm:a c|tog:false|aftertog:c|set:x y:2|idx0:x|fe:x,y|str:x y"
	if got := res.String(); got != want {
		t.Fatalf("DOMTokenList JS output mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

// TestCATH81_EndToEnd_DOMTokenList_RunsJS generates the DOMTokenList binding,
// compiles it with the fake impl, runs the JS above in goja, and asserts the
// composite output. Red until variadic arg-spreading is fixed.
func TestCATH81_EndToEnd_DOMTokenList_RunsJS(t *testing.T) {
	runGeneratedJS(t, cath81DTLIDL, cath81DTLRunner)
}

// TestCATH81_VariadicOp_SpreadsAllArgs is a source-level guard that runs even
// without a Go toolchain (the E2E test above skips there). It asserts the
// generated binding for a variadic operation spreads ALL JS arguments into the
// Go variadic, rather than forwarding only call.Argument(0).
func TestCATH81_VariadicOp_SpreadsAllArgs(t *testing.T) {
	t.Parallel()
	tokens := arg("tokens", "DOMString")
	tokens.Variadic = true
	def := regularMergedDef("Toks", "", op("add", idlType("undefined"), tokens))
	src := sourceOf(t, firstDecl(t, codegen.NewBindingDecls(def, tm, codegen.NewDiagnostics())), gojaPkg)

	// The bug: only the first arg is forwarded.
	if strings.Contains(src, "b.impl.Add(rt.Coerce[string](b.ctx, call.Argument(0)))") {
		t.Errorf("variadic add must not forward only call.Argument(0):\n%s", src)
	}
	// The fix: a Go variadic spread (…) into the call.
	if !strings.Contains(src, "...)") {
		t.Errorf("variadic add must spread all JS args into the Go variadic (expected a `...)` spread):\n%s", src)
	}
}
