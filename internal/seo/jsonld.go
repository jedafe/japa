package seo

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JSONLDResolver resolves a dynamic, admin-managed JSON-LD template for a given
// content/entity type by interpolating field values into the {{field_key}}
// placeholders (Section 14 — content_type_jsonld_templates, Section 26 — SEO
// structured-data pipeline). Implementations fetch the active template from
// the content_type_jsonld_templates table and call InterpolateJSONLD.
type JSONLDResolver interface {
	// ResolveJSONLD returns the interpolated JSON-LD for the given content
	// type. A nil/empty result (with nil error) means no active template
	// exists for the content type — callers should fall back to a hardcoded
	// generator.
	ResolveJSONLD(contentType string, fields map[string]string) (json.RawMessage, error)
}

// InterpolateJSONLD parses a JSON-LD template body and replaces every
// {{field_key}} token found inside string values with the corresponding entry
// from fields. Tokens without a matching field are replaced with an empty
// string so the emitted JSON never leaks raw placeholders to crawlers. The
// template structure (objects, arrays, nesting) is preserved; only string
// scalar values are interpolated. Non-string values (numbers, booleans, null)
// are passed through unchanged.
//
// The returned RawMessage is valid JSON ready to embed in an API response or a
// <script type="application/ld+json"> tag. An empty/nil template yields nil.
func InterpolateJSONLD(template []byte, fields map[string]string) (json.RawMessage, error) {
	if len(template) == 0 {
		return nil, nil
	}

	var node any
	if err := json.Unmarshal(template, &node); err != nil {
		return nil, fmt.Errorf("parsing JSON-LD template: %w", err)
	}

	interpolated := interpolateNode(node, fields)

	out, err := json.Marshal(interpolated)
	if err != nil {
		return nil, fmt.Errorf("marshalling interpolated JSON-LD: %w", err)
	}
	return out, nil
}

// interpolateNode walks a decoded JSON value recursively, interpolating
// {{field_key}} tokens inside every string it encounters.
func interpolateNode(node any, fields map[string]string) any {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			v[key] = interpolateNode(val, fields)
		}
		return v
	case []any:
		for i, val := range v {
			v[i] = interpolateNode(val, fields)
		}
		return v
	case string:
		return interpolateString(v, fields)
	default:
		return node
	}
}

// interpolateString replaces every {{key}} token in s with fields[key]. A
// missing key yields an empty string. Tokens are matched greedily and may
// appear multiple times in the same string.
func interpolateString(s string, fields map[string]string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		start := strings.Index(s[i:], "{{")
		if start < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+start])
		end := strings.Index(s[i+start:], "}}")
		if end < 0 {
			b.WriteString(s[i+start:])
			break
		}
		key := strings.TrimSpace(s[i+start+2 : i+start+end])
		b.WriteString(fields[key])
		i += start + end + 2
	}
	return b.String()
}
