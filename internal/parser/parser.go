package parser

import (
	"strings"
)

type tokenType int

const (
	tokenIdent tokenType = iota
	tokenAnd
	tokenOr
	tokenNot
	tokenLParen
	tokenRParen
	tokenEOF
)

type token struct {
	Type  tokenType
	Value string
}

func lex(input string) []token {
	s := strings.ReplaceAll(input, "(", " ( ")
	s = strings.ReplaceAll(s, ")", " ) ")
	s = strings.ReplaceAll(s, "+", " + ")
	s = strings.ReplaceAll(s, "-", " - ")
	s = strings.ReplaceAll(s, "|", " | ")
	fields := strings.Fields(s)
	var tokens []token
	for _, f := range fields {
		switch strings.ToUpper(f) {
		case "AND", "+":
			tokens = append(tokens, token{tokenAnd, "AND"})
		case "OR", "|":
			tokens = append(tokens, token{tokenOr, "OR"})
		case "NOT", "-":
			tokens = append(tokens, token{tokenNot, "NOT"})
		case "(":
			tokens = append(tokens, token{tokenLParen, "("})
		case ")":
			tokens = append(tokens, token{tokenRParen, ")"})
		default:
			tokens = append(tokens, token{tokenIdent, f})
		}
	}
	return append(tokens, token{tokenEOF, ""})
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) parseExpression() string {
	var parts []string

	for p.pos < len(p.tokens) {
		tok := p.tokens[p.pos]
		if tok.Type == tokenRParen || tok.Type == tokenEOF {
			break
		}

		// --- Implicit AND Logic ---
		// If the last part added was a term (ident or group) and the current
		// token is also a term (ident or '('), insert an implicit "AND".
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			isLastTerm := strings.HasPrefix(last, "cat:") || strings.HasSuffix(last, ")")
			isCurrTerm := tok.Type == tokenIdent || tok.Type == tokenLParen

			if isLastTerm && isCurrTerm {
				parts = append(parts, "AND")
			}
		}

		p.pos++
		switch tok.Type {
		case tokenLParen:
			parts = append(parts, "("+p.parseExpression()+")")
		case tokenIdent:
			parts = append(parts, "cat:"+tok.Value)
		case tokenAnd, tokenOr, tokenNot:
			parts = append(parts, tok.Value)
		}
	}

	if p.pos < len(p.tokens) && p.tokens[p.pos].Type == tokenRParen {
		p.pos++
	}
	return strings.Join(parts, " ")
}

func ParseReconstructCategoryExpression(input string) string {
	tokens := lex(input)
	p := &parser{tokens: tokens}
	return "(" + p.parseExpression() + ")"
}
