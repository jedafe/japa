// Package expr implements a small, whitelist-only expression language used by
// template literals. It supports numbers, strings, identifiers, unary and
// binary operators, and a limited set of function calls.
package expr

// Node is the interface implemented by all expression AST nodes.
type Node interface {
	node()
}

// NumberNode represents a numeric literal.
type NumberNode struct{ Val float64 }

func (NumberNode) node() {}

// StringNode represents a string literal.
type StringNode struct{ Val string }

func (StringNode) node() {}

// IdentNode represents a variable or constant identifier.
type IdentNode struct{ Name string }

func (IdentNode) node() {}

// UnaryNode represents a unary operation (currently only negation).
type UnaryNode struct {
	Op   TokenKind
	Expr Node
}

func (UnaryNode) node() {}

// BinaryNode represents a binary operation such as +, -, ==, or <.
type BinaryNode struct {
	Left  Node
	Op    TokenKind
	Right Node
}

func (BinaryNode) node() {}

// CallNode represents a function call with a name and arguments.
type CallNode struct {
	Name string
	Args []Node
}

func (CallNode) node() {}
