package assessment

import (
	"encoding/json"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/formlogic"
)

func TestMissingRequiredSkipsHidden(t *testing.T) {
	fields := []AssessmentField{
		{Key: "gate", Required: true},
		{Key: "dep", Required: true,
			Config: FieldScoringConfig{VisibleIf: &formlogic.Condition{
				All: []formlogic.Clause{{Field: "gate", Op: "eq", Value: "yes"}},
			}}},
	}
	vals, _ := json.Marshal(map[string]any{"gate": "no"})
	var answers map[string]any
	_ = json.Unmarshal(vals, &answers)

	vis := visibleFieldSet(fields, answers, nil)
	if got := missingRequiredFields(fields, vals, vis); len(got) != 0 {
		t.Fatalf("hidden required field blocked submit: %v", got)
	}
}

// TestMissingRequiredSkipsStepRuleHidden verifies a required field inside a
// step_rules-hidden dimension cannot block submission (ADR-137).
func TestMissingRequiredSkipsStepRuleHidden(t *testing.T) {
	fields := []AssessmentField{
		{Key: "purpose", Required: true, Config: FieldScoringConfig{Dimension: "general"}},
		{Key: "work_visa", Required: true, Config: FieldScoringConfig{Dimension: "professional"}},
	}
	vals, _ := json.Marshal(map[string]any{"purpose": "study"})
	var answers map[string]any
	_ = json.Unmarshal(vals, &answers)
	rules := map[string]StepRule{
		"professional": {VisibleIf: &formlogic.Condition{
			All: []formlogic.Clause{{Field: "purpose", Op: "eq", Value: "work"}},
		}},
	}

	vis := visibleFieldSet(fields, answers, rules)
	if vis["work_visa"] {
		t.Fatal("field in a step_rules-hidden dimension should not be visible")
	}
	if got := missingRequiredFields(fields, vals, vis); len(got) != 0 {
		t.Fatalf("step-hidden required field blocked submit: %v", got)
	}
}

// TestMissingRequiredSkipsJumpHiddenDim verifies a required field inside a
// dimension covered by a triggered forward jump cannot block submission.
func TestMissingRequiredSkipsJumpHiddenDim(t *testing.T) {
	fields := []AssessmentField{
		{Key: "purpose", Required: true, Config: FieldScoringConfig{
			Dimension: "general",
			Jump:      &formlogic.JumpRule{Step: "personal", When: formlogic.Condition{All: []formlogic.Clause{{Op: "eq", Value: "study"}}}},
		}},
		{Key: "work_visa", Required: true, Config: FieldScoringConfig{Dimension: "professional"}},
		{Key: "dependents", Required: true, Config: FieldScoringConfig{Dimension: "personal"}},
	}
	vals, _ := json.Marshal(map[string]any{"purpose": "study", "dependents": "none"})
	var answers map[string]any
	_ = json.Unmarshal(vals, &answers)

	vis := visibleFieldSet(fields, answers, nil)
	if vis["work_visa"] {
		t.Fatal("field in a jump-hidden dimension should not be visible")
	}
	if got := missingRequiredFields(fields, vals, vis); len(got) != 0 {
		t.Fatalf("jump-hidden required field blocked submit: %v", got)
	}
}

func TestComputeScoreSkipsHidden(t *testing.T) {
	fields := []gen.CustomField{
		{Key: "gate", FieldType: "select", Config: []byte(`{"weight":1,"dimension":"d","option_scores":{"yes":100,"no":0}}`)},
		{Key: "dep", FieldType: "select", Config: []byte(`{"weight":1,"dimension":"d","option_scores":{"x":100},"visible_if":{"all":[{"field":"gate","op":"eq","value":"yes"}]}}`)},
	}
	// gate=no hides dep; a stale dep=x answer must not score.
	vals, _ := json.Marshal(map[string]any{"gate": "no", "dep": "x"})
	got := computeScore(vals, fields, ScoringConfig{})
	if got.DimensionScores["d"] != 0 {
		t.Fatalf("hidden field scored: %v", got.DimensionScores)
	}
}

// TestComputeScoreSkipsJumpHiddenDim verifies a stale answer for a field in a
// jump-hidden dimension is excluded from scoring (ADR-137).
func TestComputeScoreSkipsJumpHiddenDim(t *testing.T) {
	fields := []gen.CustomField{
		{Key: "purpose", FieldType: "select", Config: []byte(`{"weight":1,"dimension":"general","option_scores":{"study":0},"jump":{"when":{"all":[{"field":"purpose","op":"eq","value":"study"}]},"step":"personal"}}`)},
		{Key: "work_visa", FieldType: "select", Config: []byte(`{"weight":1,"dimension":"professional","option_scores":{"applied":100}}`)},
		{Key: "dependents", FieldType: "select", Config: []byte(`{"weight":1,"dimension":"personal","option_scores":{"none":50}}`)},
	}
	// purpose=study jumps general→personal, hiding professional; a stale
	// work_visa answer must not score.
	vals, _ := json.Marshal(map[string]any{"purpose": "study", "work_visa": "applied", "dependents": "none"})
	got := computeScore(vals, fields, ScoringConfig{})
	if got.DimensionScores["professional"] != 0 {
		t.Fatalf("jump-hidden dimension scored: %v", got.DimensionScores)
	}
	if got.DimensionScores["personal"] == 0 {
		t.Fatal("jump target dimension should still score")
	}
}

func TestPublicFieldDTOsShipsRulesNotScores(t *testing.T) {
	out := publicFieldDTOs([]AssessmentField{{
		ID: "1", Label: "Q", Key: "q", FieldType: "select",
		Config: FieldScoringConfig{
			Weight: 1, Dimension: "d", OptionScores: map[string]int{"a": 5},
			VisibleIf: &formlogic.Condition{All: []formlogic.Clause{{Field: "g", Op: "answered"}}},
		},
	}})
	cfg := out[0]["config"].(map[string]any)
	if _, leak := cfg["option_scores"]; leak {
		t.Fatal("option_scores leaked to public payload")
	}
	if _, ok := cfg["visible_if"]; !ok {
		t.Fatal("visible_if missing from public payload")
	}
}
