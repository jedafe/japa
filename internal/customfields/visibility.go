package customfields

import (
	"encoding/json"
	"strings"

	"github.com/jedafe/japa/internal/formlogic"
)

// canonicalAnswers maps submitted values (label- OR key-keyed) onto field
// keys and normalizes select/multiselect answers to canonical option
// values, so rules compare against option values whether the client sent
// labels (embed renderer) or values (assessment). Storage keeps the
// submitted key style — this view is eval-only.
// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md (ADR-137).
func canonicalAnswers(fields []fieldDef, vals map[string]any) map[string]any {
	out := make(map[string]any, len(vals))
	for k, v := range vals {
		out[k] = v
	}

	for _, f := range fields {
		if v, ok := vals[f.Label]; ok {
			out[f.Key] = v
		}
	}

	for _, f := range fields {
		v, ok := out[f.Key]
		if !ok {
			continue
		}

		out[f.Key] = canonicalFieldValue(f, v)
	}

	return out
}

// canonicalFieldValue normalizes one answer for rule evaluation — choice
// fields resolve labels to option values; other types pass through.
func canonicalFieldValue(f fieldDef, v any) any {
	switch f.FieldType {
	case FieldTypeSelect:
		if s, isStr := v.(string); isStr && s != "" {
			return NormalizeChoiceValue(f.Options, s)
		}
	case FieldTypeMultiselect:
		return canonicalMulti(f, v)
	}

	return v
}

// canonicalMulti resolves each element of a multiselect answer to its option
// value — the payload is either a []any (assessment) or a comma-joined
// string (generic embed renderer).
func canonicalMulti(f fieldDef, v any) any {
	switch arr := v.(type) {
	case string:
		parts := strings.Split(arr, ",")

		norm := make([]any, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				norm = append(norm, NormalizeChoiceValue(f.Options, t))
			}
		}

		return norm
	case []any:
		norm := make([]any, len(arr))
		for i, item := range arr {
			if s, isStr := item.(string); isStr {
				norm[i] = NormalizeChoiceValue(f.Options, s)
			} else {
				norm[i] = item
			}
		}

		return norm
	}

	return v
}

// stepDef is the minimal step shape the hidden-field pass needs — the step's
// ID (jump target + field StepKey match) and its raw config (visible_if).
type stepDef struct {
	ID     string
	Config []byte
}

// hiddenFieldKeys returns the keys of fields hidden under canon — shared by
// the required-check skip and the hidden-answer drop. A field is hidden when
// its own visible_if fails OR when its containing step is hidden (step
// visible_if, a triggered forward jump, or a step left with no visible
// fields), mirroring the client-side path so the server never requires what
// the user could not see.
func hiddenFieldKeys(fields []fieldDef, steps []stepDef, canon map[string]any) map[string]bool {
	hidden := make(map[string]bool, len(fields))
	for _, f := range fields {
		if !formlogic.FieldVisible(f.Config, canon, f.Key) {
			hidden[f.Key] = true
		}
	}

	for sk := range hiddenStepKeys(fields, steps, canon) {
		for _, f := range fields {
			if f.StepKey == sk {
				hidden[f.Key] = true
			}
		}
	}

	return hidden
}

// hiddenStepKeys returns the step IDs hidden by a step's own visible_if or
// by a triggered forward jump interval. The auto-hide rule (a step whose
// defined fields are all hidden) needs no handling here — those fields are
// already individually hidden, so hiding the step adds nothing to the set.
func hiddenStepKeys(fields []fieldDef, steps []stepDef, canon map[string]any) map[string]bool {
	if len(steps) == 0 {
		return nil
	}

	order := make([]string, 0, len(steps))
	for _, st := range steps {
		order = append(order, st.ID)
	}

	ruleFields := make([]formlogic.RuleField, 0, len(fields))
	for _, f := range fields {
		ruleFields = append(ruleFields, formlogic.RuleField{Key: f.Key, StepKey: f.StepKey, Config: f.Config})
	}

	hidden := formlogic.HiddenByJump(ruleFields, order, canon)
	for _, st := range steps {
		if !formlogic.StepVisible(st.Config, canon) {
			hidden[st.ID] = true
		}
	}

	return hidden
}

// dropHiddenValues removes entries whose field's visible_if fails against
// the canonical answers; matched by label OR key so the stored snapshot is
// the visible-path truth.
func dropHiddenValues(fields []fieldDef, steps []stepDef, vals map[string]any) map[string]any {
	hidden := hiddenFieldKeys(fields, steps, canonicalAnswers(fields, vals))

	byKeyOrLabel := make(map[string]fieldDef, len(fields)*2)
	for _, f := range fields {
		byKeyOrLabel[f.Key] = f
		byKeyOrLabel[f.Label] = f
	}

	out := make(map[string]any, len(vals))
	for k, v := range vals {
		if f, ok := byKeyOrLabel[k]; ok && hidden[f.Key] {
			continue
		}

		out[k] = v
	}

	return out
}

// logicConfigJSON projects the public-safe slice of custom_fields.config:
// visible_if + jump only. Scoring keys (weight, option_scores) never ship
// on public payloads.
func logicConfigJSON(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	var cfg struct {
		VisibleIf json.RawMessage `json:"visible_if"`
		Jump      json.RawMessage `json:"jump"`
	}
	if json.Unmarshal(raw, &cfg) != nil || (cfg.VisibleIf == nil && cfg.Jump == nil) {
		return nil
	}

	out, _ := json.Marshal(map[string]json.RawMessage{"visible_if": cfg.VisibleIf, "jump": cfg.Jump})

	return out
}
