# webidl-go

A hand-written [Web IDL](https://webidl.spec.whatwg.org/) parser in Go.

Ports the parsing layer of [w3c/webidl2.js](https://github.com/w3c/webidl2.js)
(the reference parser, used by web-platform-tests) to idiomatic Go. The AST is
Go-native; a separate translator emits the webidl2.js JSON shape for diffing
against the reference test baselines.

## Status

- **68 / 68** webidl2.js syntax-corpus tests pass (AST structurally matches the baseline).
- **84** invalid-corpus tests reject as expected; **18** are validator-only checks (e.g. missing `[Exposed]`, duplicate member names) which are out of scope for the parser and skipped.
- **332 / 334** webref shipping-spec IDL files parse cleanly. The two failures (`DOM-Style.idl`, `svg-paths.idl`) are also rejected by webidl2.js with the same errors — they predate modern Web IDL syntax.

## Use

```go
import "github.com/iansmith/webidl/webidl"

defs, err := webidl.Parse(src)
if err != nil {
    log.Fatal(err)
}
for _, d := range defs {
    switch x := d.(type) {
    case *webidl.Interface:
        fmt.Println(x.Name, len(x.Members))
    }
}
```

For diffing against the webidl2.js JSON shape:

```go
shape := webidl.ToJSONShape(defs)
out, _ := json.MarshalIndent(shape, "", "  ")
```

## CLI

```
go run ./cmd/webidl path/to/spec.idl          # JSON AST to stdout
go run ./cmd/webidl -tree path/to/spec.idl    # human-readable summary
cat spec.idl | go run ./cmd/webidl            # read from stdin
```

## Testing

Tests rely on the webidl2.js and webref corpora. Run the setup script to clone them:

```
./script/setup
go test ./webidl/...
```

The cloned corpora are gitignored — they're large and not part of this project.

## Scope

This is a **parser only**. The reference implementation has a second pass
that runs semantic validation (require `[Exposed]`, no duplicate member names,
no nullable union of dictionaries, etc.) — none of that is implemented here.
If you need a validator, build it on top of the AST.

We also do not preserve source trivia (whitespace, comments). The parser
emits an abstract AST, not a concrete syntax tree; you cannot round-trip IDL
byte-for-byte through it.

## Design notes

- Hand-written recursive descent. The Web IDL grammar (§13 of the spec) is
  LL(1) by design, so no backtracking or parser generator is needed.
- File layout mirrors `webidl2.js/lib/productions/` loosely: one Go file
  per major concern (tokenizer, parser core, productions, AST, JSON shape).
- Errors propagate via `panic` / `recover` at the top of `Parse()`. This is
  conventional for Go hand-written parsers (cf. `text/template`, `go/parser`).
