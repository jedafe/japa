package customfields

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// EntityFieldValues returns one entity's display-ready custom field values
// keyed by field key — the canonical single-entity read for the §80 Tool
// Surface Hydration merge. The entity type is normalized through
// ValueEntityType so space-backed callers may pass "university"/"city"/etc.
// and still read the shared 'space' bucket. Returns nil when the query
// handle is nil, the ID is invalid, or the lookup fails — callers treat the
// merge as best-effort enrichment and never fail a read on it.
func EntityFieldValues(ctx context.Context, q *gen.Queries, entityType string, entityID pgtype.UUID) map[string]any {
	if q == nil || !entityID.Valid {
		return nil
	}

	rows, err := q.ListCustomFieldValuesWithDefsByEntity(ctx, gen.ListCustomFieldValuesWithDefsByEntityParams{
		EntityType: ValueEntityType(entityType),
		EntityID:   entityID,
	})
	if err != nil || len(rows) == 0 {
		return nil
	}

	values := make(map[string]any, len(rows))

	for _, row := range rows {
		if row.FieldKey == "" {
			continue
		}

		if v := FieldValueFromDefRow(row.ValueText, row.ValueNumber, row.ValueBool, row.ValueDate, row.ValueJson, row.FieldType); v != nil {
			values[row.FieldKey] = v
		}
	}

	return values
}

// EntityFieldValuesForEntities is the batched counterpart of
// EntityFieldValues — one query returns display-ready field values for many
// entities of the same type so list endpoints merge community values without
// per-row queries (§80 Tool Surface Hydration). Returns a map keyed by
// entity_id; entities with no stored values are absent. Best-effort: a nil
// query handle, empty ID list, or lookup error yields an empty map.
func EntityFieldValuesForEntities(ctx context.Context, q *gen.Queries, entityType string, entityIDs []pgtype.UUID) map[pgtype.UUID]map[string]any {
	out := make(map[pgtype.UUID]map[string]any)

	if q == nil || len(entityIDs) == 0 {
		return out
	}

	rows, err := q.ListCustomFieldValuesWithDefsForEntities(ctx, gen.ListCustomFieldValuesWithDefsForEntitiesParams{
		EntityType: ValueEntityType(entityType),
		EntityIds:  entityIDs,
	})
	if err != nil {
		return out
	}

	for _, row := range rows {
		if row.FieldKey == "" {
			continue
		}

		if v := FieldValueFromDefRow(row.ValueText, row.ValueNumber, row.ValueBool, row.ValueDate, row.ValueJson, row.FieldType); v != nil {
			if out[row.EntityID] == nil {
				out[row.EntityID] = make(map[string]any)
			}

			out[row.EntityID][row.FieldKey] = v
		}
	}

	return out
}

// FieldValueFromDefRow extracts the effective value from a custom field
// values row given its five storage columns and the field's declared type —
// the typed column wins, with a fallback to the first populated column for
// legacy or unmapped field types. Shared by the single-entity and batched
// reads above, and usable by any caller holding cfv rows from another query.
func FieldValueFromDefRow(valueText *string, valueNumber pgtype.Numeric, valueBool *bool, valueDate pgtype.Timestamptz, valueJSON []byte, fieldType string) any {
	col, _ := FieldTypeValueColumn(fieldType)

	var v any

	switch col {
	case "value_number":
		v = cfvNumberValue(valueNumber)
	case "value_bool":
		if valueBool != nil {
			v = *valueBool
		}
	case "value_date":
		if valueDate.Valid {
			v = valueDate.Time.Format(time.RFC3339)
		}
	case "value_json":
		v = cfvJSONValue(valueJSON)
	default:
		if valueText != nil {
			v = *valueText
		}
	}

	if v != nil {
		return v
	}

	// Fallback: first populated column so unmapped field types still expose
	// their stored value.
	if valueText != nil {
		return *valueText
	}

	if n := cfvNumberValue(valueNumber); n != nil {
		return n
	}

	if valueBool != nil {
		return *valueBool
	}

	if valueDate.Valid {
		return valueDate.Time.Format(time.RFC3339)
	}

	return cfvJSONValue(valueJSON)
}

func cfvNumberValue(n pgtype.Numeric) any {
	if !n.Valid {
		return nil
	}

	if f, err := n.Float64Value(); err == nil && f.Valid {
		return f.Float64
	}

	return nil
}

func cfvJSONValue(b []byte) any {
	if len(b) == 0 {
		return nil
	}

	var m any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}

	return m
}
