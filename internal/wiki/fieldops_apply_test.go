package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// txCall records one Exec/QueryRow against the fake transaction.
type txCall struct {
	kind string // "exec" or "queryrow"
	sql  string
	args []any
}

// fakeTx is a pgx.Tx stub for applyOps unit tests. QueryRow pops a scripted
// Scan function from responses (in call order); Exec records the call and
// reports success. The embedded pgx.Tx satisfies the interface — any other
// method call panics, which is the point: applyOps must only use Exec +
// QueryRow (via gen.Queries).
type fakeTx struct {
	pgx.Tx

	calls     []txCall
	responses []func(dest ...any) error
}

func (f *fakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, txCall{kind: "exec", sql: sql, args: args})

	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (f *fakeTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.calls = append(f.calls, txCall{kind: "queryrow", sql: sql, args: args})

	if len(f.responses) == 0 {
		return fakeRow{err: fmt.Errorf("unexpected query: %s", sql)}
	}

	scan := f.responses[0]
	f.responses = f.responses[1:]

	return fakeRow{scan: scan}
}

type fakeRow struct {
	scan func(dest ...any) error
	err  error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	if r.scan != nil {
		return r.scan(dest...)
	}

	return nil
}

// scanUUIDAt writes a pgtype.UUID into the i-th scan destination.
func scanUUIDAt(dest []any, i int, id pgtype.UUID) error {
	p, ok := dest[i].(*pgtype.UUID)
	if !ok {
		return fmt.Errorf("scan dest[%d] is %T, want *pgtype.UUID", i, dest[i])
	}

	*p = id

	return nil
}

// scanStringAt writes a string into the i-th scan destination.
func scanStringAt(dest []any, i int, s string) error {
	p, ok := dest[i].(*string)
	if !ok {
		return fmt.Errorf("scan dest[%d] is %T, want *string", i, dest[i])
	}

	*p = s

	return nil
}

// scanFieldRow fills a GetCustomFieldByKeyRow scan — column order is
// (id, group_id, label, key, field_type, options, embed_platforms, ...).
func scanFieldRow(id pgtype.UUID, key, fieldType string) func(...any) error {
	return func(dest ...any) error {
		if err := scanUUIDAt(dest, 0, id); err != nil {
			return err
		}

		if err := scanStringAt(dest, 3, key); err != nil {
			return err
		}

		return scanStringAt(dest, 4, fieldType)
	}
}

// scanGroupRow fills a CustomFieldGroup scan — column order is
// (id, name, slug, description, is_system, created_at).
func scanGroupRow(id pgtype.UUID, slug string) func(...any) error {
	return func(dest ...any) error {
		if err := scanUUIDAt(dest, 0, id); err != nil {
			return err
		}

		return scanStringAt(dest, 2, slug)
	}
}

// scanNoop succeeds without writing — for INSERT ... RETURNING scans whose
// returned values applyOps ignores.
func scanNoop() func(...any) error { return func(...any) error { return nil } }

func scanFail(err error) func(...any) error {
	return func(...any) error { return err }
}

// valueColumns renders each populated custom_field_values column as a
// column-name → string map for comparison in the table test.
func valueColumns(p gen.UpsertCustomFieldValueParams) map[string]string {
	out := map[string]string{}

	if p.ValueText != nil {
		out["text"] = *p.ValueText
	}

	if p.ValueNumber.Valid {
		if f64, err := p.ValueNumber.Float64Value(); err == nil && f64.Valid {
			out["number"] = strconv.FormatFloat(f64.Float64, 'f', -1, 64)
		}
	}

	if p.ValueBool != nil {
		out["bool"] = strconv.FormatBool(*p.ValueBool)
	}

	if p.ValueDate.Valid {
		out["date"] = p.ValueDate.Time.Format("2006-01-02")
	}

	if p.ValueJson != nil {
		out["json"] = string(p.ValueJson)
	}

	return out
}

// TestValueToUpsertParams covers the JSON-value → value-column mapping for
// each FieldTypeValueColumn branch, plus the null-clear and bad-input cases.
func TestValueToUpsertParams(t *testing.T) {
	t.Parallel()

	fieldID := mustUUID(t, "00000000-0000-0000-0000-0000000000a2")
	entityID := mustUUID(t, "00000000-0000-0000-0000-0000000000bb")

	cases := []struct {
		name      string
		fieldType string
		value     json.RawMessage
		wantCol   string // value column expected to be populated
		want      string // expected value rendered as a string
		wantErr   string
	}{
		{name: "text writes value_text", fieldType: customfields.FieldTypeText, value: json.RawMessage(`"hello"`), wantCol: "text", want: "hello"},
		{name: "number writes value_number", fieldType: customfields.FieldTypeNumber, value: json.RawMessage(`12000`), wantCol: "number", want: "12000"},
		{name: "bool writes value_bool", fieldType: customfields.FieldTypeBoolean, value: json.RawMessage(`true`), wantCol: "bool", want: "true"},
		{name: "select writes value_json verbatim", fieldType: customfields.FieldTypeSelect, value: json.RawMessage(`"fall"`), wantCol: "json", want: `"fall"`},
		{name: "date writes value_date", fieldType: customfields.FieldTypeDate, value: json.RawMessage(`"2026-01-15"`), wantCol: "date", want: "2026-01-15"},
		{name: "null clears all value columns", fieldType: customfields.FieldTypeText, value: json.RawMessage(`null`), wantCol: "none"},
		{name: "bad JSON errors", fieldType: customfields.FieldTypeSelect, value: json.RawMessage(`{nope`), wantErr: "valid JSON"},
		{name: "non-string for text errors", fieldType: customfields.FieldTypeText, value: json.RawMessage(`[1,2]`), wantErr: "JSON string"},
		{name: "group field type has no value column", fieldType: customfields.FieldTypeGroup, value: json.RawMessage(`"x"`), wantErr: "stores no value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			field := gen.GetCustomFieldByKeyRow{ID: fieldID, Key: "k", FieldType: tc.fieldType}

			p, err := valueToUpsertParams(field, "organization", entityID, tc.value)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}

				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v should mention %q", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if p.FieldID != fieldID || p.EntityID != entityID || p.EntityType != "organization" {
				t.Fatalf("identity fields wrong: %+v", p)
			}

			cols := valueColumns(p)

			if tc.wantCol == "none" {
				if len(cols) != 0 {
					t.Fatalf("null should leave every column empty, got %v", cols)
				}

				return
			}

			if len(cols) != 1 || cols[tc.wantCol] != tc.want {
				t.Fatalf("value columns: got %v want {%s: %q}", cols, tc.wantCol, tc.want)
			}
		})
	}
}

// TestApplyOps_ExecutesOpsInOrder scripts a propose_field → set_value →
// assign_group sequence and asserts both the SQL call order and that the
// denormalized merge lands on the whitelisted organizations table.
func TestApplyOps_ExecutesOpsInOrder(t *testing.T) {
	t.Parallel()

	groupID := mustUUID(t, "00000000-0000-0000-0000-0000000000a1")
	fieldID := mustUUID(t, "00000000-0000-0000-0000-0000000000a2")
	entityID := "00000000-0000-0000-0000-0000000000bb"

	tx := &fakeTx{
		responses: []func(dest ...any) error{
			scanGroupRow(groupID, "org_fields"), // propose_field: GetCustomFieldGroupBySlug
			scanNoop(),                          // propose_field: CreateCustomField
			scanFieldRow(fieldID, "headcount", customfields.FieldTypeNumber), // set_value: GetCustomFieldByKey
			scanNoop(),                          // set_value: UpsertCustomFieldValue
			scanGroupRow(groupID, "org_fields"), // assign_group: GetCustomFieldGroupBySlug
			scanNoop(),                          // assign_group: CreateAssignment
		},
	}

	ops := []FieldOp{
		{
			Op:        OpProposeField,
			GroupSlug: "org_fields",
			Field:     &FieldDefInput{Key: "headcount", Label: "Headcount", FieldType: customfields.FieldTypeNumber},
		},
		{
			Op:         OpSetValue,
			EntityType: "organization",
			EntityID:   entityID,
			FieldKey:   "headcount",
			Value:      json.RawMessage(`42`),
		},
		{
			Op:          OpAssignGroup,
			GroupSlug:   "org_fields",
			EntityClass: "organization",
		},
	}

	if err := (&Service{}).applyOps(t.Context(), tx, ops); err != nil {
		t.Fatalf("applyOps: %v", err)
	}

	want := []struct {
		kind        string
		sqlContains string
	}{
		{"queryrow", "FROM custom_field_groups"},                   // propose_field group lookup
		{"queryrow", "INSERT INTO custom_fields ("},                // propose_field field insert
		{"queryrow", "WHERE key = $1"},                             // set_value field lookup
		{"queryrow", "INSERT INTO custom_field_values"},            // set_value cfv upsert
		{"exec", "UPDATE organizations SET field_values"},          // set_value denorm merge
		{"queryrow", "FROM custom_field_groups"},                   // assign_group group lookup
		{"queryrow", "INSERT INTO custom_field_group_assignments"}, // assign_group assignment
	}

	if len(tx.calls) != len(want) {
		t.Fatalf("call count: got %d want %d — calls: %#v", len(tx.calls), len(want), tx.calls)
	}

	for i, w := range want {
		got := tx.calls[i]
		if got.kind != w.kind || !strings.Contains(got.sql, w.sqlContains) {
			t.Fatalf("call[%d]: got %s %q, want %s containing %q", i, got.kind, got.sql, w.kind, w.sqlContains)
		}
	}

	// The denorm merge passes field_key, raw JSON value, entity UUID.
	merge := tx.calls[4]
	if merge.args[0] != "headcount" || merge.args[1] != "42" {
		t.Fatalf("denorm merge args: %#v", merge.args)
	}

	gotEntityID, ok := merge.args[2].(pgtype.UUID)
	if !ok || gotEntityID.String() != entityID {
		t.Fatalf("denorm merge entity id: %#v", merge.args[2])
	}

	// The cfv upsert args are (field_id, entity_type, entity_id, ...): the
	// field ID must come from the resolved field row.
	upsert := tx.calls[3]
	if gotFieldID, ok := upsert.args[0].(pgtype.UUID); !ok || gotFieldID != fieldID {
		t.Fatalf("upsert field_id: %#v", upsert.args[0])
	}
}

// TestApplyOps_SetValueNormalizesEntityType verifies space-backed entity
// types store cfv rows under entity_type 'space' — and skip the denorm merge
// entirely because spaces has no field_values column.
func TestApplyOps_SetValueNormalizesEntityType(t *testing.T) {
	t.Parallel()

	fieldID := mustUUID(t, "00000000-0000-0000-0000-0000000000a2")

	tx := &fakeTx{
		responses: []func(dest ...any) error{
			scanFieldRow(fieldID, "tuition_ug", customfields.FieldTypeText),
			scanNoop(),
		},
	}

	ops := []FieldOp{{
		Op:         OpSetValue,
		EntityType: "university",
		EntityID:   "00000000-0000-0000-0000-0000000000bb",
		FieldKey:   "tuition_ug",
		Value:      json.RawMessage(`"12000 USD"`),
	}}

	if err := (&Service{}).applyOps(t.Context(), tx, ops); err != nil {
		t.Fatalf("applyOps: %v", err)
	}

	if len(tx.calls) != 2 {
		t.Fatalf("expected 2 calls (lookup + upsert, no denorm), got %d: %#v", len(tx.calls), tx.calls)
	}

	for _, c := range tx.calls {
		if c.kind == "exec" {
			t.Fatalf("university entity type must not trigger a denorm merge: %#v", c)
		}
	}

	// UpsertCustomFieldValue arg[1] is entity_type — 'university' must
	// normalize to 'space'.
	if tx.calls[1].args[1] != "space" {
		t.Fatalf("cfv entity_type: got %v want space", tx.calls[1].args[1])
	}
}

// TestApplyOps_StopsOnFirstError proves a failing op aborts the array — later
// ops never reach the database.
func TestApplyOps_StopsOnFirstError(t *testing.T) {
	t.Parallel()

	tx := &fakeTx{
		responses: []func(dest ...any) error{
			scanFail(errors.New("group lookup blew up")),
		},
	}

	ops := []FieldOp{
		{Op: OpAssignGroup, GroupSlug: "g1", EntityClass: "organization"},
		{Op: OpAssignGroup, GroupSlug: "g2", EntityClass: "organization"},
	}

	err := (&Service{}).applyOps(t.Context(), tx, ops)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "ops[0]") || !strings.Contains(err.Error(), "group lookup blew up") {
		t.Fatalf("error should identify the failing op and wrap the cause: %v", err)
	}

	if len(tx.calls) != 1 {
		t.Fatalf("second op must not run after the first fails — calls: %#v", tx.calls)
	}
}

// TestApplyOps_UpsertFieldCreateAndUpdate covers the two upsert_field paths:
// existing key → UPDATE custom_fields, missing key → INSERT into the group.
func TestApplyOps_UpsertFieldCreateAndUpdate(t *testing.T) {
	t.Parallel()

	groupID := mustUUID(t, "00000000-0000-0000-0000-0000000000a1")
	fieldID := mustUUID(t, "00000000-0000-0000-0000-0000000000a2")

	field := &FieldDefInput{Key: "intake", Label: "Intake", FieldType: customfields.FieldTypeText}

	t.Run("existing field updates", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{responses: []func(dest ...any) error{
			scanGroupRow(groupID, "org_fields"),
			scanFieldRow(fieldID, "intake", customfields.FieldTypeText),
			scanNoop(), // UpdateCustomField RETURNING
		}}

		err := (&Service{}).applyOps(t.Context(), tx, []FieldOp{
			{Op: OpUpsertField, GroupSlug: "org_fields", Field: field},
		})
		if err != nil {
			t.Fatalf("applyOps: %v", err)
		}

		if len(tx.calls) != 3 || !strings.Contains(tx.calls[2].sql, "UPDATE custom_fields") {
			t.Fatalf("expected group lookup + field lookup + UPDATE, got %#v", tx.calls)
		}
	})

	t.Run("missing field creates", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{responses: []func(dest ...any) error{
			scanGroupRow(groupID, "org_fields"),
			scanFail(pgx.ErrNoRows), // GetCustomFieldByKey misses
			scanNoop(),              // CreateCustomField RETURNING
		}}

		err := (&Service{}).applyOps(t.Context(), tx, []FieldOp{
			{Op: OpUpsertField, GroupSlug: "org_fields", Field: field},
		})
		if err != nil {
			t.Fatalf("applyOps: %v", err)
		}

		if len(tx.calls) != 3 || !strings.Contains(tx.calls[2].sql, "INSERT INTO custom_fields (") {
			t.Fatalf("expected group lookup + field lookup + INSERT, got %#v", tx.calls)
		}
	})
}

// TestApplyOps_ProposeGroup verifies the group → fields → assignment chain.
func TestApplyOps_ProposeGroup(t *testing.T) {
	t.Parallel()

	groupID := mustUUID(t, "00000000-0000-0000-0000-0000000000a1")

	tx := &fakeTx{responses: []func(dest ...any) error{
		scanGroupRow(groupID, "visa_outcomes"), // CreateCustomFieldGroup RETURNING
		scanNoop(),                             // CreateCustomField field[0]
		scanNoop(),                             // CreateCustomField field[1]
		scanNoop(),                             // CreateAssignment
	}}

	ops := []FieldOp{{
		Op:          OpProposeGroup,
		EntityClass: "space_type:university",
		Group: &GroupDefInput{
			Slug: "visa_outcomes",
			Name: "Visa Outcomes",
			Fields: []FieldDefInput{
				{Key: "intake", Label: "Intake", FieldType: customfields.FieldTypeText},
				{Key: "rate", Label: "Rate", FieldType: customfields.FieldTypePercentage},
			},
		},
	}}

	if err := (&Service{}).applyOps(t.Context(), tx, ops); err != nil {
		t.Fatalf("applyOps: %v", err)
	}

	want := []string{
		"INSERT INTO custom_field_groups",
		"INSERT INTO custom_fields (",
		"INSERT INTO custom_fields (",
		"INSERT INTO custom_field_group_assignments",
	}

	if len(tx.calls) != len(want) {
		t.Fatalf("call count: got %d want %d — %#v", len(tx.calls), len(want), tx.calls)
	}

	for i, w := range want {
		if !strings.Contains(tx.calls[i].sql, w) {
			t.Fatalf("call[%d]: got %q want containing %q", i, tx.calls[i].sql, w)
		}
	}

	// The assignment binds the new group to the op's entity_class.
	assign := tx.calls[3]
	if gotGroupID, ok := assign.args[0].(pgtype.UUID); !ok || gotGroupID != groupID {
		t.Fatalf("assignment group_id: %#v", assign.args[0])
	}

	if assign.args[1] != "space_type:university" {
		t.Fatalf("assignment entity_class: %v", assign.args[1])
	}
}
