package formlogic

import (
	"reflect"
	"testing"
)

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

	// A non-object clause element unmarshals to a zero Clause{} (the type
	// error is swallowed) which evaluates false — broken clauses fail
	// closed, not vacuous-true. Kept in sync with the TS/Dart evaluators.
	if FieldVisible([]byte(`{"visible_if":{"all":[123]}}`), answers, "k") {
		t.Fatal("malformed all clause should fail closed")
	}

	if FieldVisible([]byte(`{"visible_if":{"any":[123]}}`), answers, "k") {
		t.Fatal("malformed any clause should fail closed")
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

func TestVisiblePath(t *testing.T) {
	t.Parallel()

	mk := func(key, cfg string) RuleField {
		return RuleField{Key: key, StepKey: "s1", Config: []byte(cfg)}
	}

	// gated opens on driver unanswered-or-yes; revealed needs driver=yes.
	// With stage=post the driver is hidden — its retained answer is off
	// the visible path and reads as unanswered downstream (ADR-137).
	fields := []RuleField{
		mk("driver", `{"visible_if":{"all":[{"field":"stage","op":"eq","value":"pre"}]}}`),
		mk("gated", `{"visible_if":{"any":[{"field":"driver","op":"not_answered"},{"field":"driver","op":"eq","value":"yes"}]}}`),
		mk("revealed", `{"visible_if":{"all":[{"field":"driver","op":"eq","value":"yes"}]}}`),
	}

	visible, evalAnswers := VisiblePath(fields, map[string]any{
		"stage": "post", "driver": "yes", "gated": "x", "revealed": "y",
	})

	if visible["driver"] {
		t.Fatal("driver should be hidden — stage=post")
	}

	if !visible["gated"] {
		t.Fatal("gated should show — hidden driver's stale answer reads unanswered")
	}

	if visible["revealed"] {
		t.Fatal("revealed should stay hidden — a stale driver answer can't reveal it")
	}

	if _, ok := evalAnswers["driver"]; ok {
		t.Fatal("hidden driver's answer must not enter the eval map")
	}

	if _, ok := evalAnswers["gated"]; !ok {
		t.Fatal("visible answered field should enter the eval map")
	}

	// Chain through a VISIBLE driver still works: stage=pre shows driver,
	// its answer then gates deeper fields.
	visible, _ = VisiblePath(fields, map[string]any{"stage": "pre", "driver": "yes"})
	if !visible["driver"] || !visible["gated"] || !visible["revealed"] {
		t.Fatalf("visible driver should gate deeper fields: %v", visible)
	}
}

func TestHiddenByJumpVisibleSet(t *testing.T) {
	t.Parallel()

	fields := []RuleField{
		{Key: "j1", StepKey: "s1", Config: []byte(`{"jump":{"step":"s3"}}`)},
	}

	// Answered but ordered-hidden source cannot fire when the caller
	// supplies the VisiblePath result.
	hidden := HiddenByJump(fields, []string{"s1", "s2", "s3"}, map[string]any{"j1": "x"}, nil, map[string]bool{"j1": false})
	if len(hidden) != 0 {
		t.Fatalf("hidden source should not fire: %v", hidden)
	}
}

func TestHiddenByJump(t *testing.T) {
	t.Parallel()

	mk := func(key, step, cfg string) RuleField {
		return RuleField{Key: key, StepKey: step, Config: []byte(cfg)}
	}
	order := []string{"s1", "s2", "s3", "s4", "s5"}

	jump := func(step, val string) string {
		return `{"jump":{"when":{"any":[{"op":"eq","value":"` + val + `"}]},"step":"` + step + `"}}`
	}

	// Suppression set: j1 s1→s4 hides s2,s3 — j2 sits inside s2 and cannot
	// fire; j3's interval (s4→s5) is empty. Union set: j1 s1→s3 hides only
	// s2; j3 in still-visible s3 adds s4.
	suppress := []RuleField{
		mk("j1", "s1", jump("s4", "a")),
		mk("j2", "s2", jump("s5", "b")),
		mk("j3", "s4", jump("s5", "c")),
	}
	union := []RuleField{
		mk("j1", "s1", jump("s3", "a")),
		mk("j2", "s2", jump("s5", "b")),
		mk("j3", "s3", jump("s5", "c")),
	}
	answered := map[string]any{"j1": "a", "j2": "b", "j3": "c"}
	whenless := []RuleField{mk("u1", "s1", `{"jump":{"step":"s3"}}`)}

	cases := []struct {
		name    string
		fields  []RuleField
		answers map[string]any
		seed    map[string]bool
		want    map[string]bool
	}{
		{
			name: "forward jump hides open interval",
			fields: []RuleField{
				mk("f1", "s1", jump("s4", "go")),
				mk("f2", "s2", `{}`),
			},
			answers: map[string]any{"f1": "go"},
			want:    map[string]bool{"s2": true, "s3": true},
		},
		{
			name: "untriggered jump hides nothing",
			fields: []RuleField{
				mk("f1", "s1", jump("s4", "go")),
			},
			answers: map[string]any{"f1": "stop"},
			want:    map[string]bool{},
		},
		{
			name: "backward self and unknown targets ignored",
			fields: []RuleField{
				mk("f3", "s3", jump("s1", "1")),
				mk("f4", "s3", jump("s3", "1")),
				mk("f5", "s3", `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"ghost"}}`),
			},
			answers: map[string]any{"f3": 1.0, "f4": 1.0, "f5": 1.0},
			want:    map[string]bool{},
		},
		{
			name: "visible_if-hidden field cannot fire",
			fields: []RuleField{
				mk("f6", "s1", `{"visible_if":{"all":[{"field":"gate","op":"eq","value":0}]},"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}`),
			},
			answers: map[string]any{"f6": 1.0},
			want:    map[string]bool{},
		},
		{
			name:    "field in jump-hidden step is suppressed",
			fields:  suppress,
			answers: answered,
			want:    map[string]bool{"s2": true, "s3": true},
		},
		{
			name:    "intervals union across visible sources",
			fields:  union,
			answers: answered,
			want:    map[string]bool{"s2": true, "s4": true},
		},
		{
			name:    "rule-hidden step seed suppresses its fields",
			fields:  union,
			answers: answered,
			seed:    map[string]bool{"s3": true},
			want:    map[string]bool{"s2": true, "s3": true},
		},
		{
			name:    "when-less jump fires when field answered",
			fields:  whenless,
			answers: map[string]any{"u1": "x"},
			want:    map[string]bool{"s2": true},
		},
		{
			name:    "when-less jump silent when unanswered",
			fields:  whenless,
			answers: map[string]any{},
			want:    map[string]bool{},
		},
		{
			name:    "when-less jump silent on empty answer",
			fields:  whenless,
			answers: map[string]any{"u1": ""},
			want:    map[string]bool{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := HiddenByJump(c.fields, order, c.answers, c.seed, nil); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}
