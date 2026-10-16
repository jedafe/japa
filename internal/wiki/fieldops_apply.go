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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// schemaField is the resolved definition of one field in an entity's
// editable schema — enough for value coercion and options checks without a
// per-op lookup. The map is built from the entity's assigned groups so a
// field_key colliding with an unrelated group's field resolves to the right
// row: field keys are unique per group (custom_fields group_id+key), not
// globally, and a global key lookup may return a field the entity's schema
// does not contain.
type schemaField struct {
	id        pgtype.UUID
	key       string
	fieldType string
	options   []byte
}

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
// commits. schema is the revision target's field map (entitySchemaFields) —
// set_value resolves field_key inside it, and fields created by proposal
// ops are registered into it so later set_value ops in the same array can
// target them. Pass nil for target-less (proposal-only) revisions.
func (s *Service) applyOps(ctx context.Context, tx pgx.Tx, ops []FieldOp, schema map[string]schemaField) error {
	q := gen.New(tx)

	if schema == nil {
		schema = map[string]schemaField{}
	}

	for i := range ops {
		op := ops[i]

		var err error

		switch op.Op {
		case OpSetValue:
			err = applySetValue(ctx, q, tx, op, schema)
		case OpProposeField:
			err = applyProposeField(ctx, q, op, schema)
		case OpProposeGroup:
			err = applyProposeGroup(ctx, q, op, schema)
		case OpUpsertField:
			err = applyUpsertField(ctx, q, op, schema)
		case OpAssignGroup:
			err = applyAssignGroup(ctx, q, op)
		case OpDeleteField:
			err = applyDeleteField(ctx, q, op, schema)
		default:
			err = fmt.Errorf("unknown op %q", op.Op)
		}

		if err != nil {
			return fmt.Errorf("ops[%d] %s: %w", i, op.Op, err)
		}
	}

	return nil
}

// applySetValue resolves the field definition from the entity's schema map
// (field keys are group-unique only — the schema scope is what makes
// field_key unambiguous), coerces the op's JSON value into the correct
// custom_field_values column, upserts the cfv row, then merges the value
// into the entity's denormalized field_values column when the entity type
// is whitelisted.
func applySetValue(ctx context.Context, q *gen.Queries, tx pgx.Tx, op FieldOp, schema map[string]schemaField) error {
	field, ok := schema[op.FieldKey]
	if !ok {
		return fmt.Errorf("field %q does not exist in the entity's field schema", op.FieldKey)
	}

	// The apply-path twin of the submit-time computed check — a revision
	// queued before a spec was added (or an earlier upsert_field that
	// introduced one) cannot slip a manual value through approval. Same
	// eligibility gate as the read path: an ineligible spec is ignored
	// rather than producing a field that is neither computed nor
	// manually editable.
	if customfields.ComputedEligibleField(field.fieldType, field.options) {
		return fmt.Errorf("field %q is computed and cannot be set manually", op.FieldKey)
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
	// upsert semantics). CF12: write the normalized bytes produced by
	// valueToUpsertParams (the cfv row's canonical value), never the raw op
	// payload — the mirror must converge with cfv. cfv stays canonical: on
	// any dual-write divergence the cfv row wins at read time (CF11), and a
	// mismatch here is logged by the caller-side audit, not silently kept.
	if table, ok := entityFieldValuesTable[op.EntityType]; ok {
		denorm := denormValueJSON(params, op.Value)
		if _, err := tx.Exec(ctx,
			"UPDATE "+table+" SET field_values = field_values || jsonb_build_object($1, $2::jsonb) WHERE id = $3",
			op.FieldKey, string(denorm), entityID,
		); err != nil {
			return fmt.Errorf("denorm merge into %s.field_values: %w", table, err)
		}

		if diverges(denorm, op.Value) {
			logDivergence(ctx, op.EntityType, op.FieldKey, entityID, op.Value, denorm)
		}
	}

	return nil
}

// denormValueJSON renders the cfv row's canonical stored value for the
// denormalized mirror: the typed column wins, falling back to the raw op
// payload only when no typed column was set (null-clear).
func denormValueJSON(params gen.UpsertCustomFieldValueParams, raw json.RawMessage) json.RawMessage {
	switch {
	case params.ValueText != nil:
		if b, err := json.Marshal(*params.ValueText); err == nil {
			return b
		}
	case params.ValueNumber.Valid:
		if f, err := params.ValueNumber.Float64Value(); err == nil && f.Valid {
			if b, err := json.Marshal(f.Float64); err == nil {
				return b
			}
		}
	case params.ValueBool != nil:
		if b, err := json.Marshal(*params.ValueBool); err == nil {
			return b
		}
	case params.ValueDate.Valid:
		if b, err := json.Marshal(params.ValueDate.Time.UTC().Format(time.RFC3339)); err == nil {
			return b
		}
	case len(params.ValueJson) > 0:
		return json.RawMessage(params.ValueJson)
	}

	return raw
}

// diverges reports whether the normalized denorm bytes differ from the raw
// op payload beyond whitespace — i.e. normalization rewrote the value.
func diverges(normalized, raw json.RawMessage) bool {
	return strings.TrimSpace(string(normalized)) != strings.TrimSpace(string(raw))
}

// logDivergence warns on a dual-write normalization gap: the cfv row and the
// denorm mirror now hold different bytes for the same key. The cfv row is
// canonical (CF11); this log is the audit trail, not a second write.
func logDivergence(ctx context.Context, entityType, fieldKey string, entityID pgtype.UUID, raw, normalized json.RawMessage) {
	_ = ctx

	slog.Warn("wiki: denorm divergence normalized",
		"entity_type", entityType, "entity_id", entityID.String(),
		"field_key", fieldKey,
		"raw_bytes", len(raw), "normalized_bytes", len(normalized))
}

// applyProposeField adds one community-proposed field to an existing group.
// The group must already exist — propose_group is the op that creates groups.
func applyProposeField(ctx context.Context, q *gen.Queries, op FieldOp, schema map[string]schemaField) error {
	group, err := q.GetCustomFieldGroupBySlug(ctx, op.GroupSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("group %q does not exist", op.GroupSlug)
		}

		return fmt.Errorf("lookup group %q: %w", op.GroupSlug, err)
	}

	return createFieldInGroup(ctx, q, group.ID, op.Field, schema)
}

// applyProposeGroup creates a new field group, assigns it to the op's
// entity_class, then inserts each of its fields. The assignment runs BEFORE
// the field inserts: the §8.1a permission seed in createFieldInGroup grants
// can_edit only when the field's group is assigned to an entity class.
func applyProposeGroup(ctx context.Context, q *gen.Queries, op FieldOp, schema map[string]schemaField) error {
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("group slug %q already exists", op.Group.Slug)
		}

		return fmt.Errorf("create group %q: %w", op.Group.Slug, err)
	}

	if err := assignGroupToClass(ctx, q, group.ID, op.EntityClass); err != nil {
		return fmt.Errorf("assign group %q: %w", op.Group.Slug, err)
	}

	for i := range op.Group.Fields {
		if err := createFieldInGroup(ctx, q, group.ID, &op.Group.Fields[i], schema); err != nil {
			return fmt.Errorf("group.fields[%d]: %w", i, err)
		}
	}

	return nil
}

// applyUpsertField updates an existing field definition in place, or creates
// it in the named group when the key does not yet exist (upsert semantics —
// the op payload is authoritative for the fields it carries). The existing
// field is resolved inside the op's group — keys are group-unique only, so
// a global key lookup could update an unrelated group's field.
func applyUpsertField(ctx context.Context, q *gen.Queries, op FieldOp, schema map[string]schemaField) error {
	group, err := q.GetCustomFieldGroupBySlug(ctx, op.GroupSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("group %q does not exist", op.GroupSlug)
		}

		return fmt.Errorf("lookup group %q: %w", op.GroupSlug, err)
	}

	existing, err := q.GetCustomFieldByGroupAndKey(ctx, gen.GetCustomFieldByGroupAndKeyParams{
		GroupID: group.ID,
		Key:     op.Field.Key,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return createFieldInGroup(ctx, q, group.ID, op.Field, schema)
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
		// The op payload is authoritative (upsert semantics): every apply
		// rewrites the section token, "" clearing it.
		UpdateSection: true,
		Section:       op.Field.Section,
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

	// Register the updated definition so a later set_value op in the same
	// array validates/applies against the new type and options.
	schema[existing.Key] = schemaField{
		id:        existing.ID,
		key:       existing.Key,
		fieldType: op.Field.FieldType,
		options:   options,
	}

	return nil
}

// applyDeleteField removes a non-system field definition from the named
// group — a super_admin-group op (§80 Super-Admin Direct Publish). The
// custom_fields row hard-deletes and its custom_field_values cascade by
// FK; the key leaves the op-array schema so a later set_value in the same
// revision fails instead of writing into a deleted field.
func applyDeleteField(ctx context.Context, q *gen.Queries, op FieldOp, schema map[string]schemaField) error {
	group, err := q.GetCustomFieldGroupBySlug(ctx, op.GroupSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("group %q does not exist", op.GroupSlug)
		}

		return fmt.Errorf("lookup group %q: %w", op.GroupSlug, err)
	}

	field, err := q.GetCustomFieldByGroupAndKey(ctx, gen.GetCustomFieldByGroupAndKeyParams{
		GroupID: group.ID,
		Key:     op.FieldKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("field %q does not exist in group %q", op.FieldKey, op.GroupSlug)
		}

		return fmt.Errorf("lookup field %q: %w", op.FieldKey, err)
	}

	if field.IsSystem {
		return fmt.Errorf("field %q is system-managed and cannot be deleted", op.FieldKey)
	}

	if err := q.DeleteCustomField(ctx, field.ID); err != nil {
		return fmt.Errorf("delete field %q: %w", op.FieldKey, err)
	}

	delete(schema, op.FieldKey)

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
// existing group, seeds its §8.1a field permissions (the runtime twin of the
// 000733 bootstrap — without permission rows the fail-closed field-level
// check would leave an approved field permanently invisible and uneditable),
// and registers it in the op-array schema so a later set_value op in the
// same revision resolves it. Community fields are never system fields and
// are shown in the frontend by default (§5).
func createFieldInGroup(ctx context.Context, q *gen.Queries, groupID pgtype.UUID, f *FieldDefInput, schema map[string]schemaField) error {
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

	if f.Section != "" {
		params.Section = &f.Section
	}

	if f.Placeholder != "" {
		params.Placeholder = &f.Placeholder
	}

	if f.DefaultValue != "" {
		params.DefaultValue = &f.DefaultValue
	}

	field, err := q.CreateCustomField(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("field %q already exists in this group", f.Key)
		}

		return fmt.Errorf("create field %q: %w", f.Key, err)
	}

	if err := q.SeedFieldPermissions(ctx, gen.SeedFieldPermissionsParams{
		FieldID: field.ID,
		GroupID: groupID,
	}); err != nil {
		return fmt.Errorf("seed permissions for %q: %w", f.Key, err)
	}

	schema[f.Key] = schemaField{
		id:        field.ID,
		key:       field.Key,
		fieldType: field.FieldType,
		options:   field.Options,
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

// marshalFieldOptions validates and compacts a field definition's raw
// options payload for the custom_fields.options jsonb column. Empty or
// "null" payloads store NULL, matching the admin handler.
func marshalFieldOptions(opts json.RawMessage) ([]byte, error) {
	if len(opts) == 0 || string(opts) == "null" {
		return nil, nil
	}

	if !json.Valid(opts) {
		return nil, errors.New("options are not valid JSON")
	}

	var buf bytes.Buffer
	if err := json.Compact(&buf, opts); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// valueToUpsertParams maps an op's JSON value onto the custom_field_values
// column the field's type stores in (§06 FieldTypeValueColumn). The op's
// value is authoritative: a JSON null clears the value (all columns NULL),
// text/number/bool/date columns get a decoded scalar, and JSON-typed columns
// keep the raw bytes verbatim.
func valueToUpsertParams(field schemaField, entityType string, entityID pgtype.UUID, v json.RawMessage) (gen.UpsertCustomFieldValueParams, error) {
	params := gen.UpsertCustomFieldValueParams{
		FieldID:    field.id,
		EntityType: entityType,
		EntityID:   entityID,
	}

	column, ok := customfields.FieldTypeValueColumn(field.fieldType)
	if !ok {
		return params, fmt.Errorf("field %q type %q stores no value", field.key, field.fieldType)
	}

	// JSON null clears the value — the upsert overwrites every column.
	if string(v) == "null" {
		return params, nil
	}

	// §80 Itemized Pricing: price_range is a composite {avg,min,max} object —
	// coerce to the canonical shape before the raw-JSON column setter.
	if field.fieldType == customfields.FieldTypePriceRange {
		norm, nerr := normalizePriceRange(v, field.key)
		if nerr != nil {
			return params, nerr
		}

		v = norm
	}

	// §80 Wiki Topic Pages: price_parts is a multi-part composite — coerce
	// to the canonical {"parts": {...}} shape against the field's declared
	// options.parts schema; unknown part keys are rejected.
	if field.fieldType == customfields.FieldTypePriceParts {
		norm, nerr := normalizePriceParts(v, field.key, field.options)
		if nerr != nil {
			return params, nerr
		}

		v = norm
	}

	// CF2: sanitize rich_text HTML at apply time — the same pass the submit
	// path runs, so revisions approved from queues written before the submit
	// gate still land clean. Scrubbed before the column setter marshals.
	if field.fieldType == customfields.FieldTypeRichText && len(v) > 0 && wikiRichTextSanitizer != nil {
		v = customfields.SanitizeRichTextJSON(v, wikiRichTextSanitizer)
	}

	// CF3: canonical select matching is value-only — normalize labels to
	// values before the column setter so stored bytes converge with the REST
	// value handler.
	if (field.fieldType == customfields.FieldTypeSelect || field.fieldType == customfields.FieldTypeMultiselect) && len(v) > 0 {
		v = customfields.CanonicalizeSelectJSON(field.fieldType, field.options, v)
	}

	setter, ok := valueColumnSetters[column]
	if !ok {
		return params, fmt.Errorf("field %q maps to unhandled value column %q", field.key, column)
	}

	if err := setter(&params, field.key, v); err != nil {
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
	"2006-01-02T15:04", // <input type="datetime-local"> emits no seconds
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

// normalizePriceRange coerces a set_value payload into the canonical
// price_range object {"avg":n,"min":n,"max":n} (§80 Itemized Pricing).
// Each key is optional but at least one must be present; min <= max is
// enforced when both are supplied. Unknown keys are rejected (like
// normalizePriceParts) — silently dropping a typo'd bound would store a
// value the submitter did not intend.
func normalizePriceRange(v json.RawMessage, key string) (json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(v, &raw); err != nil {
		return nil, fmt.Errorf("field %q expects an {avg,min,max} object: %w", key, err)
	}

	for k := range raw {
		if k != "avg" && k != "min" && k != "max" {
			return nil, fmt.Errorf("%w: field %q: unknown price_range key %q", ErrInvalidInput, key, k)
		}
	}

	out := make(map[string]float64, 3)

	for _, k := range []string{"avg", "min", "max"} {
		rv, present := raw[k]
		if !present || string(rv) == "null" {
			continue
		}

		var n float64
		if err := json.Unmarshal(rv, &n); err != nil {
			return nil, fmt.Errorf("field %q: %q must be a number: %w", key, k, err)
		}

		if n < 0 {
			return nil, fmt.Errorf("field %q: %q must not be negative", key, k)
		}

		out[k] = n
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("field %q expects at least one of avg/min/max", key)
	}

	if lo, ok := out["min"]; ok {
		if hi, ok2 := out["max"]; ok2 && lo > hi {
			return nil, fmt.Errorf("field %q: min must not exceed max", key)
		}
	}

	return json.Marshal(out)
}

// normalizePriceParts coerces a set_value payload into the canonical
// price_parts object {"parts": {"<part_key>": <number|string>}} (§80 Wiki
// Topic Pages). The input may be the canonical shape or a bare parts map;
// part keys are validated against the field's options.parts schema —
// unknown keys are rejected, not silently dropped. money/number parts
// coerce to non-negative numerics (JSON numbers or numeric strings); text
// parts must be JSON strings. Fields without a parts schema cannot coerce —
// the schema is required at definition time (ValidOptionsShape), so a
// missing schema fails closed.
func normalizePriceParts(v json.RawMessage, key string, options []byte) (json.RawMessage, error) {
	opts := customfields.PricePartsOptionsFromJSON(options)
	if opts == nil {
		return nil, fmt.Errorf("field %q is price_parts but declares no options.parts schema", key)
	}

	declared := make(map[string]customfields.PricePartsPart, len(opts.Parts))
	for _, p := range opts.Parts {
		declared[p.Key] = p
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(v, &raw); err != nil {
		return nil, fmt.Errorf("field %q expects a {parts:{...}} object: %w", key, err)
	}

	// Accept a bare parts map for submission convenience; when the payload
	// carries a "parts" member it must be the parts object and the only
	// top-level member.
	if inner, ok := raw["parts"]; ok {
		if len(raw) != 1 {
			return nil, fmt.Errorf("field %q: %q carries members alongside parts", key, key)
		}

		raw = nil
		if err := json.Unmarshal(inner, &raw); err != nil {
			return nil, fmt.Errorf("field %q: parts must be an object: %w", key, err)
		}
	}

	out := make(map[string]any, len(raw))

	for pk, rv := range raw {
		part, known := declared[pk]
		if !known {
			return nil, fmt.Errorf("field %q: unknown part %q (not in options.parts)", key, pk)
		}

		if string(rv) == "null" {
			continue
		}

		kind := part.Kind
		if kind == "" {
			kind = customfields.PricePartKindNumber
		}

		if kind == customfields.PricePartKindText {
			var s string
			if err := json.Unmarshal(rv, &s); err != nil {
				return nil, fmt.Errorf("field %q: part %q must be a string: %w", key, pk, err)
			}

			out[pk] = s

			continue
		}

		// money/number parts coerce to a non-negative float — JSON numbers
		// and numeric strings both accepted (set_value submitters quote
		// input-field values).
		var n float64

		var ns string
		if err := json.Unmarshal(rv, &n); err != nil {
			if serr := json.Unmarshal(rv, &ns); serr != nil {
				return nil, fmt.Errorf("field %q: part %q must be a number: %w", key, pk, err)
			}

			n, err = strconv.ParseFloat(strings.TrimSpace(ns), 64)
			if err != nil {
				return nil, fmt.Errorf("field %q: part %q must be a number", key, pk)
			}
		}

		if !finite(n) {
			return nil, fmt.Errorf("field %q: part %q must be a finite number", key, pk)
		}

		if n < 0 {
			return nil, fmt.Errorf("field %q: part %q must not be negative", key, pk)
		}

		out[pk] = n
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("field %q expects at least one part value", key)
	}

	parts, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]json.RawMessage{"parts": parts})
}
