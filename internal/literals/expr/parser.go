package expr

import (
	"fmt"
	"strconv"
)

type Parser struct {
	l    *Lexer
	cur  Token
	peek Token
}

func Parse(input string) (Node, error) {
	p := &Parser{l: NewLexer(input)}
	p.next()
	p.next()
	return p.parseExpression(0)
}

func (p *Parser) next() {
	p.cur = p.peek
	p.peek = p.l.NextToken()
}

func (p *Parser) expect(k TokenKind) error {
	if p.cur.Kind != k {
		return fmt.Errorf("expected %v, got %v at pos %d", kindName(k), kindName(p.cur.Kind), p.cur.Pos)
	}
	p.next()
	return nil
}

func (p *Parser) parseExpression(precedence int) (Node, error) {
	left, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}

	for !p.cur.IsEOF() && precedence < p.precedence(p.cur.Kind) {
		op := p.cur.Kind
		p.next()
		right, err := p.parseExpression(p.precedence(op) + 1)
		if err != nil {
			return nil, err
		}
		left = BinaryNode{Left: left, Op: op, Right: right}
	}
	return left, nil
}

func (p *Parser) parsePrefix() (Node, error) {
	switch p.cur.Kind {
	case TokenNumber:
		v, err := strconv.ParseFloat(p.cur.Val, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", p.cur.Val, err)
		}
		n := NumberNode{Val: v}
		p.next()
		return n, nil
	case TokenString:
		n := StringNode{Val: p.cur.Val}
		p.next()
		return n, nil
	case TokenIdent:
		name := p.cur.Val
		pos := p.cur.Pos
		p.next()
		if p.cur.Kind == TokenLParen {
			// function call
			p.next()
			var args []Node
			for p.cur.Kind != TokenRParen && !p.cur.IsEOF() {
				arg, err := p.parseExpression(0)
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if p.cur.Kind == TokenComma {
					p.next()
				} else if p.cur.Kind != TokenRParen {
					return nil, fmt.Errorf("expected , or ) at pos %d", p.cur.Pos)
				}
			}
			if p.cur.Kind != TokenRParen {
				return nil, fmt.Errorf("unclosed call at pos %d", pos)
			}
			p.next()
			return CallNode{Name: name, Args: args}, nil
		}
		return IdentNode{Name: name}, nil
	case TokenMinus:
		p.next()
		expr, err := p.parsePrefix()
		if err != nil {
			return nil, err
		}
		return UnaryNode{Op: TokenMinus, Expr: expr}, nil
	case TokenLParen:
		p.next()
		expr, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return expr, nil
	}
	return nil, fmt.Errorf("unexpected token %v at pos %d", p.cur.Val, p.cur.Pos)
}

func (p *Parser) precedence(k TokenKind) int {
	switch k {
	case TokenPipe:
		return 10
	case TokenEq, TokenNeq:
		return 20
	case TokenLt, TokenGt, TokenLte, TokenGte:
		return 30
	case TokenPlus, TokenMinus:
		return 40
	case TokenMul, TokenDiv, TokenMod:
		return 50
	}
	return 0
}

func (p *Parser) String() string { return p.l.input }
