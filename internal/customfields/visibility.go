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
// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md (ADR-137)
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
		switch f.FieldType {
		case FieldTypeSelect:
			if s, isStr := v.(string); isStr && s != "" {
				out[f.Key] = NormalizeChoiceValue(f.Options, s)
			}
		case FieldTypeMultiselect:
			switch arr := v.(type) {
			case string: // embed renderer submits comma-joined values
				parts := strings.Split(arr, ",")
				norm := make([]any, 0, len(parts))
				for _, p := range parts {
					if t := strings.TrimSpace(p); t != "" {
						norm = append(norm, NormalizeChoiceValue(f.Options, t))
					}
				}
				out[f.Key] = norm
			case []any:
				norm := make([]any, len(arr))
				for i, item := range arr {
					if s, isStr := item.(string); isStr {
						norm[i] = NormalizeChoiceValue(f.Options, s)
					} else {
						norm[i] = item
					}
				}
				out[f.Key] = norm
			}
		}
	}
	return out
}

// hiddenFieldKeys returns the keys of fields whose visible_if fails under
// canon — shared by the required-check skip and the hidden-answer drop.
func hiddenFieldKeys(fields []fieldDef, canon map[string]any) map[string]bool {
	hidden := make(map[string]bool, len(fields))
	for _, f := range fields {
		if !formlogic.FieldVisible(f.Config, canon, f.Key) {
			hidden[f.Key] = true
		}
	}
	return hidden
}

// dropHiddenValues removes entries whose field's visible_if fails against
// the canonical answers; matched by label OR key so the stored snapshot is
// the visible-path truth.
func dropHiddenValues(fields []fieldDef, vals map[string]any) map[string]any {
	hidden := hiddenFieldKeys(fields, canonicalAnswers(fields, vals))
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
