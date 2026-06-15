package webidl

import (
	"fmt"
	"slices"
)

// ParseError reports a parse or lex error.
type ParseError struct {
	Line    int
	Message string
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Message)
	}
	return e.Message
}

// Parse lexes and parses a WebIDL source string into a list of definitions.
func Parse(src string) ([]Definition, error) {
	tokens, err := Tokenize(src)
	if err != nil {
		if te, ok := err.(*TokenizeError); ok {
			return nil, &ParseError{Line: te.Line, Message: te.Message}
		}
		return nil, err
	}
	p := &parser{tokens: tokens}
	return p.parseAll()
}

// ParseAll lexes and parses a WebIDL source string in tolerant mode, returning
// all definitions that could be parsed alongside every error encountered.
// Unlike Parse, ParseAll continues past syntax errors at definition boundaries
// so that callers see the full set of diagnostics in a single pass.
func ParseAll(src string) ([]Definition, []*ParseError) {
	tokens, err := Tokenize(src)
	if err != nil {
		if te, ok := err.(*TokenizeError); ok {
			return nil, []*ParseError{{Line: te.Line, Message: te.Message}}
		}
		return nil, []*ParseError{{Message: err.Error()}}
	}
	p := &parser{tokens: tokens}
	return p.parseAllTolerant()
}

// parseAllTolerant is the tolerant entry point. Each definition attempt is
// wrapped in its own recover so a panic in one definition doesn't abort the
// rest. After any failure the cursor is advanced to the next definition
// boundary via syncToNextDefinition.
func (p *parser) parseAllTolerant() (defs []Definition, errs []*ParseError) {
	if len(p.tokens) == 1 { // only EOF sentinel
		return nil, nil
	}
	for p.current().Kind != TokEOF {
		firstTok := p.current()
		def, ea, pe := p.tryParseDefinition()
		if pe != nil {
			errs = append(errs, pe)
			p.syncToNextDefinition()
			continue
		}
		if def == nil {
			if len(ea) > 0 {
				errs = append(errs, &ParseError{Line: firstTok.Line, Message: "Stray extended attributes"})
				p.syncToNextDefinition()
				continue
			}
			break // no production matched and no stray extattrs — normal exit
		}
		def.setExtAttrs(ea)
		def.setSpan(spanFrom(firstTok))
		defs = append(defs, def)
	}
	if p.current().Kind != TokEOF {
		errs = append(errs, &ParseError{Line: p.current().Line, Message: "Unrecognised tokens"})
	}
	return defs, errs
}

// tryParseDefinition attempts to parse one top-level definition (including any
// leading extended attributes). It catches any *ParseError panic and returns
// it as pe instead of propagating it. Non-ParseError panics re-panic.
func (p *parser) tryParseDefinition() (def Definition, ea []*ExtAttr, pe *ParseError) {
	defer func() {
		if r := recover(); r != nil {
			if caught, ok := r.(*ParseError); ok {
				pe = caught
				return
			}
			panic(r)
		}
	}()
	ea = p.parseExtAttrs()
	def = p.parseDefinition()
	return def, ea, nil
}

// definitionStarters is the set of keywords that can open a top-level WebIDL
// definition. Used by syncToNextDefinition to identify recovery boundaries.
var definitionStarters = []string{
	"interface", "dictionary", "enum", "typedef",
	"namespace", "callback", "partial",
}

// syncToNextDefinition advances the cursor past any partially-consumed tokens
// to the next likely top-level definition boundary: a ';' followed immediately
// by a definition-starter keyword or '[' (start of extended attributes).
// Stops at EOF without advancing past it.
func (p *parser) syncToNextDefinition() {
	for p.current().Kind != TokEOF {
		if p.consume(";") != nil {
			t := p.current()
			if t.Kind == TokInline &&
				(slices.Contains(definitionStarters, t.Value) || t.Value == "[") {
				return
			}
		} else {
			p.pos++
		}
	}
}

// parser is the recursive-descent state.
type parser struct {
	tokens []Token
	pos    int
}

// current returns the token at the cursor (always valid; an EOF sentinel is
// guaranteed by the tokenizer).
func (p *parser) current() *Token {
	return &p.tokens[p.pos]
}

// errorf panics with a ParseError carrying the current token's line.
func (p *parser) errorf(format string, args ...any) {
	panic(&ParseError{Line: p.current().Line, Message: fmt.Sprintf(format, args...)})
}

// probeKind reports whether the current token has any of the given kinds.
func (p *parser) probeKind(kinds ...TokenKind) bool {
	return slices.Contains(kinds, p.current().Kind)
}

// probe reports whether the current token is an inline with the given value.
func (p *parser) probe(value string) bool {
	t := p.current()
	return t.Kind == TokInline && t.Value == value
}

// consume returns the current token (and advances) if it is an inline with
// one of the given values; otherwise nil.
func (p *parser) consume(values ...string) *Token {
	t := p.current()
	if t.Kind != TokInline || !slices.Contains(values, t.Value) {
		return nil
	}
	p.pos++
	return t
}

// consumeKind returns the current token (and advances) if it is any of the
// given kinds; otherwise nil.
func (p *parser) consumeKind(kinds ...TokenKind) *Token {
	t := p.current()
	if !slices.Contains(kinds, t.Kind) {
		return nil
	}
	p.pos++
	return t
}

// unconsume rewinds the cursor.
func (p *parser) unconsume(pos int) { p.pos = pos }

// parseAll is the entry point. Catches parser panics and converts to error.
func (p *parser) parseAll() (defs []Definition, err error) {
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(*ParseError); ok {
				err = pe
				return
			}
			panic(r)
		}
	}()
	if len(p.tokens) == 1 { // only EOF
		return nil, nil
	}
	for {
		firstTok := p.current()
		ea := p.parseExtAttrs()
		def := p.parseDefinition()
		if def == nil {
			if len(ea) > 0 {
				p.errorf("Stray extended attributes")
			}
			break
		}
		def.setExtAttrs(ea)
		def.setSpan(spanFrom(firstTok))
		defs = append(defs, def)
	}
	if p.current().Kind != TokEOF {
		p.errorf("Unrecognised tokens")
	}
	return defs, nil
}

// parseDefinition tries each top-level production in order.
func (p *parser) parseDefinition() Definition {
	if d := p.parseCallback(); d != nil {
		return d
	}
	if d := p.parseInterfaceLike(nil); d != nil {
		return d
	}
	if d := p.parsePartial(); d != nil {
		return d
	}
	if d := p.parseDictionary(nil); d != nil {
		return d
	}
	if d := p.parseEnum(); d != nil {
		return d
	}
	if d := p.parseTypedef(); d != nil {
		return d
	}
	if d := p.parseIncludes(); d != nil {
		return d
	}
	if d := p.parseNamespace(nil); d != nil {
		return d
	}
	return nil
}

// parsePartial handles `partial dictionary ...`, `partial interface ...`,
// `partial namespace ...`.
func (p *parser) parsePartial() Definition {
	partial := p.consume("partial")
	if partial == nil {
		return nil
	}
	if d := p.parseDictionary(partial); d != nil {
		return d
	}
	if d := p.parseInterfaceLike(partial); d != nil {
		return d
	}
	if d := p.parseNamespace(partial); d != nil {
		return d
	}
	p.errorf("Partial doesn't apply to anything")
	return nil
}

