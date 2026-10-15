// Package wiki — service_computed.go implements options.computed: a
// field whose displayed value is an aggregate of source fields' stored
// cfv on the same entity (§80 page rollup amendment). Read-time only —
// computed values are never persisted, and a declared-computed field
// rejects manual set_value (see checkSetValueFields/applySetValue).
package wiki

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// applyComputedFields post-processes a rendered field list: each field
// whose options declare a computed spec gets its Value folded from the
// sources' stored values, Computed flagged, and CanEdit cleared (the
// field is derived — there is nothing to edit). Sources the caller
// cannot can_view contribute nothing (fail-closed, same contract as the
// rollups). Mutates dtos in place; call after Visible is populated and
// before fieldSections so the computed values land in both views.
func (s *Service) applyComputedFields(ctx context.Context, dtos []EntityFieldDTO, values []gen.ListCustomFieldValuesWithDefsByEntityRow, visibleSet map[string]bool, canManage bool) {
	if s.fields == nil || len(dtos) == 0 {
		return
	}

	// Index the entity's stored values by group+key — the spec's lookup
	// coordinate after group-slug resolution.
	bySource := make(map[string]gen.ListCustomFieldValuesWithDefsByEntityRow, len(values))
	for _, v := range values {
		bySource[v.FieldGroupID.String()+"/"+v.FieldKey] = v
	}

	groupIDs := map[string]pgtype.UUID{} // slug → id, memoized per call
	sourceVisible := func(id pgtype.UUID) bool {
		if canManage {
			return true
		}
		return visibleSet != nil && visibleSet[id.String()]
	}

	for i := range dtos {
		if !dtos[i].Visible {
			continue
		}
		spec := customfields.ComputedSpecFromOptions(dtos[i].Options)
		if spec == nil || !rollupFieldTypes[dtos[i].FieldType] {
			continue // computed is opt-in per aggregate-eligible type
		}
		if dtos[i].FieldType == customfields.FieldTypePriceParts &&
			customfields.PrimaryPartFromOptions(dtos[i].Options) == "" {
			continue // a parts field with no primary_part can't emit a scalar
		}

		scalars := []float64{}
		for _, src := range spec.Sources {
			gid, ok := groupIDs[src.Group]
			if !ok {
				g, err := s.fields.GetFieldGroupBySlug(ctx, src.Group)
				if err != nil {
					slog.Warn("wiki: computed field: source group unresolved",
						"group", src.Group, "error", err)
					groupIDs[src.Group] = pgtype.UUID{}
					continue
				}
				gid = g.ID
				groupIDs[src.Group] = gid
			}
			if !gid.Valid {
				continue
			}
			v, ok := bySource[gid.String()+"/"+src.Key]
			if !ok || !sourceVisible(v.FieldID) {
				continue
			}
			if f, ok := cfvSourceScalar(v); ok {
				scalars = append(scalars, f)
			}
		}

		dtos[i].Computed = true
		dtos[i].CanEdit = false
		if len(scalars) == 0 {
			dtos[i].Value = nil
			continue
		}
		dtos[i].Value = computedValueJSON(dtos[i].FieldType, dtos[i].Options, foldComputed(spec.Metric, scalars))
	}
}

// cfvSourceScalar projects one stored source value to the scalar it
// contributes: scalar numerics read value_number; price_range reads avg
// (min then max fallback when avg is undeclared); price_parts reads the
// part named by the source field's own options.primary_part.
func cfvSourceScalar(v gen.ListCustomFieldValuesWithDefsByEntityRow) (float64, bool) {
	switch v.FieldType {
	case customfields.FieldTypePriceRange:
		var pr struct {
			Avg *float64 `json:"avg"`
			Min *float64 `json:"min"`
			Max *float64 `json:"max"`
		}
		if json.Unmarshal(v.ValueJson, &pr) != nil {
			return 0, false
		}
		for _, p := range []*float64{pr.Avg, pr.Min, pr.Max} {
			if p != nil && finite(*p) {
				return *p, true
			}
		}
		return 0, false
	case customfields.FieldTypePriceParts:
		pp := customfields.PrimaryPartFromOptions(v.FieldOptions)
		if pp == "" {
			return 0, false
		}
		var parts struct {
			Parts map[string]json.RawMessage `json:"parts"`
		}
		if json.Unmarshal(v.ValueJson, &parts) != nil {
			return 0, false
		}
		raw, ok := parts.Parts[pp]
		if !ok {
			return 0, false
		}
		var f float64
		if json.Unmarshal(raw, &f) != nil || !finite(f) {
			return 0, false
		}
		return f, true
	default:
		f, ok := cfvNumeric(v.ValueNumber)
		return f, ok && finite(f)
	}
}

// foldComputed applies the spec's metric to the collected source scalars.
// scalars is non-empty by contract (callers guard before folding).
func foldComputed(metric string, scalars []float64) float64 {
	switch metric {
	case "min":
		return minOf(scalars)
	case "max":
		return maxOf(scalars)
	case "sum":
		var s float64
		for _, x := range scalars {
			s += x
		}
		return s
	default: // "avg"
		return avgOf(scalars)
	}
}

// computedValueJSON renders the aggregate in the field type's storage
// shape so every existing value renderer works unchanged: scalar types
// get a bare number, price_range gets {"avg":v}, price_parts gets
// {"parts":{primary:v}} using the field's own options.primary_part.
func computedValueJSON(fieldType string, options []byte, v float64) json.RawMessage {
	switch fieldType {
	case customfields.FieldTypePriceRange:
		b, _ := json.Marshal(map[string]float64{"avg": v})
		return b
	case customfields.FieldTypePriceParts:
		pp := customfields.PrimaryPartFromOptions(options)
		if pp == "" {
			return nil
		}
		b, _ := json.Marshal(map[string]any{"parts": map[string]float64{pp: v}})
		return b
	default:
		b, _ := json.Marshal(v)
		return b
	}
}
