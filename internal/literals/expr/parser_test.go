package expr

import "testing"

func TestParseArithmetic(t *testing.T) {
	t.Parallel()

	n, err := Parse("Current_Year + 1")
	if err != nil {
		t.Fatal(err)
	}

	bn, ok := n.(BinaryNode)
	if !ok {
		t.Fatalf("expected BinaryNode, got %T", n)
	}

	if _, ok := bn.Left.(IdentNode); !ok {
		t.Fatalf("left = %T, want IdentNode", bn.Left)
	}

	if _, ok := bn.Right.(NumberNode); !ok {
		t.Fatalf("right = %T, want NumberNode", bn.Right)
	}
}

func TestParseFunction(t *testing.T) {
	t.Parallel()

	n, err := Parse("adjective(Destination_Country)")
	if err != nil {
		t.Fatal(err)
	}

	call, ok := n.(CallNode)
	if !ok || call.Name != "adjective" || len(call.Args) != 1 {
		t.Fatalf("expected CallNode adjective with 1 arg, got %v", n)
	}
}

func TestParseComparison(t *testing.T) {
	t.Parallel()

	_, err := Parse("Current_Year >= 2026")
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseStringConcat(t *testing.T) {
	t.Parallel()

	n, err := Parse(`"Hello" + " " + "World"`)
	if err != nil {
		t.Fatal(err)
	}

	bn, ok := n.(BinaryNode)
	if !ok {
		t.Fatalf("expected BinaryNode, got %T", n)
	}

	if bn.Op != TokenPlus {
		t.Fatalf("expected +, got %v", bn.Op)
	}

	if _, ok := bn.Left.(BinaryNode); !ok {
		t.Fatalf("expected left-associative tree, left = %T", bn.Left)
	}
}
