package expr

type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenNumber
	TokenString
	TokenIdent
	TokenPlus
	TokenMinus
	TokenMul
	TokenDiv
	TokenMod
	TokenLParen
	TokenRParen
	TokenComma
	TokenEq
	TokenNeq
	TokenLt
	TokenGt
	TokenLte
	TokenGte
	TokenPipe
)

type Token struct {
	Kind TokenKind
	Val  string
	Pos  int
}

func (t Token) IsEOF() bool { return t.Kind == TokenEOF }

func kindName(k TokenKind) string {
	switch k {
	case TokenNumber:
		return "number"
	case TokenString:
		return "string"
	case TokenIdent:
		return "ident"
	case TokenPlus:
		return "+"
	case TokenMinus:
		return "-"
	case TokenMul:
		return "*"
	case TokenDiv:
		return "/"
	case TokenMod:
		return "%"
	case TokenLParen:
		return "("
	case TokenRParen:
		return ")"
	case TokenComma:
		return ","
	case TokenEq:
		return "=="
	case TokenNeq:
		return "!="
	case TokenLt:
		return "<"
	case TokenGt:
		return ">"
	case TokenLte:
		return "<="
	case TokenGte:
		return ">="
	case TokenPipe:
		return "|"
	}
	return "EOF"
}

type Lexer struct {
	input string
	pos   int
	ch    byte
}

func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.pos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.pos]
	}
	l.pos++
}

func (l *Lexer) peek() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) readNumber() string {
	start := l.pos - 1
	for isDigit(l.ch) || l.ch == '.' {
		l.readChar()
	}
	return l.input[start : l.pos-1]
}

func (l *Lexer) readString() string {
	// l.ch == '"'
	l.readChar()
	start := l.pos - 1
	for l.ch != '"' && l.ch != 0 {
		l.readChar()
	}
	s := l.input[start : l.pos-1]
	l.readChar() // consume closing "
	return s
}

func (l *Lexer) readIdent() string {
	start := l.pos - 1
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[start : l.pos-1]
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()
	pos := l.pos - 1
	switch l.ch {
	case 0:
		return Token{Kind: TokenEOF, Pos: pos}
	case '+':
		l.readChar()
		return Token{Kind: TokenPlus, Pos: pos}
	case '-':
		l.readChar()
		return Token{Kind: TokenMinus, Pos: pos}
	case '*':
		l.readChar()
		return Token{Kind: TokenMul, Pos: pos}
	case '/':
		l.readChar()
		return Token{Kind: TokenDiv, Pos: pos}
	case '%':
		l.readChar()
		return Token{Kind: TokenMod, Pos: pos}
	case '(':
		l.readChar()
		return Token{Kind: TokenLParen, Pos: pos}
	case ')':
		l.readChar()
		return Token{Kind: TokenRParen, Pos: pos}
	case ',':
		l.readChar()
		return Token{Kind: TokenComma, Pos: pos}
	case '|':
		l.readChar()
		return Token{Kind: TokenPipe, Pos: pos}
	case '"':
		return Token{Kind: TokenString, Val: l.readString(), Pos: pos}
	case '<':
		if l.peek() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenLte, Pos: pos}
		}
		l.readChar()
		return Token{Kind: TokenLt, Pos: pos}
	case '>':
		if l.peek() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenGte, Pos: pos}
		}
		l.readChar()
		return Token{Kind: TokenGt, Pos: pos}
	case '=':
		if l.peek() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenEq, Pos: pos}
		}
		// single = is invalid; consume and return as ident.
		l.readChar()
		return Token{Kind: TokenIdent, Val: "=", Pos: pos}
	case '!':
		if l.peek() == '=' {
			l.readChar()
			l.readChar()
			return Token{Kind: TokenNeq, Pos: pos}
		}
		l.readChar()
		return Token{Kind: TokenIdent, Val: "!", Pos: pos}
	}

	if isDigit(l.ch) {
		return Token{Kind: TokenNumber, Val: l.readNumber(), Pos: pos}
	}
	if isLetter(l.ch) {
		return Token{Kind: TokenIdent, Val: l.readIdent(), Pos: pos}
	}

	ch := l.ch
	l.readChar()
	return Token{Kind: TokenIdent, Val: string(ch), Pos: pos}
}

func isDigit(ch byte) bool  { return ch >= '0' && ch <= '9' }
func isLetter(ch byte) bool { return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') }
