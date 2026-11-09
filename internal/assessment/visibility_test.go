package assessment

import (
	"encoding/json"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/formlogic"
)

const (
	tfGate          = "gate"
	tfPurpose       = "purpose"
	tfStudy         = "study"
	tfSelect        = "select"
	tfDependents    = "dependents"
	tfWork          = "work"
	tfWorkVisa      = "work_visa"
	dimGeneral      = "general"
	dimPersonal     = "personal"
	dimProfessional = "professional"
)

func TestMissingRequiredSkipsHidden(t *testing.T) {
	t.Parallel()

	fields := []AssessmentField{
		{Key: tfGate, Required: true},
		{
			Key: "dep", Required: true,
			Config: FieldScoringConfig{VisibleIf: &formlogic.Condition{
				All: []formlogic.Clause{{Field: tfGate, Op: "eq", Value: "yes"}},
			}},
		},
	}
	vals, _ := json.Marshal(map[string]any{tfGate: "no"})

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
	t.Parallel()

	fields := []AssessmentField{
		{Key: tfPurpose, Required: true, Config: FieldScoringConfig{Dimension: dimGeneral}},
		{Key: tfWorkVisa, Required: true, Config: FieldScoringConfig{Dimension: dimProfessional}},
	}
	vals, _ := json.Marshal(map[string]any{tfPurpose: tfStudy})

	var answers map[string]any

	_ = json.Unmarshal(vals, &answers)
	rules := map[string]StepRule{
		dimProfessional: {VisibleIf: &formlogic.Condition{
			All: []formlogic.Clause{{Field: tfPurpose, Op: "eq", Value: "work"}},
		}},
	}

	vis := visibleFieldSet(fields, answers, rules)
	if vis[tfWorkVisa] {
		t.Fatal("field in a step_rules-hidden dimension should not be visible")
	}

	if got := missingRequiredFields(fields, vals, vis); len(got) != 0 {
		t.Fatalf("step-hidden required field blocked submit: %v", got)
	}
}

// TestMissingRequiredSkipsJumpHiddenDim verifies a required field inside a
// dimension covered by a triggered forward jump cannot block submission.
func TestMissingRequiredSkipsJumpHiddenDim(t *testing.T) {
	t.Parallel()

	fields := []AssessmentField{
		{Key: tfPurpose, Required: true, Config: FieldScoringConfig{
			Dimension: dimGeneral,
			Jump:      &formlogic.JumpRule{Step: dimPersonal, When: formlogic.Condition{All: []formlogic.Clause{{Op: "eq", Value: tfStudy}}}},
		}},
		{Key: tfWorkVisa, Required: true, Config: FieldScoringConfig{Dimension: dimProfessional}},
		{Key: tfDependents, Required: true, Config: FieldScoringConfig{Dimension: dimPersonal}},
	}
	vals, _ := json.Marshal(map[string]any{tfPurpose: tfStudy, tfDependents: "none"})

	var answers map[string]any

	_ = json.Unmarshal(vals, &answers)

	vis := visibleFieldSet(fields, answers, nil)
	if vis[tfWorkVisa] {
		t.Fatal("field in a jump-hidden dimension should not be visible")
	}

	if got := missingRequiredFields(fields, vals, vis); len(got) != 0 {
		t.Fatalf("jump-hidden required field blocked submit: %v", got)
	}
}

func TestComputeScoreSkipsHidden(t *testing.T) {
	t.Parallel()

	fields := []gen.CustomField{
		{Key: tfGate, FieldType: tfSelect, Config: []byte(`{"weight":1,"dimension":"d","option_scores":{"yes":100,"no":0}}`)},
		{Key: "dep", FieldType: tfSelect, Config: []byte(`{"weight":1,"dimension":"d","option_scores":{"x":100},"visible_if":{"all":[{"field":"gate","op":"eq","value":"yes"}]}}`)},
	}
	// gate=no hides dep; a stale dep=x answer must not score.
	vals, _ := json.Marshal(map[string]any{tfGate: "no", "dep": "x"})

	got := computeScore(vals, fields, ScoringConfig{})
	if got.DimensionScores["d"] != 0 {
		t.Fatalf("hidden field scored: %v", got.DimensionScores)
	}
}

// TestComputeScoreSkipsJumpHiddenDim verifies a stale answer for a field in a
// jump-hidden dimension is excluded from scoring (ADR-137).
func TestComputeScoreSkipsJumpHiddenDim(t *testing.T) {
	t.Parallel()

	fields := []gen.CustomField{
		{Key: tfPurpose, FieldType: tfSelect, Config: []byte(`{"weight":1,"dimension":"general","option_scores":{"study":0},"jump":{"when":{"all":[{"field":"purpose","op":"eq","value":"study"}]},"step":"personal"}}`)},
		{Key: tfWorkVisa, FieldType: tfSelect, Config: []byte(`{"weight":1,"dimension":"professional","option_scores":{"applied":100}}`)},
		{Key: tfDependents, FieldType: tfSelect, Config: []byte(`{"weight":1,"dimension":"personal","option_scores":{"none":50}}`)},
	}
	// purpose=study jumps general→personal, hiding professional; a stale
	// work_visa answer must not score.
	vals, _ := json.Marshal(map[string]any{tfPurpose: tfStudy, tfWorkVisa: "applied", tfDependents: "none"})

	got := computeScore(vals, fields, ScoringConfig{})
	if got.DimensionScores[dimProfessional] != 0 {
		t.Fatalf("jump-hidden dimension scored: %v", got.DimensionScores)
	}

	if got.DimensionScores[dimPersonal] == 0 {
		t.Fatal("jump target dimension should still score")
	}
}

func TestPublicFieldDTOsShipsRulesNotScores(t *testing.T) {
	t.Parallel()

	out := publicFieldDTOs([]AssessmentField{{
		ID: "1", Label: "Q", Key: "q", FieldType: tfSelect,
		Config: FieldScoringConfig{
			Weight: 1, Dimension: "d", OptionScores: map[string]int{"a": 5},
			VisibleIf: &formlogic.Condition{All: []formlogic.Clause{{Field: "g", Op: "answered"}}},
		},
	}})

	cfg, ok := out[0]["config"].(map[string]any)
	if !ok {
		t.Fatal("config missing from public field DTO")
	}

	if _, leak := cfg["option_scores"]; leak {
		t.Fatal("option_scores leaked to public payload")
	}

	if _, ok := cfg["visible_if"]; !ok {
		t.Fatal("visible_if missing from public payload")
	}
}
