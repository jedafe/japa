package customfields

import (
	"encoding/json"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
)

// computedFieldTypes are the field types a computed spec may aggregate
// (§80 page rollup amendment) — the same set the category rollups fold.
// price_parts additionally requires options.primary_part: a parts field
// without it can't emit a single scalar.
var computedFieldTypes = map[string]bool{
	FieldTypeNumber:     true,
	FieldTypeCurrency:   true,
	FieldTypePercentage: true,
	FieldTypePriceRange: true,
	FieldTypePriceParts: true,
}

// ComputedEligibleField reports whether a field's options.computed spec
// is honored — spec present AND type aggregate-eligible (price_parts
// needs primary_part). Read, write-guard, and hydration paths share this
// predicate so an ineligible spec is ignored everywhere rather than
// producing a field that is neither computed nor manually editable.
func ComputedEligibleField(fieldType string, options []byte) bool {
	if ComputedSpecFromOptions(options) == nil || !computedFieldTypes[fieldType] {
		return false
	}

	if fieldType == FieldTypePriceParts && PrimaryPartFromOptions(options) == "" {
		return false
	}

	return true
}

// ComputedScalarFromValue projects one stored cfv value to the scalar it
// contributes to a computed fold: scalar numerics read value_number;
// price_range reads avg (min then max fallback when avg is undeclared);
// price_parts reads the part named by the field's own
// options.primary_part. Non-eligible or unparseable values report false.
func ComputedScalarFromValue(fieldType string, options, valueJSON []byte, valueNumber pgtype.Numeric) (float64, bool) {
	switch fieldType {
	case FieldTypePriceRange:
		var pr struct {
			Avg *float64 `json:"avg"`
			Min *float64 `json:"min"`
			Max *float64 `json:"max"`
		}

		if json.Unmarshal(valueJSON, &pr) != nil {
			return 0, false
		}

		for _, p := range []*float64{pr.Avg, pr.Min, pr.Max} {
			if p != nil && computedFinite(*p) {
				return *p, true
			}
		}

		return 0, false
	case FieldTypePriceParts:
		pp := PrimaryPartFromOptions(options)
		if pp == "" {
			return 0, false
		}

		var parts struct {
			Parts map[string]json.RawMessage `json:"parts"`
		}

		if json.Unmarshal(valueJSON, &parts) != nil {
			return 0, false
		}

		raw, ok := parts.Parts[pp]
		if !ok {
			return 0, false
		}

		var f float64
		if json.Unmarshal(raw, &f) != nil || !computedFinite(f) {
			return 0, false
		}

		return f, true
	default:
		f, ok := computedNumeric(valueNumber)
		return f, ok && computedFinite(f)
	}
}

// FoldComputedMetric applies a computed spec's metric to the collected
// source scalars — scalars is non-empty by contract (callers guard).
func FoldComputedMetric(metric string, scalars []float64) float64 {
	switch metric {
	case "min":
		m := scalars[0]
		for _, x := range scalars[1:] {
			if x < m {
				m = x
			}
		}

		return m
	case "max":
		m := scalars[0]
		for _, x := range scalars[1:] {
			if x > m {
				m = x
			}
		}

		return m
	case "sum":
		var s float64
		for _, x := range scalars {
			s += x
		}

		return s
	default: // "avg" — also the parser's empty-metric default
		var sum float64
		for _, x := range scalars {
			sum += x
		}

		return sum / float64(len(scalars))
	}
}

// ComputedDisplayValue renders the aggregate in the field type's native
// display shape so value consumers work unchanged: scalar types get a
// bare number, price_range gets {"avg":v}, price_parts gets
// {"parts":{primary:v}} using the field's own options.primary_part.
func ComputedDisplayValue(fieldType string, options []byte, v float64) any {
	switch fieldType {
	case FieldTypePriceRange:
		return map[string]any{"avg": v}
	case FieldTypePriceParts:
		pp := PrimaryPartFromOptions(options)
		if pp == "" {
			return nil
		}

		return map[string]any{"parts": map[string]any{pp: v}}
	default:
		return v
	}
}

// computedFinite rejects NaN/Inf — pg numeric accepts 'NaN', and a
// non-finite aggregate fails JSON serialization downstream.
func computedFinite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func computedNumeric(n pgtype.Numeric) (float64, bool) {
	if !n.Valid {
		return 0, false
	}

	if f, err := n.Float64Value(); err == nil && f.Valid {
		return f.Float64, true
	}

	return 0, false
}
