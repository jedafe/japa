package expr

// TokenKind identifies the kind of a lexical token.
type TokenKind int

const (
	// TokenEOF represents the end of the input.
	TokenEOF TokenKind = iota
	// TokenNumber represents a numeric literal.
	TokenNumber
	// TokenString represents a string literal.
	TokenString
	// TokenIdent represents an identifier or variable.
	TokenIdent
	// TokenPlus is the + operator.
	TokenPlus
	// TokenMinus is the - operator.
	TokenMinus
	// TokenMul is the * operator.
	TokenMul
	// TokenDiv is the / operator.
	TokenDiv
	// TokenMod is the % operator.
	TokenMod
	// TokenLParen is a left parenthesis.
	TokenLParen
	// TokenRParen is a right parenthesis.
	TokenRParen
	// TokenComma is a comma.
	TokenComma
	// TokenEq is the == operator.
	TokenEq
	// TokenNeq is the != operator.
	TokenNeq
	// TokenLt is the < operator.
	TokenLt
	// TokenGt is the > operator.
	TokenGt
	// TokenLte is the <= operator.
	TokenLte
	// TokenGte is the >= operator.
	TokenGte
	// TokenPipe is the | operator.
	TokenPipe
)

var tokenNames = map[TokenKind]string{
	TokenNumber: "number",
	TokenString: "string",
	TokenIdent:  "ident",
	TokenPlus:   "+",
	TokenMinus:  "-",
	TokenMul:    "*",
	TokenDiv:    "/",
	TokenMod:    "%",
	TokenLParen: "(",
	TokenRParen: ")",
	TokenComma:  ",",
	TokenEq:     "==",
	TokenNeq:    "!=",
	TokenLt:     "<",
	TokenGt:     ">",
	TokenLte:    "<=",
	TokenGte:    ">=",
	TokenPipe:   "|",
}

var singleCharTokens = map[byte]TokenKind{
	'+': TokenPlus,
	'-': TokenMinus,
	'*': TokenMul,
	'/': TokenDiv,
	'%': TokenMod,
	'(': TokenLParen,
	')': TokenRParen,
	',': TokenComma,
	'|': TokenPipe,
}

// Token represents a single token from the lexer.
type Token struct {
	Kind TokenKind
	Val  string
	Pos  int
}

// IsEOF reports whether the token is the end-of-file marker.
func (t Token) IsEOF() bool { return t.Kind == TokenEOF }

func kindName(k TokenKind) string {
	if n, ok := tokenNames[k]; ok {
		return n
	}

	return "EOF"
}

// Lexer turns an expression string into a stream of Tokens.
type Lexer struct {
	input string
	pos   int
	ch    byte
}

// NewLexer returns a new lexer for the given input.
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

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	pos := l.pos - 1
	ch := l.ch

	if ch == 0 {
		return Token{Kind: TokenEOF, Pos: pos}
	}

	if kind, ok := singleCharTokens[ch]; ok {
		l.readChar()

		return Token{Kind: kind, Pos: pos}
	}

	switch ch {
	case '<', '>', '=', '!':
		return l.lexTwoCharOp(ch, pos)
	case '"':
		return Token{Kind: TokenString, Val: l.readString(), Pos: pos}
	}

	if isDigit(ch) {
		return Token{Kind: TokenNumber, Val: l.readNumber(), Pos: pos}
	}

	if isLetter(ch) {
		return Token{Kind: TokenIdent, Val: l.readIdent(), Pos: pos}
	}

	l.readChar()

	return Token{Kind: TokenIdent, Val: string(ch), Pos: pos}
}

func (l *Lexer) lexTwoCharOp(ch byte, pos int) Token {
	l.readChar()

	if ch == '<' && l.ch == '=' {
		l.readChar()

		return Token{Kind: TokenLte, Pos: pos}
	}

	if ch == '<' {
		return Token{Kind: TokenLt, Pos: pos}
	}

	if ch == '>' && l.ch == '=' {
		l.readChar()

		return Token{Kind: TokenGte, Pos: pos}
	}

	if ch == '>' {
		return Token{Kind: TokenGt, Pos: pos}
	}

	if ch == '=' && l.ch == '=' {
		l.readChar()

		return Token{Kind: TokenEq, Pos: pos}
	}

	if ch == '!' && l.ch == '=' {
		l.readChar()

		return Token{Kind: TokenNeq, Pos: pos}
	}

	return Token{Kind: TokenIdent, Val: string(ch), Pos: pos}
}

func isDigit(ch byte) bool  { return ch >= '0' && ch <= '9' }
func isLetter(ch byte) bool { return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') }
