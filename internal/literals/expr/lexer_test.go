package expr

import "testing"

func TestLexer(t *testing.T) {
	input := `Current_Year + 1 >= 2026`
	l := NewLexer(input)
	expected := []TokenKind{TokenIdent, TokenPlus, TokenNumber, TokenGte, TokenNumber}
	for _, want := range expected {
		tok := l.NextToken()
		if tok.Kind != want {
			t.Fatalf("token kind = %v; want %v (val=%s)", tok.Kind, want, tok.Val)
		}
	}
	if l.NextToken().Kind != TokenEOF {
		t.Fatal("expected EOF")
	}
}
