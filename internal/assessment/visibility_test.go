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
	tfVisaType      = "visa_type"
	tfStudent       = "student"
	tfBusiness      = "business"
	tfJourneyStage  = "journey_stage"
	tfPlanning      = "planning"
	tfFamily        = "family"
	tfInvestment    = "investment"
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

// TestVisibleFieldSetStaleDriverAnswer verifies a hidden driver's retained
// answer is off the visible path: journey_stage=relocated hides purpose, so
// a stale purpose answer must read unanswered — visa_type stays visible
// (its not_answered clause opens) and its visible business answer drives
// business_readiness legitimately, while a stale purpose=business alone
// cannot reveal it — matching the live readiness gates (ADR-137).
func TestVisibleFieldSetStaleDriverAnswer(t *testing.T) {
	t.Parallel()

	fields := []AssessmentField{
		{Key: tfJourneyStage, Required: true},
		{
			Key: tfPurpose,
			Config: FieldScoringConfig{VisibleIf: &formlogic.Condition{
				Any: []formlogic.Clause{
					{Field: tfJourneyStage, Op: "not_answered"},
					{Field: tfJourneyStage, Op: "in", Value: []any{"considering", tfPlanning, "preparing", "applied", "approved"}},
				},
			}},
		},
		{
			Key: tfVisaType,
			Config: FieldScoringConfig{VisibleIf: &formlogic.Condition{
				Any: []formlogic.Clause{
					{Field: tfPurpose, Op: "not_answered"},
					{Field: tfPurpose, Op: "in", Value: []any{"relocation", tfFamily, "vacation", "other"}},
				},
			}},
		},
		{
			Key: "business_readiness",
			Config: FieldScoringConfig{VisibleIf: &formlogic.Condition{
				Any: []formlogic.Clause{
					{Field: tfPurpose, Op: "eq", Value: tfBusiness},
					{Field: tfVisaType, Op: "in", Value: []any{tfBusiness, tfInvestment}},
				},
			}},
		},
	}

	// Draft carried a pre-move purpose=work; the user then marked relocated
	// and answered visa_type=business. purpose is hidden so its stale answer
	// reads unanswered — visa_type shows and drives business_readiness.
	vis := visibleFieldSet(fields, map[string]any{
		tfJourneyStage: "relocated", tfPurpose: tfWork, tfVisaType: tfBusiness,
	}, nil)

	if vis[tfPurpose] {
		t.Fatal("purpose should be hidden at relocated")
	}

	if !vis[tfVisaType] {
		t.Fatal("visa_type should be visible — hidden purpose reads unanswered, restoring the backfill path")
	}

	if !vis["business_readiness"] {
		t.Fatal("business_readiness should show via the visible visa_type=business answer")
	}

	// Same relocation but a stale purpose=business and no visa_type answer:
	// the hidden purpose answer must not reveal business_readiness.
	vis = visibleFieldSet(fields, map[string]any{
		tfJourneyStage: "relocated", tfPurpose: tfBusiness,
	}, nil)

	if vis["business_readiness"] {
		t.Fatal("business_readiness must hide — a hidden driver's stale answer can't reveal it")
	}

	if !vis[tfVisaType] {
		t.Fatal("visa_type should still be visible — purpose reads unanswered")
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
