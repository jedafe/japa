package expr

import (
	"testing"
	"time"
)

func TestEvalArithmetic(t *testing.T) {
	t.Parallel()

	n, _ := Parse("2025 + 1")

	v, err := Eval(t.Context(), n, NewEnv(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}

	if f, ok := toNumber(v); !ok || f != 2026 {
		t.Fatalf("got %v, want 2026", v)
	}
}

func TestEvalAdjective(t *testing.T) {
	t.Parallel()

	n, _ := Parse(`adjective("Nigeria")`)

	v, err := Eval(t.Context(), n, NewEnv(time.Now()))
	if err != nil {
		t.Fatal(err)
	}

	if s, ok := toString(v); !ok || s != "Nigerian" {
		t.Fatalf("got %v, want Nigerian", v)
	}
}

func TestEvalFormat(t *testing.T) {
	t.Parallel()

	n, _ := Parse(`format(Now, "YYYY")`)

	v, err := Eval(t.Context(), n, NewEnv(time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}

	if s, ok := toString(v); !ok || s != "2026" {
		t.Fatalf("got %v, want 2026", v)
	}
}

func TestEvalStringConcat(t *testing.T) {
	t.Parallel()

	n, _ := Parse(`"Hello" + " " + "World"`)

	v, err := Eval(t.Context(), n, NewEnv(time.Now()))
	if err != nil {
		t.Fatal(err)
	}

	if s, ok := toString(v); !ok || s != "Hello World" {
		t.Fatalf("got %v, want 'Hello World'", v)
	}
}

func TestEvalNumberStringConcat(t *testing.T) {
	t.Parallel()

	n, _ := Parse(`"Year: " + 2026`)

	v, err := Eval(t.Context(), n, NewEnv(time.Now()))
	if err != nil {
		t.Fatal(err)
	}

	if s, ok := toString(v); !ok || s != "Year: 2026" {
		t.Fatalf("got %v, want 'Year: 2026'", v)
	}
}

func TestEvalLeftAssociativeSubtraction(t *testing.T) {
	t.Parallel()

	n, _ := Parse("10 - 5 - 2")

	v, err := Eval(t.Context(), n, NewEnv(time.Now()))
	if err != nil {
		t.Fatal(err)
	}

	if f, ok := toNumber(v); !ok || f != 3 {
		t.Fatalf("got %v, want 3", v)
	}
}

func TestEvalIf(t *testing.T) {
	t.Parallel()

	n, _ := Parse(`if(1 == 1, "yes", "no")`)

	v, err := Eval(t.Context(), n, NewEnv(time.Now()))
	if err != nil {
		t.Fatal(err)
	}

	if s, ok := toString(v); !ok || s != "yes" {
		t.Fatalf("got %v, want yes", v)
	}
}
