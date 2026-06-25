package codegen_test

import (
	"strings"
	"testing"

	"github.com/iansmith/webidl/codegen"
	"github.com/iansmith/webidl/webidl"
)

// ---------------------------------------------------------------------------
// helpers for building ExtAttr values
// ---------------------------------------------------------------------------

func extAttr(name string) *webidl.ExtAttr {
	return &webidl.ExtAttr{Name: name}
}

func extAttrIdent(name, value string) *webidl.ExtAttr {
	return &webidl.ExtAttr{
		Name: name,
		RHS:  &webidl.ExtAttrRHS{Type: "identifier", Value: value},
	}
}

func extAttrStar(name string) *webidl.ExtAttr {
	return &webidl.ExtAttr{
		Name: name,
		RHS:  &webidl.ExtAttrRHS{Type: "*"},
	}
}

func extAttrIdentList(name string, values ...string) *webidl.ExtAttr {
	items := make([]*webidl.ExtAttrItem, len(values))
	for i, v := range values {
		items[i] = &webidl.ExtAttrItem{Type: "identifier", Value: v}
	}
	return &webidl.ExtAttr{
		Name: name,
		RHS: &webidl.ExtAttrRHS{
			Type:   "identifier-list",
			IsList: true,
			Items:  items,
		},
	}
}

// ---------------------------------------------------------------------------
// table-driven tests
// ---------------------------------------------------------------------------

func TestParseExtAttrs_Empty(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs(nil, diag)
	if got.ExposedScopes != nil {
		t.Errorf("empty: ExposedScopes = %v; want nil", got.ExposedScopes)
	}
	if got.SecureContext || got.RaisesException || got.Custom || got.Replaceable ||
		got.Clamp || got.EnforceRange || got.AllowShared || got.NewObject || got.SameObject ||
		got.ReflectPresent || got.CEReactions || got.LegacyUnforgeable {
		t.Error("empty: expected all bool fields to be false")
	}
	if got.RuntimeEnabled != "" {
		t.Errorf("empty: RuntimeEnabled = %q; want empty", got.RuntimeEnabled)
	}
	if got.PutForwards != "" {
		t.Errorf("empty: PutForwards = %q; want empty", got.PutForwards)
	}
	if got.ReflectAttr != "" {
		t.Errorf("empty: ReflectAttr = %q; want empty", got.ReflectAttr)
	}
	if len(got.UnknownAttrs) != 0 {
		t.Errorf("empty: UnknownAttrs = %v; want empty", got.UnknownAttrs)
	}
	if !diag.IsClean() {
		t.Errorf("empty: unexpected diagnostics: %s", diag.Format())
	}
}

func TestParseExtAttrs_ExposedWindow(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttrIdent("Exposed", "Window")}, diag)
	if len(got.ExposedScopes) != 1 || got.ExposedScopes[0] != "Window" {
		t.Errorf("Exposed=Window: ExposedScopes = %v; want [Window]", got.ExposedScopes)
	}
	if !diag.IsClean() {
		t.Errorf("Exposed=Window: unexpected diagnostics: %s", diag.Format())
	}
}

func TestParseExtAttrs_ExposedList(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs(
		[]*webidl.ExtAttr{extAttrIdentList("Exposed", "Window", "Worker")},
		diag,
	)
	if len(got.ExposedScopes) != 2 || got.ExposedScopes[0] != "Window" || got.ExposedScopes[1] != "Worker" {
		t.Errorf("Exposed=(Window,Worker): ExposedScopes = %v; want [Window Worker]", got.ExposedScopes)
	}
}

func TestParseExtAttrs_ExposedStar(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttrStar("Exposed")}, diag)
	if len(got.ExposedScopes) != 1 || got.ExposedScopes[0] != "*" {
		t.Errorf("Exposed=*: ExposedScopes = %v; want [*]", got.ExposedScopes)
	}
}

func TestParseExtAttrs_RaisesException(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("RaisesException")}, diag)
	if !got.RaisesException {
		t.Error("RaisesException: expected true")
	}
}

func TestParseExtAttrs_Custom(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("Custom")}, diag)
	if !got.Custom {
		t.Error("Custom: expected true")
	}
}

func TestParseExtAttrs_RuntimeEnabled(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttrIdent("RuntimeEnabled", "Foo")}, diag)
	if got.RuntimeEnabled != "Foo" {
		t.Errorf("RuntimeEnabled=Foo: got %q; want Foo", got.RuntimeEnabled)
	}
}

func TestParseExtAttrs_PutForwards(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttrIdent("PutForwards", "theAttr")}, diag)
	if got.PutForwards != "theAttr" {
		t.Errorf("PutForwards=theAttr: got %q; want theAttr", got.PutForwards)
	}
}

func TestParseExtAttrs_Replaceable(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("Replaceable")}, diag)
	if !got.Replaceable {
		t.Error("Replaceable: expected true")
	}
}

func TestParseExtAttrs_Clamp(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("Clamp")}, diag)
	if !got.Clamp {
		t.Error("Clamp: expected true")
	}
}

func TestParseExtAttrs_EnforceRange(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("EnforceRange")}, diag)
	if !got.EnforceRange {
		t.Error("EnforceRange: expected true")
	}
}

func TestParseExtAttrs_AllowShared(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("AllowShared")}, diag)
	if !got.AllowShared {
		t.Error("AllowShared: expected true")
	}
}

func TestParseExtAttrs_NewObject(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("NewObject")}, diag)
	if !got.NewObject {
		t.Error("NewObject: expected true")
	}
}

func TestParseExtAttrs_SameObject(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("SameObject")}, diag)
	if !got.SameObject {
		t.Error("SameObject: expected true")
	}
}

func TestParseExtAttrs_ReflectNoRHS(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("Reflect")}, diag)
	if !got.ReflectPresent {
		t.Error("Reflect (no RHS): ReflectPresent expected true")
	}
	if got.ReflectAttr != "" {
		t.Errorf("Reflect (no RHS): ReflectAttr = %q; want empty", got.ReflectAttr)
	}
}

func TestParseExtAttrs_ReflectWithAttr(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttrIdent("Reflect", "foo")}, diag)
	if !got.ReflectPresent {
		t.Error("Reflect=foo: ReflectPresent expected true")
	}
	if got.ReflectAttr != "foo" {
		t.Errorf("Reflect=foo: ReflectAttr = %q; want foo", got.ReflectAttr)
	}
}

func TestParseExtAttrs_CEReactions(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("CEReactions")}, diag)
	if !got.CEReactions {
		t.Error("CEReactions: expected true")
	}
}

func TestParseExtAttrs_LegacyUnforgeable(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("LegacyUnforgeable")}, diag)
	if !got.LegacyUnforgeable {
		t.Error("LegacyUnforgeable: expected true")
	}
}

func TestParseExtAttrs_UnknownAttr(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("UnknownFoo")}, diag)
	if len(got.UnknownAttrs) != 1 || got.UnknownAttrs[0] != "UnknownFoo" {
		t.Errorf("UnknownFoo: UnknownAttrs = %v; want [UnknownFoo]", got.UnknownAttrs)
	}
	if !strings.Contains(diag.Format(), "warning") {
		t.Errorf("UnknownFoo: expected 'warning' in diagnostics: %s", diag.Format())
	}
	if !strings.Contains(diag.Format(), "UnknownFoo") {
		t.Errorf("UnknownFoo: expected attribute name in warning: %s", diag.Format())
	}
}

func TestParseExtAttrs_MultipleMixed(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	attrs := []*webidl.ExtAttr{
		extAttrIdent("Exposed", "Window"),
		extAttr("RaisesException"),
		extAttr("Custom"),
	}
	got := codegen.ParseExtAttrs(attrs, diag)
	if len(got.ExposedScopes) != 1 || got.ExposedScopes[0] != "Window" {
		t.Errorf("mixed: ExposedScopes = %v; want [Window]", got.ExposedScopes)
	}
	if !got.RaisesException {
		t.Error("mixed: RaisesException expected true")
	}
	if !got.Custom {
		t.Error("mixed: Custom expected true")
	}
	if !diag.IsClean() {
		t.Errorf("mixed: unexpected diagnostics: %s", diag.Format())
	}
}

func TestParseExtAttrs_MultipleUnknown(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	attrs := []*webidl.ExtAttr{
		extAttr("Foo"),
		extAttr("Bar"),
	}
	got := codegen.ParseExtAttrs(attrs, diag)
	if len(got.UnknownAttrs) != 2 {
		t.Errorf("multiple unknown: UnknownAttrs = %v; want [Foo Bar]", got.UnknownAttrs)
	}
	warningCount := strings.Count(diag.Format(), "warning")
	if warningCount != 2 {
		t.Errorf("multiple unknown: expected 2 warnings, got %d:\n%s", warningCount, diag.Format())
	}
}

func TestParseExtAttrs_NilDiag(t *testing.T) {
	t.Parallel()
	// Must not panic when diag is nil.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ParseExtAttrs panicked with nil diag: %v", r)
		}
	}()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("UnknownFoo")}, nil)
	if len(got.UnknownAttrs) != 1 {
		t.Errorf("nil diag: expected UnknownAttrs=[UnknownFoo], got %v", got.UnknownAttrs)
	}
}

func TestParseExtAttrs_SecureContext(t *testing.T) {
	t.Parallel()
	diag := codegen.NewDiagnostics()
	got := codegen.ParseExtAttrs([]*webidl.ExtAttr{extAttr("SecureContext")}, diag)
	if !got.SecureContext {
		t.Error("SecureContext: expected true")
	}
	if !diag.IsClean() {
		t.Errorf("SecureContext: unexpected diagnostics: %s", diag.Format())
	}
}
