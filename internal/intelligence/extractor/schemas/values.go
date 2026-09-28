package schemas

import "encoding/json"

// HasExtractedValues reports whether a marshalled intel_records.extracted
// payload contains at least one field with a non-null value.
//
// The canonical shape is {field: {value, confidence, evidence}} produced by
// the extractor and curators, but flat seed/legacy payloads ({field: raw})
// are also recognised: any top-level scalar or array leaf counts as a value.
// A payload that is unparseable is reported as having values so callers fail
// open toward review rather than silently discarding data they cannot read.
func HasExtractedValues(raw []byte) bool {
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return true
	}

	for _, v := range fields {
		if m, ok := v.(map[string]any); ok {
			if val, has := m["value"]; has {
				if val != nil {
					return true
				}
				continue
			}
			// Object leaf without a "value" key (e.g. a nested jsonb_object
			// field emitted flat) counts as data.
			return true
		}

		if v != nil {
			return true
		}
	}

	return false
}
