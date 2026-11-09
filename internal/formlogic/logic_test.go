package formlogic

import "testing"

const (
	fldPurpose = "purpose"
	fldNope    = "nope"
	valWork    = "work"
	valStudy   = "study"
	opAnswered = "answered"
)

func TestEvalClauseOps(t *testing.T) {
	t.Parallel()

	answers := map[string]any{
		fldPurpose: valStudy,
		"pets":     []any{"dog", "cat"},
		"score":    80.0,
		"flag":     true,
		"empty":    "",
		"emptyarr": []any{},
	}

	cases := []struct {
		name string
		cl   Clause
		want bool
	}{
		{"eq hit", Clause{Field: fldPurpose, Op: "eq", Value: valStudy}, true},
		{"eq miss", Clause{Field: fldPurpose, Op: "eq", Value: valWork}, false},
		{"eq numeric", Clause{Field: "score", Op: "eq", Value: "80"}, true},
		{"eq bool", Clause{Field: "flag", Op: "eq", Value: "true"}, true},
		{"neq answered diff", Clause{Field: fldPurpose, Op: "neq", Value: valWork}, true},
		{"neq answered same", Clause{Field: fldPurpose, Op: "neq", Value: valStudy}, false},
		{"neq unanswered", Clause{Field: fldNope, Op: "neq", Value: "x"}, false},
		{"in hit", Clause{Field: fldPurpose, Op: "in", Value: []any{valWork, valStudy}}, true},
		{"in miss", Clause{Field: fldPurpose, Op: "in", Value: []any{valWork}}, false},
		{"contains hit", Clause{Field: "pets", Op: "contains", Value: "dog"}, true},
		{"contains miss", Clause{Field: "pets", Op: "contains", Value: "fish"}, false},
		{"contains nonarray", Clause{Field: fldPurpose, Op: "contains", Value: "x"}, false},
		{"answered yes", Clause{Field: fldPurpose, Op: opAnswered}, true},
		{"answered empty", Clause{Field: "empty", Op: opAnswered}, false},
		{"answered emptyarr", Clause{Field: "emptyarr", Op: opAnswered}, false},
		{"answered missing", Clause{Field: fldNope, Op: opAnswered}, false},
		{"not_answered missing", Clause{Field: fldNope, Op: "not_answered"}, true},
		{"not_answered present", Clause{Field: fldPurpose, Op: "not_answered"}, false},
		{"self default", Clause{Op: "eq", Value: valStudy}, true}, // selfKey fldPurpose
	}
	for _, c := range cases {
		selfKey := ""
		if c.name == "self default" {
			selfKey = fldPurpose
		}

		if got := evalClause(c.cl, answers, selfKey); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestConditionAllAny(t *testing.T) {
	t.Parallel()

	answers := map[string]any{"a": "x", "b": "y"}

	all := &Condition{All: []Clause{{Field: "a", Op: "eq", Value: "x"}, {Field: "b", Op: "eq", Value: "y"}}}
	if !all.Eval(answers, "") {
		t.Fatal("all should pass")
	}

	bad := &Condition{All: []Clause{{Field: "a", Op: "eq", Value: "x"}}, Any: []Clause{{Field: "b", Op: "eq", Value: "z"}}}
	if bad.Eval(answers, "") {
		t.Fatal("all&&any: any-group failed should be false")
	}

	var nilc *Condition
	if !nilc.Eval(answers, "") {
		t.Fatal("nil condition should be true")
	}
}

func TestFieldVisible(t *testing.T) {
	t.Parallel()

	if !FieldVisible(nil, map[string]any{}, "k") {
		t.Fatal("no config → visible")
	}

	if !FieldVisible([]byte(`{bad json`), map[string]any{}, "k") {
		t.Fatal("malformed → fail-open visible")
	}

	cfg := []byte(`{"visible_if":{"all":[{"field":"a","op":"eq","value":1}]}}`)
	if FieldVisible(cfg, map[string]any{"a": fldNope}, "k") {
		t.Fatal("failing rule should hide")
	}

	if !FieldVisible(cfg, map[string]any{"a": 1.0}, "k") {
		t.Fatal("passing rule should show")
	}
}

func TestHiddenByJump(t *testing.T) {
	t.Parallel()

	mk := func(key, step, cfg string) RuleField {
		return RuleField{Key: key, StepKey: step, Config: []byte(cfg)}
	}
	order := []string{"s1", "s2", "s3", "s4", "s5"}

	// Basic forward jump: s1 field jumps to s4 → s2,s3 hidden.
	fields := []RuleField{
		mk("f1", "s1", `{"jump":{"when":{"any":[{"op":"eq","value":"go"}]},"step":"s4"}}`),
		mk("f2", "s2", `{}`),
	}

	h := HiddenByJump(fields, order, map[string]any{"f1": "go"})
	if !h["s2"] || !h["s3"] || h["s4"] || len(h) != 2 {
		t.Fatalf("expected {s2,s3}, got %v", h)
	}

	// No trigger → nothing hidden.
	h = HiddenByJump(fields, order, map[string]any{"f1": "stop"})
	if len(h) != 0 {
		t.Fatalf("untriggered jump hid %v", h)
	}

	// Backward/self/unknown targets ignored.
	bad := []RuleField{
		mk("f3", "s3", `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s1"}}`),
		mk("f4", "s3", `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}`),
		mk("f5", "s3", `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"ghost"}}`),
	}

	h = HiddenByJump(bad, order, map[string]any{"f3": 1.0, "f4": 1.0, "f5": 1.0})
	if len(h) != 0 {
		t.Fatalf("invalid targets hid %v", h)
	}

	// Hidden firing field cannot jump.
	hid := []RuleField{
		mk("f6", "s1", `{"visible_if":{"all":[{"field":"gate","op":"eq","value":0}]},"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}`),
	}

	h = HiddenByJump(hid, order, map[string]any{"f6": 1.0})
	if len(h) != 0 {
		t.Fatalf("hidden field fired jump: %v", h)
	}

	// Union of intervals + chain: s1→s4 hides 2-3; s2 field jumps 5 → union {s2,s3,s4}.
	fields = []RuleField{
		mk("j1", "s1", `{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s4"}}`),
		mk("j2", "s2", `{"jump":{"when":{"any":[{"op":"eq","value":"b"}]},"step":"s5"}}`),
	}

	h = HiddenByJump(fields, order, map[string]any{"j1": "a", "j2": "b"})
	if len(h) != 3 || !h["s2"] || !h["s3"] || !h["s4"] {
		t.Fatalf("union expected {s2,s3,s4}, got %v", h)
	}
}
