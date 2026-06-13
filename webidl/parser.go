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

