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
	got := dropHiddenValues(fields, map[string]any{"purpose": "work"})
	if _, ok := got["admission_status"]; ok {
		t.Fatal("hidden field value should be dropped")
	}
	if _, ok := got["purpose"]; !ok {
		t.Fatal("visible field value must stay")
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
