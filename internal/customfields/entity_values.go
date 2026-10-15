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

	sources := make([]cfvComputedSource, 0, len(rows))
	for _, row := range rows {
		if row.FieldKey == "" {
			continue
		}

		if v := FieldValueFromDefRow(row.ValueText, row.ValueNumber, row.ValueBool, row.ValueDate, row.ValueJson, row.FieldType); v != nil {
			values[row.FieldKey] = v
		}
		sources = append(sources, cfvComputedSource{
			groupID: row.FieldGroupID, key: row.FieldKey, typ: row.FieldType,
			options: row.FieldOptions, valueJSON: row.ValueJson, valueNumber: row.ValueNumber,
		})
	}

	if overlay := loadComputedOverlay(ctx, q); overlay != nil {
		overlay.apply(sources, values)
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

	sourcesByEntity := make(map[pgtype.UUID][]cfvComputedSource, len(entityIDs))
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
		sourcesByEntity[row.EntityID] = append(sourcesByEntity[row.EntityID], cfvComputedSource{
			groupID: row.FieldGroupID, key: row.FieldKey, typ: row.FieldType,
			options: row.FieldOptions, valueJSON: row.ValueJson, valueNumber: row.ValueNumber,
		})
	}

	// Overlay computed values per entity — an entity with no cfv rows has
	// no sources to fold, so only existing value maps need visiting.
	if overlay := loadComputedOverlay(ctx, q); overlay != nil {
		for entityID, values := range out {
			overlay.apply(sourcesByEntity[entityID], values)
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

// cfvComputedSource is the normalized projection of one cfv row for the
// computed overlay — the two generated row shapes (single-entity and
// batched) feed the same fold.
type cfvComputedSource struct {
	groupID     pgtype.UUID
	key         string
	typ         string
	options     []byte
	valueJSON   []byte
	valueNumber pgtype.Numeric
}

// computedOverlay carries the global computed-field set plus the
// group-slug → id memo so the batched hydration read resolves each
// spec's sources once per call, not once per entity per source.
type computedOverlay struct {
	ctx      context.Context
	q        *gen.Queries
	defs     []gen.ListComputedFieldDefsRow
	groupIDs map[string]pgtype.UUID
}

// loadComputedOverlay fetches every field declaring options.computed —
// a small global set (§80 page rollup amendment). Returns nil when the
// query handle is absent, the lookup fails, or nothing is computed;
// callers then emit the raw read untouched.
func loadComputedOverlay(ctx context.Context, q *gen.Queries) *computedOverlay {
	if q == nil {
		return nil
	}
	defs, err := q.ListComputedFieldDefs(ctx)
	if err != nil || len(defs) == 0 {
		return nil
	}
	return &computedOverlay{ctx: ctx, q: q, defs: defs, groupIDs: make(map[string]pgtype.UUID)}
}

// apply folds each eligible computed field's sources against the
// entity's stored rows and replaces the keyed display value — a stale
// stored cfv never shadows the computed contract, so a field with zero
// usable sources deletes its key instead of falling back (matching the
// wiki page read path). No per-field can_view gate: this hydration read
// already merges all cfv unauthenticated.
func (o *computedOverlay) apply(sources []cfvComputedSource, values map[string]any) {
	byCoord := make(map[string]cfvComputedSource, len(sources))
	for _, s := range sources {
		byCoord[s.groupID.String()+"/"+s.key] = s
	}

	for _, def := range o.defs {
		if !ComputedEligibleField(def.FieldType, def.Options) {
			continue
		}
		spec := ComputedSpecFromOptions(def.Options)

		scalars := make([]float64, 0, len(spec.Sources))
		for _, src := range spec.Sources {
			gid := o.resolveGroup(src.Group)
			if !gid.Valid {
				continue
			}
			if row, ok := byCoord[gid.String()+"/"+src.Key]; ok {
				if f, ok := ComputedScalarFromValue(row.typ, row.options, row.valueJSON, row.valueNumber); ok {
					scalars = append(scalars, f)
				}
			}
		}

		if len(scalars) == 0 {
			delete(values, def.Key)
			continue
		}
		values[def.Key] = ComputedDisplayValue(def.FieldType, def.Options, FoldComputedMetric(spec.Metric, scalars))
	}
}

// resolveGroup memoizes group slug → id; failures cache the invalid
// UUID so an unknown slug is skipped, not re-queried per source.
func (o *computedOverlay) resolveGroup(slug string) pgtype.UUID {
	if gid, ok := o.groupIDs[slug]; ok {
		return gid
	}
	g, err := o.q.GetCustomFieldGroupBySlug(o.ctx, slug)
	if err != nil {
		o.groupIDs[slug] = pgtype.UUID{}
		return pgtype.UUID{}
	}
	o.groupIDs[slug] = g.ID
	return g.ID
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
