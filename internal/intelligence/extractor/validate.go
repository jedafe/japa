// Package extractor implements the AI extraction pipeline: it calls the
// §20 AI gateway to extract structured scholarship fields from source text,
// parses the LLM JSON output, and validates it against the entity schema.
//
// The validation step is the hallucination guard: any non-null value that
// lacks supporting evidence is treated as a hallucination and reset to null.
package extractor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
)

// FieldExtraction is the per-field {value, confidence, evidence} triple
// returned by the LLM and checked by Validate.
type FieldExtraction struct {
	Value      any     `json:"value"`
	Confidence float64 `json:"confidence"`
	Evidence   *string `json:"evidence"`
}

// Extraction is the full LLM output — a map of field name to FieldExtraction.
type Extraction map[string]FieldExtraction

// ValidationError describes a validation failure produced by the
// hallucination guard or schema checks.
type ValidationError struct {
	Field   string
	Message string
}

// Validate checks an LLM extraction against the entity schema and enforces
// the hallucination guard. It returns the cleaned extraction (hallucinated
// fields reset to null) and any validation errors.
//
// Rules:
//   - Non-null value with nil/empty evidence  -> reset to null, record error.
//   - Null value with non-empty evidence      -> clear evidence, record error.
//   - Confidence outside [0,1]                -> clamp to 0, record error.
//   - Required field missing                  -> record error.
//   - Type coercion per spec.Type (date, country_array, enum, numeric).
func Validate(ext Extraction, schema schemas.EntitySchema) (Extraction, []ValidationError) {
	var errs []ValidationError

	cleaned := make(Extraction, len(schema.Fields))

	for _, spec := range schema.Fields {
		field, ok := ext[spec.Name]
		if !ok {
			cleaned[spec.Name] = FieldExtraction{Value: nil, Confidence: 0, Evidence: nil}

			if spec.Required {
				errs = append(errs, ValidationError{spec.Name, "required field missing"})
			}

			continue
		}

		// Hallucination guard: non-null value MUST have non-empty evidence.
		if field.Value != nil {
			if field.Evidence == nil || strings.TrimSpace(*field.Evidence) == "" {
				errs = append(errs, ValidationError{spec.Name, "non-null value without evidence — treated as hallucination, reset to null"})
				cleaned[spec.Name] = FieldExtraction{Value: nil, Confidence: 0, Evidence: nil}

				continue
			}
		} else {
			// value is null -> evidence MUST be null (consistency).
			if field.Evidence != nil && strings.TrimSpace(*field.Evidence) != "" {
				errs = append(errs, ValidationError{spec.Name, "null value with non-empty evidence — inconsistent, evidence cleared"})
				field.Evidence = nil
			}
		}

		// Confidence sanity: out-of-range confidence is clamped to 0.
		if field.Confidence < 0 || field.Confidence > 1 {
			errs = append(errs, ValidationError{spec.Name, fmt.Sprintf("confidence %f out of [0,1] — clamped to 0", field.Confidence)})
			field.Confidence = 0
		}

		// Type coercion / validation per spec.Type.
		if field.Value != nil {
			coerced, coerceErr := coerceType(field.Value, spec)
			if coerceErr != nil {
				errs = append(errs, ValidationError{spec.Name, coerceErr.Error()})
				cleaned[spec.Name] = FieldExtraction{Value: nil, Confidence: 0, Evidence: nil}

				continue
			}

			field.Value = coerced
		}

		cleaned[spec.Name] = field
	}

	return cleaned, errs
}

// coercer coerces a raw LLM value into the Go representation expected for the
// field's declared type, or returns an error describing why it cannot be.
type coercer func(value any, spec schemas.FieldSpec) (any, error)

// typeCoercers dispatches on FieldSpec.Type; unlisted types pass through
// unchanged.
var typeCoercers = map[string]coercer{
	"date":          coerceDate,
	"country_array": stringArrayCoercer("country_array"),
	"text_array":    stringArrayCoercer("text_array"),
	"numeric":       coerceNumeric,
	"string":        coerceString,
	"jsonb_object":  coerceJSONBObject,
}

// coerceType performs basic type coercion/validation for the field's declared
// type. It returns the coerced value, or an error describing why the value
// could not be coerced (in which case the caller resets the field to null).
func coerceType(value any, spec schemas.FieldSpec) (any, error) {
	if c, ok := typeCoercers[spec.Type]; ok {
		return c(value, spec)
	}

	return value, nil
}

// coerceDate validates an ISO 8601 date string.
func coerceDate(value any, spec schemas.FieldSpec) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("date field %q: expected ISO 8601 string, got %T", spec.Name, value)
	}

	if _, err := time.Parse("2006-01-02", s); err != nil {
		return nil, fmt.Errorf("date field %q: %q is not a valid ISO 8601 date", spec.Name, s)
	}

	return s, nil
}

// stringArrayCoercer returns a coercer for array-of-string field types. Some
// LLMs emit a single string instead of an array; it is accepted and wrapped.
func stringArrayCoercer(label string) coercer {
	return func(value any, spec schemas.FieldSpec) (any, error) {
		arr, ok := value.([]any)
		if !ok {
			if s, ok2 := value.(string); ok2 {
				return []string{s}, nil
			}

			return nil, fmt.Errorf("%s field %q: expected array, got %T", label, spec.Name, value)
		}

		out := make([]string, 0, len(arr))

		for _, el := range arr {
			s, ok := el.(string)
			if !ok {
				return nil, fmt.Errorf("%s field %q: element is not a string (%T)", label, spec.Name, el)
			}

			out = append(out, s)
		}

		return out, nil
	}
}

// coerceNumeric coerces numeric JSON values (and numeric strings) to float64.
func coerceNumeric(value any, spec schemas.FieldSpec) (any, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return nil, fmt.Errorf("numeric field %q: %w", spec.Name, err)
		}

		return f, nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err != nil {
			return nil, fmt.Errorf("numeric field %q: %q is not a number", spec.Name, v)
		}

		return f, nil
	default:
		return nil, fmt.Errorf("numeric field %q: unsupported type %T", spec.Name, value)
	}
}

// coerceString validates a plain string field, enforcing EnumValues when the
// schema declares them.
func coerceString(value any, spec schemas.FieldSpec) (any, error) {
	if len(spec.EnumValues) > 0 {
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("enum field %q: expected string, got %T", spec.Name, value)
		}

		valid := false

		for _, ev := range spec.EnumValues {
			if s == ev {
				valid = true
				break
			}
		}

		if !valid {
			return nil, fmt.Errorf("enum field %q: %q not in allowed values %v", spec.Name, s, spec.EnumValues)
		}

		return s, nil
	}

	if _, ok := value.(string); !ok {
		return nil, fmt.Errorf("string field %q: expected string, got %T", spec.Name, value)
	}

	return value, nil
}

// coerceJSONBObject accepts any non-nil value as a JSON object; the storage
// layer serializes it to JSONB.
func coerceJSONBObject(value any, _ schemas.FieldSpec) (any, error) {
	return value, nil
}

// extractionFromMap converts a parsed JSON object (map[string]any) into an
// Extraction. It accepts top-level nil values and maps them to
// FieldExtraction{Value: nil} so that fields emitted as {"field": null} are
// not silently dropped. This helper is shared by ParseExtraction and
// splitRecords.
func extractionFromMap(m map[string]any) Extraction {
	if m == nil {
		return nil
	}

	ext := make(Extraction, len(m))

	for k, v := range m {
		if v == nil {
			ext[k] = FieldExtraction{Value: nil}
			continue
		}

		fm, ok := v.(map[string]any)
		if !ok {
			continue
		}

		conf, _ := fm["confidence"].(float64)

		var ev *string
		if e, ok := fm["evidence"].(string); ok {
			ev = &e
		}

		ext[k] = FieldExtraction{Value: fm["value"], Confidence: conf, Evidence: ev}
	}

	return ext
}

// ParseExtraction parses raw LLM JSON output into an Extraction. It supports
// both a single record object and a top-level "records" array containing one
// or more record objects.
func ParseExtraction(raw []byte) (Extraction, error) {
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("extractor: parse: %w", err)
	}

	// Multi-record wrapper: {"records": [...]}.
	if rawRecords, ok := top["records"].([]any); ok {
		return Extraction{"records": FieldExtraction{Value: rawRecords}}, nil
	}

	ext := extractionFromMap(top)

	return ext, nil
}
