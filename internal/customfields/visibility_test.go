package customfields

import (
	"testing"
)

// TestDropHiddenFieldValues verifies a field hidden by visible_if neither
// blocks submission as required nor survives in the stored values.
func TestDropHiddenFieldValues(t *testing.T) {
	fields := []fieldDef{
		{Label: "Purpose", Key: "purpose", FieldType: "select", IsRequired: true},
		{Label: "Admission", Key: "admission_status", FieldType: "select", IsRequired: true,
			Config: []byte(`{"visible_if":{"all":[{"field":"purpose","op":"eq","value":"study"}]}}`)},
	}
	got := dropHiddenValues(fields, nil, map[string]any{"purpose": "work"})
	if _, ok := got["admission_status"]; ok {
		t.Fatal("hidden field value should be dropped")
	}
	if _, ok := got["purpose"]; !ok {
		t.Fatal("visible field value must stay")
	}
}

// TestDropHiddenFieldValuesStepRule verifies a field whose containing step
// is hidden by the step's own visible_if is treated as hidden: its stale
// answer is dropped and it cannot block the required check.
func TestDropHiddenFieldValuesStepRule(t *testing.T) {
	fields := []fieldDef{
		{Label: "Purpose", Key: "purpose", FieldType: "select", IsRequired: true},
		{Label: "Admission", Key: "admission_status", FieldType: "select", IsRequired: true, StepKey: "s2"},
	}
	steps := []stepDef{
		{ID: "s1"},
		{ID: "s2", Config: []byte(`{"visible_if":{"all":[{"field":"purpose","op":"eq","value":"study"}]}}`)},
	}
	got := dropHiddenValues(fields, steps, map[string]any{"purpose": "work", "admission_status": "applied"})
	if _, ok := got["admission_status"]; ok {
		t.Fatal("field in a step-hidden step should be dropped")
	}
	if _, ok := got["purpose"]; !ok {
		t.Fatal("visible field value must stay")
	}
}

// TestDropHiddenFieldValuesJumpHiddenStep verifies a field inside a step
// covered by a triggered forward-jump interval is treated as hidden.
func TestDropHiddenFieldValuesJumpHiddenStep(t *testing.T) {
	fields := []fieldDef{
		{Label: "Purpose", Key: "purpose", FieldType: "select", IsRequired: true, StepKey: "s1",
			Config: []byte(`{"jump":{"when":{"all":[{"field":"purpose","op":"eq","value":"study"}]},"step":"s3"}}`)},
		{Label: "Work visa", Key: "work_visa", FieldType: "select", IsRequired: true, StepKey: "s2"},
		{Label: "Admission", Key: "admission_status", FieldType: "select", IsRequired: true, StepKey: "s3"},
	}
	steps := []stepDef{{ID: "s1"}, {ID: "s2"}, {ID: "s3"}}
	got := dropHiddenValues(fields, steps, map[string]any{
		"purpose": "study", "work_visa": "applied", "admission_status": "admitted",
	})
	if _, ok := got["work_visa"]; ok {
		t.Fatal("field in a jump-skipped step should be dropped")
	}
	if _, ok := got["admission_status"]; !ok {
		t.Fatal("jump target step's fields must stay")
	}
}

// TestCanonicalAnswersNormalizes verifies label-keyed submissions map onto
// field keys and select answers normalize labels to canonical option values.
func TestCanonicalAnswersNormalizes(t *testing.T) {
	fields := []fieldDef{
		{Label: "Purpose", Key: "purpose", FieldType: "select",
			Options: []byte(`[{"value":"study","label":"Study Abroad"},{"value":"work","label":"Work"}]`)},
		{Label: "Pets", Key: "pets", FieldType: "multiselect",
			Options: []byte(`[{"value":"dog","label":"Dog"},{"value":"cat","label":"Cat"}]`)},
	}

	canon := canonicalAnswers(fields, map[string]any{
		"Purpose": "Work",
		"pets":    "Dog,Cat",
	})

	if canon["purpose"] != "work" {
		t.Fatalf("label-keyed + label-valued answer should canonicalize to 'work', got %v", canon["purpose"])
	}
	arr, ok := canon["pets"].([]any)
	if !ok || len(arr) != 2 || arr[0] != "dog" || arr[1] != "cat" {
		t.Fatalf("multiselect should normalize to value array, got %v", canon["pets"])
	}
}
