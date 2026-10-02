package wiki

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/customfields"
)

const testEntityID = "00000000-0000-0000-0000-0000000000aa"

func validSetValueOp() FieldOp {
	return FieldOp{
		Op:         OpSetValue,
		EntityType: "university",
		EntityID:   testEntityID,
		FieldKey:   "tuition_ug",
		Value:      json.RawMessage(`12000`),
	}
}

func validFieldDef() *FieldDefInput {
	return &FieldDefInput{
		Key:       "visa_success_rate",
		Label:     "Visa Success Rate",
		FieldType: customfields.FieldTypePercentage,
	}
}

// TestValidateOps_RejectsUnknownOp ensures op names outside the §5
// vocabulary are rejected before any semantic checks run.
func TestValidateOps_RejectsUnknownOp(t *testing.T) {
	cases := []struct {
		name string
		op   string
	}{
		{"empty op name", ""},
		{"misspelled op", "setvalue"},
		{"nonexistent op", "drop_table"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOps([]FieldOp{{Op: tc.op}}, 0)
			if err == nil {
				t.Fatalf("expected error for op %q", tc.op)
			}

			if !strings.Contains(err.Error(), "unknown op") {
				t.Fatalf("error should mention unknown op, got %v", err)
			}
		})
	}
}

// TestValidateOps_RejectsMissingEntityTargetOnSetValue covers every missing
// or malformed member a set_value op depends on.
func TestValidateOps_RejectsMissingEntityTargetOnSetValue(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*FieldOp)
		wantErr string
	}{
		{"missing entity_type", func(o *FieldOp) { o.EntityType = "" }, "entity_type"},
		{"missing entity_id", func(o *FieldOp) { o.EntityID = "" }, "entity_id"},
		{"entity_id not a UUID", func(o *FieldOp) { o.EntityID = "not-a-uuid" }, "entity_id"},
		{"missing field_key", func(o *FieldOp) { o.FieldKey = "" }, "field_key"},
		{"missing value", func(o *FieldOp) { o.Value = nil }, "value"},
		{"empty value", func(o *FieldOp) { o.Value = json.RawMessage(`  `) }, "value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := validSetValueOp()
			tc.mutate(&op)

			err := ValidateOps([]FieldOp{op}, 0)
			if err == nil {
				t.Fatal("expected validation error")
			}

			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v should mention %q", err, tc.wantErr)
			}
		})
	}

	t.Run("valid set_value passes", func(t *testing.T) {
		if err := ValidateOps([]FieldOp{validSetValueOp()}, 0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestValidateOps_BoundsMaxOps verifies the ops-count bound and that
// maxOps <= 0 disables it.
func TestValidateOps_BoundsMaxOps(t *testing.T) {
	ops := []FieldOp{validSetValueOp(), validSetValueOp(), validSetValueOp()}

	if err := ValidateOps(ops, 2); err == nil {
		t.Fatal("expected error when ops exceed maxOps")
	}

	if err := ValidateOps(ops, 3); err != nil {
		t.Fatalf("ops within bound should pass: %v", err)
	}

	if err := ValidateOps(ops, 0); err != nil {
		t.Fatalf("maxOps=0 (unlimited) should pass: %v", err)
	}

	if err := ValidateOps(ops, -1); err != nil {
		t.Fatalf("negative maxOps (unlimited) should pass: %v", err)
	}
}

// TestValidateOps_RejectsBadFieldTypeOnPropose covers field-type and
// options validation across propose_field, upsert_field, and
// propose_group's nested fields.
func TestValidateOps_RejectsBadFieldTypeOnPropose(t *testing.T) {
	cases := []struct {
		name    string
		op      FieldOp
		wantErr string
	}{
		{
			name: "propose_field unknown field_type",
			op: FieldOp{
				Op:        OpProposeField,
				GroupSlug: "university_fields",
				Field: &FieldDefInput{
					Key:       "mystery",
					Label:     "Mystery",
					FieldType: "definitely_not_a_type",
				},
			},
			wantErr: "field_type",
		},
		{
			name: "propose_field missing field payload",
			op: FieldOp{
				Op:        OpProposeField,
				GroupSlug: "university_fields",
			},
			wantErr: "field",
		},
		{
			name: "propose_field missing group_slug",
			op: FieldOp{
				Op:    OpProposeField,
				Field: validFieldDef(),
			},
			wantErr: "group_slug",
		},
		{
			name: "propose_field select without options",
			op: FieldOp{
				Op:        OpProposeField,
				GroupSlug: "university_fields",
				Field: &FieldDefInput{
					Key:       "intake",
					Label:     "Intake",
					FieldType: customfields.FieldTypeSelect,
				},
			},
			wantErr: "option",
		},
		{
			name: "propose_field select option missing label",
			op: FieldOp{
				Op:        OpProposeField,
				GroupSlug: "university_fields",
				Field: &FieldDefInput{
					Key:       "intake",
					Label:     "Intake",
					FieldType: customfields.FieldTypeSelect,
					Options:   json.RawMessage(`[{"value":"fall","label":""}]`),
				},
			},
			wantErr: "value and label",
		},
		{
			name: "upsert_field bad field_type",
			op: FieldOp{
				Op:        OpUpsertField,
				GroupSlug: "university_fields",
				Field: &FieldDefInput{
					Key:       "x",
					Label:     "X",
					FieldType: "bogus",
				},
			},
			wantErr: "field_type",
		},
		{
			name: "propose_group nested field bad type",
			op: FieldOp{
				Op:          OpProposeGroup,
				EntityClass: "space_type:university",
				Group: &GroupDefInput{
					Slug: "visa_outcomes",
					Name: "Visa Outcomes",
					Fields: []FieldDefInput{
						{Key: "ok", Label: "OK", FieldType: customfields.FieldTypeText},
						{Key: "bad", Label: "Bad", FieldType: "nope"},
					},
				},
			},
			wantErr: "group.fields[1]",
		},
		{
			name: "propose_group missing entity_class",
			op: FieldOp{
				Op: OpProposeGroup,
				Group: &GroupDefInput{
					Slug: "visa_outcomes",
					Name: "Visa Outcomes",
				},
			},
			wantErr: "entity_class",
		},
		{
			name: "propose_group missing group",
			op: FieldOp{
				Op:          OpProposeGroup,
				EntityClass: "space_type:university",
			},
			wantErr: "group",
		},
		{
			name: "assign_group missing entity_class",
			op: FieldOp{
				Op:        OpAssignGroup,
				GroupSlug: "visa_outcomes",
			},
			wantErr: "entity_class",
		},
		{
			name: "assign_group missing group_slug",
			op: FieldOp{
				Op:          OpAssignGroup,
				EntityClass: "space_type:university",
			},
			wantErr: "group_slug",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOps([]FieldOp{tc.op}, 0)
			if err == nil {
				t.Fatal("expected validation error")
			}

			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v should mention %q", err, tc.wantErr)
			}
		})
	}

	valid := []FieldOp{
		{Op: OpProposeField, GroupSlug: "university_fields", Field: validFieldDef()},
		{Op: OpUpsertField, GroupSlug: "university_fields", Field: validFieldDef()},
		{
			Op:          OpProposeGroup,
			EntityClass: "space_type:university",
			Group: &GroupDefInput{
				Slug: "visa_outcomes",
				Name: "Visa Outcomes",
				Fields: []FieldDefInput{
					{
						Key:       "intake",
						Label:     "Intake",
						FieldType: customfields.FieldTypeSelect,
						Options:   json.RawMessage(`[{"value":"fall","label":"Fall"}]`),
					},
				},
			},
		},
		{Op: OpAssignGroup, EntityClass: "space_type:university", GroupSlug: "visa_outcomes"},
	}

	if err := ValidateOps(valid, 0); err != nil {
		t.Fatalf("valid ops array rejected: %v", err)
	}
}

// TestValidateOps_AcceptsSelfReferentialOps confirms a set_value op may
// target a field_key that a propose_field earlier in the same ops array
// introduces. Structural validation does not resolve cross-op references —
// that semantic check belongs to the service layer at submit/apply time.
func TestValidateOps_AcceptsSelfReferentialOps(t *testing.T) {
	ops := []FieldOp{
		{
			Op:        OpProposeField,
			GroupSlug: "university_fields",
			Field: &FieldDefInput{
				Key:       "visa_success_rate",
				Label:     "Visa Success Rate",
				FieldType: customfields.FieldTypePercentage,
			},
		},
		{
			Op:         OpSetValue,
			EntityType: "university",
			EntityID:   testEntityID,
			FieldKey:   "visa_success_rate",
			Value:      json.RawMessage(`87.5`),
		},
	}

	if err := ValidateOps(ops, 0); err != nil {
		t.Fatalf("self-referential ops rejected: %v", err)
	}
}

// TestDeriveKind covers all three revision kinds.
func TestDeriveKind(t *testing.T) {
	ops := []FieldOp{validSetValueOp()}

	cases := []struct {
		name       string
		hasContent bool
		ops        []FieldOp
		want       string
	}{
		{"content only", true, nil, "content"},
		{"ops only", false, ops, "fields"},
		{"content and ops", true, ops, "mixed"},
		{"neither content nor ops", false, nil, "content"},
		{"empty ops slice", true, []FieldOp{}, "content"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveKind(tc.hasContent, tc.ops); got != tc.want {
				t.Fatalf("DeriveKind(%v, %d ops) = %q, want %q", tc.hasContent, len(tc.ops), got, tc.want)
			}
		})
	}
}

// TestParseOps covers nil/empty/malformed/valid payloads.
func TestParseOps(t *testing.T) {
	t.Run("nil payload", func(t *testing.T) {
		ops, err := ParseOps(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ops == nil || len(ops) != 0 {
			t.Fatalf("expected empty non-nil slice, got %#v", ops)
		}
	})

	t.Run("empty payload", func(t *testing.T) {
		ops, err := ParseOps([]byte{})
		if err != nil || len(ops) != 0 {
			t.Fatalf("expected empty slice no error, got ops=%v err=%v", ops, err)
		}
	})

	t.Run("whitespace payload", func(t *testing.T) {
		ops, err := ParseOps([]byte("  \n\t "))
		if err != nil || len(ops) != 0 {
			t.Fatalf("expected empty slice no error, got ops=%v err=%v", ops, err)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		ops, err := ParseOps([]byte(`[]`))
		if err != nil || ops == nil || len(ops) != 0 {
			t.Fatalf("expected empty non-nil slice no error, got ops=%v err=%v", ops, err)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		if _, err := ParseOps([]byte(`[{"op":`)); err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("non-array JSON", func(t *testing.T) {
		if _, err := ParseOps([]byte(`{"op":"set_value"}`)); err == nil {
			t.Fatal("expected error for non-array payload")
		}
	})

	t.Run("valid payload", func(t *testing.T) {
		raw := `[{"op":"set_value","entity_type":"university","entity_id":"` + testEntityID + `","field_key":"tuition_ug","value":12000,"source_url":"https://example.com/fees"}]`

		ops, err := ParseOps([]byte(raw))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ops) != 1 {
			t.Fatalf("expected 1 op, got %d", len(ops))
		}

		op := ops[0]
		if op.Op != OpSetValue || op.EntityType != "university" || op.EntityID != testEntityID || op.FieldKey != "tuition_ug" {
			t.Fatalf("op decoded wrong: %+v", op)
		}

		if string(op.Value) != "12000" {
			t.Fatalf("value: got %s want 12000", op.Value)
		}

		if op.SourceURL != "https://example.com/fees" {
			t.Fatalf("source_url: got %q", op.SourceURL)
		}
	})

	t.Run("valid payload with field and group defs", func(t *testing.T) {
		raw := `[
			{"op":"propose_group","entity_class":"space_type:university","group":{"slug":"visa_outcomes","name":"Visa Outcomes","fields":[{"key":"intake","label":"Intake","field_type":"select","options":[{"value":"fall","label":"Fall"}]}]}},
			{"op":"propose_field","group_slug":"university_fields","field":{"key":"visa_success_rate","label":"Visa Success Rate","field_type":"percentage"}}
		]`

		ops, err := ParseOps([]byte(raw))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ops) != 2 {
			t.Fatalf("expected 2 ops, got %d", len(ops))
		}

		if ops[0].Group == nil || ops[0].Group.Slug != "visa_outcomes" || len(ops[0].Group.Fields) != 1 {
			t.Fatalf("group def decoded wrong: %+v", ops[0].Group)
		}

		if ops[1].Field == nil || ops[1].Field.Key != "visa_success_rate" || ops[1].Field.FieldType != "percentage" {
			t.Fatalf("field def decoded wrong: %+v", ops[1].Field)
		}

		if err := ValidateOps(ops, 0); err != nil {
			t.Fatalf("parsed ops failed validation: %v", err)
		}
	})
}
