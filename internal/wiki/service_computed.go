// Package wiki — service_computed.go implements options.computed: a
// field whose displayed value is an aggregate of source fields' stored
// cfv on the same entity (§80 page rollup amendment). Read-time only —
// computed values are never persisted, and a declared-computed field
// rejects manual set_value (see checkSetValueFields/applySetValue). The
// fold primitives live in customfields so the tool-hydration overlay
// (EntityFieldValues) produces identical values.
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
		if !customfields.ComputedEligibleField(dtos[i].FieldType, dtos[i].Options) {
			continue // computed is opt-in per aggregate-eligible type
		}
		spec := customfields.ComputedSpecFromOptions(dtos[i].Options)

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
			if f, ok := customfields.ComputedScalarFromValue(v.FieldType, v.FieldOptions, v.ValueJson, v.ValueNumber); ok {
				scalars = append(scalars, f)
			}
		}

		dtos[i].Computed = true
		dtos[i].CanEdit = false
		if len(scalars) == 0 {
			dtos[i].Value = nil
			continue
		}
		dtos[i].Value = computedValueJSON(dtos[i].FieldType, dtos[i].Options, customfields.FoldComputedMetric(spec.Metric, scalars))
	}
}

// computedValueJSON wraps customfields.ComputedDisplayValue in the
// json.RawMessage the field DTO carries — nil-safe for a parts field's
// missing primary_part (unreachable: eligibility already gated it).
func computedValueJSON(fieldType string, options []byte, v float64) json.RawMessage {
	dv := customfields.ComputedDisplayValue(fieldType, options, v)
	if dv == nil {
		return nil
	}
	b, err := json.Marshal(dv)
	if err != nil {
		return nil
	}
	return b
}
