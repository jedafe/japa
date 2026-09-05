package expr

import (
	"fmt"
	"strconv"
)

var precedenceTable = map[TokenKind]int{
	TokenPipe:  10,
	TokenEq:    20,
	TokenNeq:   20,
	TokenLt:    30,
	TokenGt:    30,
	TokenLte:   30,
	TokenGte:   30,
	TokenPlus:  40,
	TokenMinus: 40,
	TokenMul:   50,
	TokenDiv:   50,
	TokenMod:   50,
}

// Parser converts a stream of tokens into an expression AST.
type Parser struct {
	l    *Lexer
	cur  Token
	peek Token
}

// Parse parses an expression string into a Node.
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
		op := p.consume().Kind

		right, err := p.parseExpression(p.precedence(op) + 1)
		if err != nil {
			return nil, err
		}

		left = BinaryNode{Left: left, Op: op, Right: right}
	}

	return left, nil
}

//nolint:exhaustive // only the relevant prefix token kinds are handled
func (p *Parser) parsePrefix() (Node, error) {
	switch p.cur.Kind {
	case TokenNumber:
		return p.parseNumber()
	case TokenString:
		return p.parseString()
	case TokenIdent:
		return p.parseIdentOrCall()
	case TokenMinus:
		return p.parseUnary()
	case TokenLParen:
		return p.parseParen()
	}

	return nil, fmt.Errorf("unexpected token %v at pos %d", p.cur.Val, p.cur.Pos)
}

func (p *Parser) parseNumber() (Node, error) {
	tok := p.consume()

	v, err := strconv.ParseFloat(tok.Val, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number %q: %w", tok.Val, err)
	}

	return NumberNode{Val: v}, nil
}

func (p *Parser) parseString() (Node, error) {
	tok := p.consume()

	return StringNode{Val: tok.Val}, nil
}

func (p *Parser) parseIdentOrCall() (Node, error) {
	tok := p.consume()
	name := tok.Val
	pos := tok.Pos

	if p.cur.Kind != TokenLParen {
		return IdentNode{Name: name}, nil
	}

	// function call
	p.consume()

	args, err := p.parseCallArgs()
	if err != nil {
		return nil, err
	}

	if p.cur.Kind != TokenRParen {
		return nil, fmt.Errorf("unclosed call at pos %d", pos)
	}

	p.consume()

	return CallNode{Name: name, Args: args}, nil
}

func (p *Parser) parseCallArgs() ([]Node, error) {
	var args []Node

	for p.cur.Kind != TokenRParen && !p.cur.IsEOF() {
		arg, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}

		args = append(args, arg)

		if p.cur.Kind == TokenComma {
			p.consume()
		} else if p.cur.Kind != TokenRParen {
			return nil, fmt.Errorf("expected , or ) at pos %d", p.cur.Pos)
		}
	}

	return args, nil
}

func (p *Parser) parseUnary() (Node, error) {
	p.consume()

	expr, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}

	return UnaryNode{Op: TokenMinus, Expr: expr}, nil
}

func (p *Parser) parseParen() (Node, error) {
	p.consume()

	expr, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return expr, nil
}

func (p *Parser) consume() Token {
	cur := p.cur
	p.next()

	return cur
}

func (p *Parser) precedence(k TokenKind) int {
	if v, ok := precedenceTable[k]; ok {
		return v
	}

	return 0
}

func (p *Parser) String() string { return p.l.input }
