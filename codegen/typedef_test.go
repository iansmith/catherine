package codegen

import "testing"

func TestNewTypedefDecl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		idlName    string
		goType     string
		isAlias    bool
		wantSource string
		wantName   string
		wantErrors int
	}{
		{
			name:       "basic distinct type",
			idlName:    "EventHandler",
			goType:     "func(Event)",
			isAlias:    false,
			wantSource: "type EventHandler func(Event)\n",
			wantName:   "EventHandler",
			wantErrors: 0,
		},
		{
			name:       "type alias",
			idlName:    "DOMString",
			goType:     "string",
			isAlias:    true,
			wantSource: "type DOMString = string\n",
			wantName:   "DOMString",
			wantErrors: 0,
		},
		{
			name:       "idl name needing sanitize",
			idlName:    "my-event",
			goType:     "func()",
			isAlias:    false,
			wantSource: "type MyEvent func()\n",
			wantName:   "MyEvent",
			wantErrors: 0,
		},
		{
			name:       "empty idlName",
			idlName:    "",
			goType:     "string",
			isAlias:    false,
			wantSource: "type X string\n",
			wantName:   "X",
			wantErrors: 1,
		},
		{
			name:       "empty goType",
			idlName:    "Foo",
			goType:     "",
			isAlias:    false,
			wantSource: "type Foo \n",
			wantName:   "Foo",
			wantErrors: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := NewDiagnostics()
			d := NewTypedefDecl(tc.idlName, tc.goType, tc.isAlias, diag)
			if d == nil {
				t.Fatal("NewTypedefDecl returned nil")
			}
			if got := d.declName(); got != tc.wantName {
				t.Errorf("declName() = %q, want %q", got, tc.wantName)
			}
			if got := d.declSource(); got != tc.wantSource {
				t.Errorf("declSource() = %q, want %q", got, tc.wantSource)
			}
			if got := len(diag.Errors()); got != tc.wantErrors {
				t.Errorf("error count = %d, want %d (diag: %s)", got, tc.wantErrors, diag.Format())
			}
		})
	}
}
