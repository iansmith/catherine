# webidl-go

A hand-written [Web IDL](https://webidl.spec.whatwg.org/) parser in Go.

Ports the parsing layer of [w3c/webidl2.js](https://github.com/w3c/webidl2.js)
(the reference parser, used by web-platform-tests) to idiomatic Go. The AST is
Go-native; a separate translator emits the webidl2.js JSON shape for diffing
against the reference test baselines.

## Status

- **68 / 68** webidl2.js syntax-corpus tests pass (AST structurally matches the baseline).
- **102 / 102** invalid-corpus tests pass (0 skipped). Validator-only checks — `[Exposed]` required, duplicate members, nullable union-of-dict, deprecated extended attributes, and more — are covered by the semantic validator.
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

### Parser

Hand-written recursive descent covering the full Web IDL grammar (§13 of the
spec). Source trivia (whitespace, comments) is not preserved — the output is
an abstract AST, not a concrete syntax tree; you cannot round-trip IDL
byte-for-byte through it.

### Semantic validator

A second pass runs semantic validation on the parsed AST:

```go
errs := webidl.Validate(defs)
for _, e := range errs {
    fmt.Println(e)
}
```

Implemented rules:

| Rule | What it checks |
|---|---|
| `no-duplicate` | No duplicate member names within an interface |
| `no-cross-overload` | Operation overloads must not mix regular and static |
| `constructor-member` | Constructor members only on interfaces with `[Constructor]` |
| `incomplete-op` | Operations must declare a return type |
| `attr-invalid-type` | Attributes may not use `sequence<>`, `record<>`, or `any` |
| `no-nullable-union-dict` | A nullable union type must not include a dictionary member |
| `async-sequence-idl-to-js` | `async_sequence<T>` is not a valid IDL-to-JS return/argument type |
| `dict-arg-default` | Dictionary-typed arguments must carry a default value |
| `dict-arg-optional` | Dictionary-typed arguments must be marked `optional` |
| `no-nullable-dict-arg` | Dictionary-typed arguments must not be nullable |
| `require-exposed` | Non-partial interfaces and namespaces must carry `[Exposed]` |
| `no-constructible-global` | `[Global]` interfaces may not declare constructors or `[LegacyFactoryFunction]` |
| `renamed-legacy` | Flags renamed extended attributes (e.g. `TreatUndefinedAs` → `LegacyTreatUndefinedAs`) |
| `migrate-allowshared` | `[AllowShared] BufferSource` → `AllowSharedBufferSource` |
| `replace-void` | `void` return type → `undefined` |
| `obsolete-async-iterable-syntax` | Async iterable must not use the legacy space-separated form |

## Design notes

- Hand-written recursive descent. The Web IDL grammar (§13 of the spec) is
  LL(1) by design, so no backtracking or parser generator is needed.
- File layout mirrors `webidl2.js/lib/productions/` loosely: one Go file
  per major concern (tokenizer, parser core, productions, AST, JSON shape).
- Errors propagate via `panic` / `recover` at the top of `Parse()`. This is
  conventional for Go hand-written parsers (cf. `text/template`, `go/parser`).
