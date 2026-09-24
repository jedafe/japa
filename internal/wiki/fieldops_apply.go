// Package wiki — fieldops_apply.go is the transactional writer that executes
// a revision's ops array in order on approval (§5 of the community-fields
// design). Every op runs inside the caller's transaction so the field
// mutations and the revision status flip commit or roll back atomically.
// All sqlc writes go through gen.New(tx); the only raw tx.Exec is the
// whitelisted denormalized field_values merge — custom_field_values is the
// canonical store, the per-entity column is a read-cache mirror.
// // Ref: docs/superpowers/specs/2026-09-24-wiki-community-fields-design.md §5
package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// entityFieldValuesTable whitelists the entity types whose backing table
// carries a denormalized field_values jsonb column (§06). The table name is
// never taken from op input — only whitelisted entity types get the merge.
// Space-backed types (space/university/country/state/city) are intentionally
// absent: the spaces table has no field_values column, and cfv is canonical.
var entityFieldValuesTable = map[string]string{
	"organization": "organizations",
	"bank":         "organizations",
	"content":      "contents",
}

// applyOps executes a revision's ops array in order inside the caller's
// transaction. Callers obtain tx via s.store.pool.Writer.Begin(ctx) — or an
// outer approval transaction — and are responsible for commit/rollback. The
// first failing op aborts the array so a partially-applied revision never
// commits.
func (s *Service) applyOps(ctx context.Context, tx pgx.Tx, ops []FieldOp) error {
	q := gen.New(tx)

	for i := range ops {
		op := ops[i]

		var err error

		switch op.Op {
		case OpSetValue:
			err = applySetValue(ctx, q, tx, op)
		case OpProposeField:
			err = applyProposeField(ctx, q, op)
		case OpProposeGroup:
			err = applyProposeGroup(ctx, q, op)
		case OpUpsertField:
			err = applyUpsertField(ctx, q, op)
		case OpAssignGroup:
			err = applyAssignGroup(ctx, q, op)
		default:
			err = fmt.Errorf("unknown op %q", op.Op)
		}

		if err != nil {
			return fmt.Errorf("ops[%d] %s: %w", i, op.Op, err)
		}
	}

	return nil
}

// applySetValue resolves the field definition, coerces the op's JSON value
// into the correct custom_field_values column, upserts the cfv row, then
// merges the value into the entity's denormalized field_values column when
// the entity type is whitelisted.
func applySetValue(ctx context.Context, q *gen.Queries, tx pgx.Tx, op FieldOp) error {
	field, err := q.GetCustomFieldByKey(ctx, op.FieldKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("field %q does not exist", op.FieldKey)
		}

		return fmt.Errorf("lookup field %q: %w", op.FieldKey, err)
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(op.EntityID); err != nil {
		return fmt.Errorf("entity_id %q is not a UUID: %w", op.EntityID, err)
	}

	params, err := valueToUpsertParams(field, customfields.ValueEntityType(op.EntityType), entityID, op.Value)
	if err != nil {
		return err
	}

	if _, err := q.UpsertCustomFieldValue(ctx, params); err != nil {
		return fmt.Errorf("upsert value for %q: %w", op.FieldKey, err)
	}

	// Denormalized merge into the entity's field_values jsonb column. The
	// column is NOT NULL DEFAULT '{}' on every whitelisted table, so plain
	// jsonb concatenation is safe (last write wins per key, matching cfv's
	// upsert semantics).
	if table, ok := entityFieldValuesTable[op.EntityType]; ok {
		if _, err := tx.Exec(ctx,
			"UPDATE "+table+" SET field_values = field_values || jsonb_build_object($1, $2::jsonb) WHERE id = $3",
			op.FieldKey, string(op.Value), entityID,
		); err != nil {
			return fmt.Errorf("denorm merge into %s.field_values: %w", table, err)
		}
	}

	return nil
}

// applyProposeField adds one community-proposed field to an existing group.
// The group must already exist — propose_group is the op that creates groups.
func applyProposeField(ctx context.Context, q *gen.Queries, op FieldOp) error {
	group, err := q.GetCustomFieldGroupBySlug(ctx, op.GroupSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("group %q does not exist", op.GroupSlug)
		}

		return fmt.Errorf("lookup group %q: %w", op.GroupSlug, err)
	}

	return createFieldInGroup(ctx, q, group.ID, op.Field)
}

// applyProposeGroup creates a new field group, inserts each of its fields,
// and assigns the group to the op's entity_class.
func applyProposeGroup(ctx context.Context, q *gen.Queries, op FieldOp) error {
	var description *string
	if op.Group.Description != "" {
		description = &op.Group.Description
	}

	group, err := q.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{
		Name:        op.Group.Name,
		Slug:        op.Group.Slug,
		Description: description,
		IsSystem:    false,
	})
	if err != nil {
		return fmt.Errorf("create group %q: %w", op.Group.Slug, err)
	}

	for i := range op.Group.Fields {
		if err := createFieldInGroup(ctx, q, group.ID, &op.Group.Fields[i]); err != nil {
			return fmt.Errorf("group.fields[%d]: %w", i, err)
		}
	}

	if err := assignGroupToClass(ctx, q, group.ID, op.EntityClass); err != nil {
		return fmt.Errorf("assign group %q: %w", op.Group.Slug, err)
	}

	return nil
}

// applyUpsertField updates an existing field definition in place, or creates
// it in the named group when the key does not yet exist (upsert semantics —
// the op payload is authoritative for the fields it carries).
func applyUpsertField(ctx context.Context, q *gen.Queries, op FieldOp) error {
	group, err := q.GetCustomFieldGroupBySlug(ctx, op.GroupSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("group %q does not exist", op.GroupSlug)
		}

		return fmt.Errorf("lookup group %q: %w", op.GroupSlug, err)
	}

	existing, err := q.GetCustomFieldByKey(ctx, op.Field.Key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return createFieldInGroup(ctx, q, group.ID, op.Field)
		}

		return fmt.Errorf("lookup field %q: %w", op.Field.Key, err)
	}

	options, err := marshalFieldOptions(op.Field.Options)
	if err != nil {
		return fmt.Errorf("marshal options for %q: %w", op.Field.Key, err)
	}

	// UpdateCustomField writes is_system = false rows only, so proposing an
	// update to a system field fails with ErrNoRows — surfaced as an error
	// rather than silently ignored.
	params := gen.UpdateCustomFieldParams{
		ID:             existing.ID,
		Label:          op.Field.Label,
		FieldType:      op.Field.FieldType,
		Options:        options,
		IsRequired:     op.Field.IsRequired,
		ShowInFrontend: existing.ShowInFrontend,
		Filterable:     existing.Filterable,
		Sortable:       existing.Sortable,
		Searchable:     existing.Searchable,
		Position:       op.Field.Position,
	}

	if op.Field.Placeholder != "" {
		params.Placeholder = &op.Field.Placeholder
	}

	if op.Field.DefaultValue != "" {
		params.DefaultValue = &op.Field.DefaultValue
	}

	if _, err := q.UpdateCustomField(ctx, params); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("field %q is system-managed and cannot be updated", op.Field.Key)
		}

		return fmt.Errorf("update field %q: %w", op.Field.Key, err)
	}

	return nil
}

// applyAssignGroup links an existing group to the op's entity_class.
func applyAssignGroup(ctx context.Context, q *gen.Queries, op FieldOp) error {
	group, err := q.GetCustomFieldGroupBySlug(ctx, op.GroupSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("group %q does not exist", op.GroupSlug)
		}

		return fmt.Errorf("lookup group %q: %w", op.GroupSlug, err)
	}

	return assignGroupToClass(ctx, q, group.ID, op.EntityClass)
}

// createFieldInGroup inserts one community-proposed field definition into an
// existing group. Community fields are never system fields and are shown in
// the frontend by default (§5).
func createFieldInGroup(ctx context.Context, q *gen.Queries, groupID pgtype.UUID, f *FieldDefInput) error {
	options, err := marshalFieldOptions(f.Options)
	if err != nil {
		return fmt.Errorf("marshal options for %q: %w", f.Key, err)
	}

	params := gen.CreateCustomFieldParams{
		GroupID:        groupID,
		Label:          f.Label,
		Key:            f.Key,
		FieldType:      f.FieldType,
		Options:        options,
		IsRequired:     f.IsRequired,
		IsSystem:       false,
		ShowInFrontend: true,
		Position:       f.Position,
	}

	if f.Placeholder != "" {
		params.Placeholder = &f.Placeholder
	}

	if f.DefaultValue != "" {
		params.DefaultValue = &f.DefaultValue
	}

	if _, err := q.CreateCustomField(ctx, params); err != nil {
		return fmt.Errorf("create field %q: %w", f.Key, err)
	}

	return nil
}

// assignGroupToClass links a group to an entity_class via
// custom_field_group_assignments. CreateAssignment is ON CONFLICT DO NOTHING,
// which yields zero RETURNING rows on a duplicate — pgx.ErrNoRows is treated
// as success so re-applying an assign op is idempotent.
func assignGroupToClass(ctx context.Context, q *gen.Queries, groupID pgtype.UUID, entityClass string) error {
	_, err := q.CreateAssignment(ctx, gen.CreateAssignmentParams{
		GroupID:     groupID,
		EntityClass: entityClass,
		EntityID:    pgtype.UUID{},
		Position:    0,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}

		return fmt.Errorf("create assignment for %q: %w", entityClass, err)
	}

	return nil
}

// marshalFieldOptions encodes field options for the custom_fields.options
// jsonb column. Empty option lists store NULL, matching the admin handler.
func marshalFieldOptions(opts []customfields.FieldOption) ([]byte, error) {
	if len(opts) == 0 {
		return nil, nil
	}

	return json.Marshal(opts)
}

// valueToUpsertParams maps an op's JSON value onto the custom_field_values
// column the field's type stores in (§06 FieldTypeValueColumn). The op's
// value is authoritative: a JSON null clears the value (all columns NULL),
// text/number/bool/date columns get a decoded scalar, and JSON-typed columns
// keep the raw bytes verbatim.
func valueToUpsertParams(field gen.GetCustomFieldByKeyRow, entityType string, entityID pgtype.UUID, v json.RawMessage) (gen.UpsertCustomFieldValueParams, error) {
	params := gen.UpsertCustomFieldValueParams{
		FieldID:    field.ID,
		EntityType: entityType,
		EntityID:   entityID,
	}

	column, ok := customfields.FieldTypeValueColumn(field.FieldType)
	if !ok {
		return params, fmt.Errorf("field %q type %q stores no value", field.Key, field.FieldType)
	}

	// JSON null clears the value — the upsert overwrites every column.
	if string(v) == "null" {
		return params, nil
	}

	setter, ok := valueColumnSetters[column]
	if !ok {
		return params, fmt.Errorf("field %q maps to unhandled value column %q", field.Key, column)
	}

	if err := setter(&params, field.Key, v); err != nil {
		return params, err
	}

	return params, nil
}

// valueColumnSetters decodes an op's JSON value into the matching
// custom_field_values column of UpsertCustomFieldValueParams.
var valueColumnSetters = map[string]func(*gen.UpsertCustomFieldValueParams, string, json.RawMessage) error{
	"value_text": func(p *gen.UpsertCustomFieldValueParams, key string, v json.RawMessage) error {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return fmt.Errorf("field %q expects a JSON string value: %w", key, err)
		}

		p.ValueText = &s

		return nil
	},
	"value_number": func(p *gen.UpsertCustomFieldValueParams, key string, v json.RawMessage) error {
		var num pgtype.Numeric
		if err := num.Scan(string(v)); err != nil {
			return fmt.Errorf("field %q expects a JSON number value: %w", key, err)
		}

		p.ValueNumber = num

		return nil
	},
	"value_bool": func(p *gen.UpsertCustomFieldValueParams, key string, v json.RawMessage) error {
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return fmt.Errorf("field %q expects a JSON boolean value: %w", key, err)
		}

		p.ValueBool = &b

		return nil
	},
	"value_date": func(p *gen.UpsertCustomFieldValueParams, key string, v json.RawMessage) error {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return fmt.Errorf("field %q expects a JSON string value: %w", key, err)
		}

		ts, err := parseDateValue(s)
		if err != nil {
			return fmt.Errorf("field %q expects a date/timestamp value: %w", key, err)
		}

		p.ValueDate = ts

		return nil
	},
	"value_json": func(p *gen.UpsertCustomFieldValueParams, key string, v json.RawMessage) error {
		if !json.Valid(v) {
			return fmt.Errorf("field %q expects a valid JSON value", key)
		}

		p.ValueJson = v

		return nil
	},
}

// dateValueLayouts are tried in order when coercing an op's JSON string into
// the value_date (timestamptz) column — covering full timestamps, date-only,
// and time-only strings (the date/datetime/time field types all land here).
var dateValueLayouts = []string{
	time.RFC3339,
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
	"15:04:05",
	"15:04",
}

// parseDateValue coerces a string into a Timestamptz for the value_date
// column. Timestamptz.Scan only accepts full Postgres timestamp strings, so
// the friendlier layouts an op submitter would send are parsed explicitly.
func parseDateValue(s string) (pgtype.Timestamptz, error) {
	for _, layout := range dateValueLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return pgtype.Timestamptz{Time: t, Valid: true}, nil
		}
	}

	return pgtype.Timestamptz{}, fmt.Errorf("unrecognized date/timestamp %q", s)
}
