package customfields

import (
	"encoding/json"
)

// Canonical choice handling for select/multiselect fields.
//
// Canonical contract: matching is value-only. Option labels are a display
// concern — every write path normalizes a submitted label to its option
// value before matching and before storage, so readers (cfvValueJSON,
// formatWikiFieldValue, choiceValueJSON) only ever see values. This keeps
// the REST value handler, form submissions, and wiki field ops consistent.

// selectOptionIndex parses a [{value,label}] options payload into the
// canonical value set plus a label→value map for write-time normalization.
// Nil maps when options are absent or malformed (fail-open to empty).
func selectOptionIndex(options []byte) (values map[string]bool, labelToValue map[string]string) {
	values = map[string]bool{}
	labelToValue = map[string]string{}

	var opts []FieldOption
	if err := json.Unmarshal(options, &opts); err != nil {
		return values, labelToValue
	}

	for _, o := range opts {
		if o.Value == "" {
			continue
		}
		values[o.Value] = true
		if o.Label != "" {
			if _, taken := labelToValue[o.Label]; !taken {
				labelToValue[o.Label] = o.Value
			}
		}
	}

	return values, labelToValue
}

// NormalizeChoiceValue maps one submitted select value to its canonical
// option value: exact values pass through, a known label resolves to its
// value, and unknown input is returned unchanged (callers reject it).
func NormalizeChoiceValue(options []byte, input string) string {
	values, byLabel := selectOptionIndex(options)
	if values[input] {
		return input
	}
	if v, ok := byLabel[input]; ok {
		return v
	}
	return input
}

// ChoiceStrings unwraps a select/multiselect JSON payload in any accepted
// write shape into its candidate strings: a bare string, a bare array of
// strings, or the {"value": …} / {"values": […]} envelopes (including a
// {"value": […]} array for select, which degrades to its members).
// Non-string members are skipped; unparsable input yields nil.
func ChoiceStrings(raw json.RawMessage) []string {
	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}

	if m, ok := probe.(map[string]any); ok {
		if inner, exists := m["values"]; exists {
			probe = inner
		} else if inner, exists := m["value"]; exists {
			probe = inner
		} else {
			return nil
		}
	}

	collect := func(arr []any) []string {
		out := make([]string, 0, len(arr))
		for _, el := range arr {
			if s, ok := el.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}

	switch t := probe.(type) {
	case string:
		return []string{t}
	case []any:
		return collect(t)
	default:
		return nil
	}
}

// CanonicalizeSelectJSON rewrites every choice string in a
// select/multiselect payload from label to value, preserving the envelope
// shape (bare or wrapped) so writers converge on canonical values without
// changing the read contract. Unparsable input is returned unchanged.
func CanonicalizeSelectJSON(fieldType string, options []byte, raw json.RawMessage) json.RawMessage {
	_ = fieldType

	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw
	}

	mapStrings := func(v any) any {
		switch t := v.(type) {
		case string:
			return NormalizeChoiceValue(options, t)
		case []any:
			out := make([]any, len(t))
			for i, el := range t {
				if s, ok := el.(string); ok {
					out[i] = NormalizeChoiceValue(options, s)
				} else {
					out[i] = el
				}
			}
			return out
		default:
			return v
		}
	}

	switch t := probe.(type) {
	case string:
		probe = NormalizeChoiceValue(options, t)
	case []any:
		probe = mapStrings(t)
	case map[string]any:
		if inner, exists := t["values"]; exists {
			t["values"] = mapStrings(inner)
		} else if inner, exists := t["value"]; exists {
			t["value"] = mapStrings(inner)
		}
		probe = t
	default:
		return raw
	}

	if out, err := json.Marshal(probe); err == nil {
		return out
	}
	return raw
}

// sanitizeValueStrings walks decoded JSON in place, replacing every string
// leaf with sanitize(s). Used to scrub rich_text HTML before it is
// marshaled for storage.
func sanitizeValueStrings(v any, sanitize func(string) string) any {
	switch t := v.(type) {
	case string:
		return sanitize(t)
	case []any:
		for i, el := range t {
			t[i] = sanitizeValueStrings(el, sanitize)
		}
		return t
	case map[string]any:
		for k, el := range t {
			t[k] = sanitizeValueStrings(el, sanitize)
		}
		return t
	default:
		return v
	}
}

// SanitizeRichTextJSON scrubs every string leaf of a rich_text value_json
// payload with sanitize (content.SanitizeHTML in production). Invalid JSON
// is returned unchanged — callers validate shape separately.
func SanitizeRichTextJSON(raw []byte, sanitize func(string) string) []byte {
	if len(raw) == 0 || sanitize == nil {
		return raw
	}

	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw
	}

	if out, err := json.Marshal(sanitizeValueStrings(probe, sanitize)); err == nil {
		return out
	}
	return raw
}
