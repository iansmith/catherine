package codegen

import "testing"

func TestNewCallbackFuncDecl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		idlName         string
		params          []callbackParam
		returnType      string
		raisesException bool
		wantSource      string
		wantName        string
		wantErrors      int
	}{
		{
			name:            "void no exception",
			idlName:         "F",
			params:          nil,
			returnType:      "",
			raisesException: false,
			wantSource:      "type F func()\n",
			wantName:        "F",
			wantErrors:      0,
		},
		{
			name:            "void with exception",
			idlName:         "F",
			params:          nil,
			returnType:      "",
			raisesException: true,
			wantSource:      "type F func() error\n",
			wantName:        "F",
			wantErrors:      0,
		},
		{
			name:            "return type no exception",
			idlName:         "F",
			params:          []callbackParam{{goType: "int32"}},
			returnType:      "string",
			raisesException: false,
			wantSource:      "type F func(int32) string\n",
			wantName:        "F",
			wantErrors:      0,
		},
		{
			name:            "return type with exception",
			idlName:         "F",
			params:          []callbackParam{{goType: "int32"}},
			returnType:      "string",
			raisesException: true,
			wantSource:      "type F func(int32) (string, error)\n",
			wantName:        "F",
			wantErrors:      0,
		},
		{
			name:  "multiple params",
			idlName: "F",
			params: []callbackParam{
				{goType: "string"},
				{goType: "bool"},
			},
			returnType:      "",
			raisesException: false,
			wantSource:      "type F func(string, bool)\n",
			wantName:        "F",
			wantErrors:      0,
		},
		{
			name:            "variadic param",
			idlName:         "F",
			params:          []callbackParam{{goType: "string", variadic: true}},
			returnType:      "",
			raisesException: false,
			wantSource:      "type F func(...string)\n",
			wantName:        "F",
			wantErrors:      0,
		},
		{
			name:            "empty name",
			idlName:         "",
			params:          nil,
			returnType:      "",
			raisesException: false,
			wantSource:      "type X func()\n",
			wantName:        "X",
			wantErrors:      1,
		},
		{
			name:            "name needing sanitize",
			idlName:         "my-callback",
			params:          nil,
			returnType:      "",
			raisesException: false,
			wantSource:      "type MyCallback func()\n",
			wantName:        "MyCallback",
			wantErrors:      0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := NewDiagnostics()
			d := NewCallbackFuncDecl(tc.idlName, tc.params, tc.returnType, tc.raisesException, diag)
			if d == nil {
				t.Fatal("NewCallbackFuncDecl returned nil")
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
