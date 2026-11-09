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

	vis := visibleFieldSet(fields, answers)
	if got := missingRequiredFields(fields, vals, vis); len(got) != 0 {
		t.Fatalf("hidden required field blocked submit: %v", got)
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
