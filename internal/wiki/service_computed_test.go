package wiki

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

func entityVal(fieldID, groupID pgtype.UUID, key, fieldType string, num pgtype.Numeric, jsn []byte) gen.ListCustomFieldValuesWithDefsByEntityRow {
	return gen.ListCustomFieldValuesWithDefsByEntityRow{
		FieldID: fieldID, FieldGroupID: groupID, FieldKey: key, FieldType: fieldType,
		ValueNumber: num, ValueJson: jsn,
	}
}

func computedDTO(t *testing.T, key, fieldType string, sources string) EntityFieldDTO {
	t.Helper()
	return EntityFieldDTO{
		ID: pgid(0xF0).String(), Key: key, Label: key, FieldType: fieldType,
		Options: json.RawMessage(`{"computed":{"metric":"avg","sources":` + sources + `}}`),
		Visible: true,
		CanEdit: true,
	}
}

func TestApplyComputedFields_Avg(t *testing.T) {
	groupA, groupB := pgid(0xA0), pgid(0xB0)
	srcField := pgid(0xA1)

	ff := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{
			groupA.String(): {ID: groupA, Slug: "g_food"},
			groupB.String(): {ID: groupB, Slug: "g_transport"},
		},
	}
	s := &Service{fields: ff}

	dtos := []EntityFieldDTO{
		computedDTO(t, "exp_food", customfields.FieldTypeCurrency,
			`[{"group":"g_food","key":"monthly_total"},{"group":"g_transport","key":"monthly_total"}]`),
	}
	values := []gen.ListCustomFieldValuesWithDefsByEntityRow{
		entityVal(srcField, groupA, "monthly_total", customfields.FieldTypeCurrency, numericVal(t, "300"), nil),
		entityVal(pgid(0xB1), groupB, "monthly_total", customfields.FieldTypeCurrency, numericVal(t, "500"), nil),
	}
	visible := map[string]bool{srcField.String(): true, pgid(0xB1).String(): true}

	s.applyComputedFields(context.Background(), dtos, values, visible, false)

	var got float64
	if err := json.Unmarshal(dtos[0].Value, &got); err != nil || got != 400 {
		t.Fatalf("value: got %s err %v", dtos[0].Value, err)
	}
	if !dtos[0].Computed || dtos[0].CanEdit {
		t.Fatalf("computed/can_edit flags wrong: %+v", dtos[0])
	}
}

func TestApplyComputedFields_NoSourcesEmpty(t *testing.T) {
	s := &Service{fields: &fakeFieldReader{}}
	dtos := []EntityFieldDTO{
		computedDTO(t, "exp_food", customfields.FieldTypeCurrency, `[{"group":"nope","key":"k"}]`),
	}
	dtos[0].Value = json.RawMessage(`999`) // stored manual value — must not leak through
	s.applyComputedFields(context.Background(), dtos, nil, map[string]bool{}, false)
	if dtos[0].Value != nil {
		t.Fatalf("stored value must not survive a computed field: %s", dtos[0].Value)
	}
	if !dtos[0].Computed {
		t.Fatal("still computed even with zero sources")
	}
}

func TestApplyComputedFields_SourceVisibility(t *testing.T) {
	groupA := pgid(0xA0)
	hidden, shown := pgid(0xA1), pgid(0xA2)
	ff := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{groupA.String(): {ID: groupA, Slug: "g"}},
	}
	s := &Service{fields: ff}

	dtos := []EntityFieldDTO{
		computedDTO(t, "f", customfields.FieldTypeNumber, `[{"group":"g","key":"a"},{"group":"g","key":"b"}]`),
	}
	values := []gen.ListCustomFieldValuesWithDefsByEntityRow{
		entityVal(hidden, groupA, "a", customfields.FieldTypeNumber, numericVal(t, "10"), nil),
		entityVal(shown, groupA, "b", customfields.FieldTypeNumber, numericVal(t, "30"), nil),
	}
	// 'hidden' not in the caller's can_view set — contributes nothing.
	s.applyComputedFields(context.Background(), dtos, values, map[string]bool{shown.String(): true}, false)

	var got float64
	if err := json.Unmarshal(dtos[0].Value, &got); err != nil || got != 30 {
		t.Fatalf("invisible source must not aggregate, got %s", dtos[0].Value)
	}
}

func TestApplyComputedFields_MetricSumAndPriceRangeSource(t *testing.T) {
	groupA := pgid(0xA0)
	srcField := pgid(0xA1)
	ff := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{groupA.String(): {ID: groupA, Slug: "g"}},
	}
	s := &Service{fields: ff}

	dtos := []EntityFieldDTO{{
		ID: pgid(0xF0).String(), Key: "total", Label: "Total", FieldType: customfields.FieldTypeCurrency,
		Options: json.RawMessage(`{"computed":{"metric":"sum","sources":[{"group":"g","key":"rent"}]}}`),
		Visible: true, CanEdit: true,
	}}
	values := []gen.ListCustomFieldValuesWithDefsByEntityRow{
		entityVal(srcField, groupA, "rent", customfields.FieldTypePriceRange, pgtype.Numeric{}, []byte(`{"min":100,"avg":140,"max":200}`)),
	}
	s.applyComputedFields(context.Background(), dtos, values, map[string]bool{srcField.String(): true}, false)

	var got float64
	if err := json.Unmarshal(dtos[0].Value, &got); err != nil || got != 140 {
		t.Fatalf("price_range source should project avg, got %s", dtos[0].Value)
	}
}

func TestCheckSetValueFields_ComputedRejected(t *testing.T) {
	s := &Service{fields: &fakeFieldReader{
		perms: map[string][2]bool{},
	}}
	computedOpts := []byte(`{"computed":{"sources":[{"group":"g","key":"k"}]}}`)
	schema := map[string]schemaField{
		"derived": {id: pgid(0x01), key: "derived", fieldType: customfields.FieldTypeCurrency, options: computedOpts},
		"manual":  {id: pgid(0x02), key: "manual", fieldType: customfields.FieldTypeCurrency},
	}
	op := FieldOp{Op: OpSetValue, FieldKey: "derived", EntityType: "space", EntityID: pgid(0xEE).String(), Value: json.RawMessage(`100`)}

	err := s.checkSetValueFields(context.Background(), pgid(0x01), []FieldOp{op}, true, schema)
	if err == nil {
		t.Fatal("set_value on a computed field must be rejected")
	}
}

func TestApplySetValue_ComputedRejected(t *testing.T) {
	// The computed check precedes any query — nil q/tx prove it never
	// reaches the store.
	schema := map[string]schemaField{
		"derived": {id: pgid(0x01), key: "derived", fieldType: customfields.FieldTypeCurrency,
			options: []byte(`{"computed":{"sources":[{"group":"g","key":"k"}]}}`)},
	}
	op := FieldOp{Op: OpSetValue, FieldKey: "derived", EntityType: "space", EntityID: pgid(0xEE).String(), Value: json.RawMessage(`100`)}
	if err := applySetValue(context.Background(), nil, nil, op, schema); err == nil {
		t.Fatal("applySetValue must reject computed fields")
	}
}
