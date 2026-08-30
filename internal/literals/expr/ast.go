package expr

type Node interface {
	node()
}

type NumberNode struct{ Val float64 }

func (NumberNode) node() {}

type StringNode struct{ Val string }

func (StringNode) node() {}

type IdentNode struct{ Name string }

func (IdentNode) node() {}

type UnaryNode struct {
	Op   TokenKind
	Expr Node
}

func (UnaryNode) node() {}

type BinaryNode struct {
	Left  Node
	Op    TokenKind
	Right Node
}

func (BinaryNode) node() {}

type CallNode struct {
	Name string
	Args []Node
}

func (CallNode) node() {}
