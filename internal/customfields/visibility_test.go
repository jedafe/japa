package customfields

import (
	"testing"
)

const (
	fldPurpose         = "purpose"
	fldPurposeLabel    = "Purpose"
	fldAdmissionStatus = "admission_status"
	fldSelectFieldType = "select"
)

// TestDropHiddenFieldValues verifies a field hidden by visible_if neither
// blocks submission as required nor survives in the stored values.
func TestDropHiddenFieldValues(t *testing.T) {
	t.Parallel()

	fields := []fieldDef{
		{Label: fldPurposeLabel, Key: fldPurpose, FieldType: fldSelectFieldType, IsRequired: true},
		{
			Label: "Admission", Key: fldAdmissionStatus, FieldType: fldSelectFieldType, IsRequired: true,
			Config: []byte(`{"visible_if":{"all":[{"field":"purpose","op":"eq","value":"study"}]}}`),
		},
	}

	got := dropHiddenValues(fields, nil, map[string]any{fldPurpose: "work"})
	if _, ok := got[fldAdmissionStatus]; ok {
		t.Fatal("hidden field value should be dropped")
	}

	if _, ok := got[fldPurpose]; !ok {
		t.Fatal("visible field value must stay")
	}
}

// TestDropHiddenFieldValuesStepRule verifies a field whose containing step
// is hidden by the step's own visible_if is treated as hidden: its stale
// answer is dropped and it cannot block the required check.
func TestDropHiddenFieldValuesStepRule(t *testing.T) {
	t.Parallel()

	fields := []fieldDef{
		{Label: fldPurposeLabel, Key: fldPurpose, FieldType: fldSelectFieldType, IsRequired: true, StepKey: "s1"},
		{Label: "Admission", Key: fldAdmissionStatus, FieldType: fldSelectFieldType, IsRequired: true, StepKey: "s2"},
	}
	steps := []stepDef{
		{ID: "s1"},
		{ID: "s2", Config: []byte(`{"visible_if":{"all":[{"field":"purpose","op":"eq","value":"study"}]}}`)},
	}

	got := dropHiddenValues(fields, steps, map[string]any{fldPurpose: "work", fldAdmissionStatus: "applied"})
	if _, ok := got[fldAdmissionStatus]; ok {
		t.Fatal("field in a step-hidden step should be dropped")
	}

	if _, ok := got[fldPurpose]; !ok {
		t.Fatal("visible field value must stay")
	}
}

// TestDropHiddenFieldValuesJumpHiddenStep verifies a field inside a step
// covered by a triggered forward-jump interval is treated as hidden.
func TestDropHiddenFieldValuesJumpHiddenStep(t *testing.T) {
	t.Parallel()

	fields := []fieldDef{
		{
			Label: fldPurposeLabel, Key: fldPurpose, FieldType: fldSelectFieldType, IsRequired: true, StepKey: "s1",
			Config: []byte(`{"jump":{"when":{"all":[{"field":"purpose","op":"eq","value":"study"}]},"step":"s3"}}`),
		},
		{Label: "Work visa", Key: "work_visa", FieldType: fldSelectFieldType, IsRequired: true, StepKey: "s2"},
		{Label: "Admission", Key: fldAdmissionStatus, FieldType: fldSelectFieldType, IsRequired: true, StepKey: "s3"},
	}
	steps := []stepDef{{ID: "s1"}, {ID: "s2"}, {ID: "s3"}}

	got := dropHiddenValues(fields, steps, map[string]any{
		fldPurpose: "study", "work_visa": "applied", fldAdmissionStatus: "admitted",
	})
	if _, ok := got["work_visa"]; ok {
		t.Fatal("field in a jump-skipped step should be dropped")
	}

	if _, ok := got[fldAdmissionStatus]; !ok {
		t.Fatal("jump target step's fields must stay")
	}
}

// TestDropHiddenFieldValuesUngroupedOnMultiStep verifies a field with no
// step on a multi-step form is treated as hidden — the renderer shows only
// steps.flatMap, so requiring it would deadlock every submission. The same
// field on a single-step form (no stepDefs) stays on the visible path.
func TestDropHiddenFieldValuesUngroupedOnMultiStep(t *testing.T) {
	t.Parallel()

	fields := []fieldDef{
		{Label: "Ungrouped", Key: "ungrouped", FieldType: "text", IsRequired: true},
		{Label: "Grouped", Key: "grouped", FieldType: "text", IsRequired: true, StepKey: "s1"},
	}
	steps := []stepDef{{ID: "s1"}}

	got := dropHiddenValues(fields, steps, map[string]any{"ungrouped": "x", "grouped": "y"})
	if _, ok := got["ungrouped"]; ok {
		t.Fatal("step-less field on a multi-step form should be dropped as unreachable")
	}

	if _, ok := got["grouped"]; !ok {
		t.Fatal("stepped field value must stay")
	}

	if !hiddenFieldKeys(fields, steps, map[string]any{})["ungrouped"] {
		t.Fatal("step-less field on a multi-step form should be hidden, not required")
	}

	got = dropHiddenValues(fields, nil, map[string]any{"ungrouped": "x"})
	if _, ok := got["ungrouped"]; !ok {
		t.Fatal("same field with no steps at all is on the visible path")
	}
}

// TestCanonicalAnswersNormalizes verifies label-keyed submissions map onto
// field keys and select answers normalize labels to canonical option values.
func TestCanonicalAnswersNormalizes(t *testing.T) {
	t.Parallel()

	fields := []fieldDef{
		{
			Label: fldPurposeLabel, Key: fldPurpose, FieldType: fldSelectFieldType,
			Options: []byte(`[{"value":"study","label":"Study Abroad"},{"value":"work","label":"Work"}]`),
		},
		{
			Label: "Pets", Key: "pets", FieldType: "multiselect",
			Options: []byte(`[{"value":"dog","label":"Dog"},{"value":"cat","label":"Cat"}]`),
		},
	}

	canon := canonicalAnswers(fields, map[string]any{
		fldPurposeLabel: "Work",
		"pets":          "Dog,Cat",
	})

	if canon[fldPurpose] != "work" {
		t.Fatalf("label-keyed + label-valued answer should canonicalize to 'work', got %v", canon[fldPurpose])
	}

	arr, ok := canon["pets"].([]any)
	if !ok || len(arr) != 2 || arr[0] != "dog" || arr[1] != "cat" {
		t.Fatalf("multiselect should normalize to value array, got %v", canon["pets"])
	}
}
