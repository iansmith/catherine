package codegen

import "testing"

func TestNewNamespaceDecl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		idlName    string
		methods    []nsMethod
		wantSource string
		wantName   string
		wantErrors int
	}{
		{
			name:    "empty namespace",
			idlName: "Console",
			methods: nil,
			wantSource: "type consoleType struct{}\n\n" +
				"var Console = &consoleType{}\n",
			wantName:   "Console",
			wantErrors: 0,
		},
		{
			name:    "one void operation",
			idlName: "Console",
			methods: []nsMethod{
				{goName: "Log", params: []ifaceParam{{goName: "msg", goType: "string"}}, returnType: ""},
			},
			wantSource: "type consoleType struct{}\n\n" +
				"var Console = &consoleType{}\n" +
				"\nfunc (c *consoleType) Log(msg string) {}\n",
			wantName:   "Console",
			wantErrors: 0,
		},
		{
			name:    "one operation with return",
			idlName: "Counter",
			methods: []nsMethod{
				{goName: "Count", params: nil, returnType: "int"},
			},
			wantSource: "type counterType struct{}\n\n" +
				"var Counter = &counterType{}\n" +
				"\nfunc (c *counterType) Count() int { panic(\"not implemented\") }\n",
			wantName:   "Counter",
			wantErrors: 0,
		},
		{
			name:    "getter readonly attribute",
			idlName: "Console",
			methods: []nsMethod{
				{goName: "ErrorCount", params: nil, returnType: "int32"},
			},
			wantSource: "type consoleType struct{}\n\n" +
				"var Console = &consoleType{}\n" +
				"\nfunc (c *consoleType) ErrorCount() int32 { panic(\"not implemented\") }\n",
			wantName:   "Console",
			wantErrors: 0,
		},
		{
			name:    "multiple methods mixed",
			idlName: "Console",
			methods: []nsMethod{
				{goName: "Log", params: []ifaceParam{{goName: "msg", goType: "string"}}, returnType: ""},
				{goName: "ErrorCount", params: nil, returnType: "int32"},
			},
			wantSource: "type consoleType struct{}\n\n" +
				"var Console = &consoleType{}\n" +
				"\nfunc (c *consoleType) Log(msg string) {}\n" +
				"\nfunc (c *consoleType) ErrorCount() int32 { panic(\"not implemented\") }\n",
			wantName:   "Console",
			wantErrors: 0,
		},
		{
			name:    "idl name needing sanitize",
			idlName: "my-namespace",
			methods: nil,
			wantSource: "type myNamespaceType struct{}\n\n" +
				"var MyNamespace = &myNamespaceType{}\n",
			wantName:   "MyNamespace",
			wantErrors: 0,
		},
		{
			name:       "empty name",
			idlName:    "",
			methods:    nil,
			wantSource: "type xType struct{}\n\nvar X = &xType{}\n",
			wantName:   "X",
			wantErrors: 1,
		},
		{
			name:    "declName returns sanitized name",
			idlName: "web-audio",
			methods: nil,
			wantSource: "type webAudioType struct{}\n\n" +
				"var WebAudio = &webAudioType{}\n",
			wantName:   "WebAudio",
			wantErrors: 0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := NewDiagnostics()
			d := NewNamespaceDecl(tc.idlName, tc.methods, diag)
			if d == nil {
				t.Fatal("NewNamespaceDecl returned nil")
			}
			if got := d.declName(); got != tc.wantName {
				t.Errorf("declName() = %q, want %q", got, tc.wantName)
			}
			if got := d.declSource(); got != tc.wantSource {
				t.Errorf("declSource() =\n%q\nwant\n%q", got, tc.wantSource)
			}
			if got := len(diag.Errors()); got != tc.wantErrors {
				t.Errorf("error count = %d, want %d (diag: %s)", got, tc.wantErrors, diag.Format())
			}
		})
	}
}
