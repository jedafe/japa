package extractor

import (
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
)

func strPtr(s string) *string { return &s }

func TestValidate_HallucinationGuard(t *testing.T) {
	// LLM claims minimum_gpa=3.5 but provides NO evidence -> must be reset to null.
	ext := Extraction{
		"title":       {Value: "DAAD EPOS 2027", Confidence: 0.97, Evidence: strPtr("...excerpt...")},
		"country":     {Value: []any{"DEU"}, Confidence: 0.95, Evidence: strPtr("...excerpt...")},
		"minimum_gpa": {Value: 3.5, Confidence: 0.80, Evidence: nil}, // HALLUCINATION
	}
	cleaned, errs := Validate(ext, schemas.Scholarship)
	if cleaned["minimum_gpa"].Value != nil {
		t.Errorf("hallucination not caught: minimum_gpa = %v, want nil", cleaned["minimum_gpa"].Value)
	}
	if len(errs) == 0 {
		t.Error("expected validation error for hallucination, got none")
	}
}

func TestValidate_NullValueWithEvidence(t *testing.T) {
	ext := Extraction{
		"title":       {Value: "Test", Confidence: 0.9, Evidence: strPtr("excerpt")},
		"country":     {Value: []any{"USA"}, Confidence: 0.9, Evidence: strPtr("excerpt")},
		"minimum_gpa": {Value: nil, Confidence: 0.99, Evidence: strPtr("some evidence")}, // inconsistent
	}
	cleaned, errs := Validate(ext, schemas.Scholarship)
	if cleaned["minimum_gpa"].Evidence != nil {
		t.Error("expected evidence cleared for null value")
	}
	if len(errs) == 0 {
		t.Error("expected validation error for null-with-evidence")
	}
}

func TestValidate_RequiredFieldMissing(t *testing.T) {
	ext := Extraction{
		"country": {Value: []any{"USA"}, Confidence: 0.9, Evidence: strPtr("excerpt")},
	}
	_, errs := Validate(ext, schemas.Scholarship)
	found := false
	for _, e := range errs {
		if e.Field == "title" && strings.Contains(e.Message, "required") {
			found = true
		}
	}
	if !found {
		t.Error("expected required-field error for missing title")
	}
}

func TestValidate_ConfidenceOutOfRange(t *testing.T) {
	ext := Extraction{
		"title":   {Value: "Test", Confidence: 1.5, Evidence: strPtr("excerpt")},
		"country": {Value: []any{"USA"}, Confidence: 0.9, Evidence: strPtr("excerpt")},
	}
	cleaned, errs := Validate(ext, schemas.Scholarship)
	if cleaned["title"].Confidence != 0 {
		t.Errorf("expected confidence clamped to 0, got %f", cleaned["title"].Confidence)
	}
	found := false
	for _, e := range errs {
		if e.Field == "title" && strings.Contains(e.Message, "confidence") {
			found = true
		}
	}
	if !found {
		t.Error("expected confidence-out-of-range error for title")
	}
}

func TestValidate_ValidExtraction(t *testing.T) {
	ext := Extraction{
		"title":       {Value: "DAAD EPOS 2027", Confidence: 0.97, Evidence: strPtr("DAAD EPOS programme 2027")},
		"country":     {Value: []any{"DEU"}, Confidence: 0.95, Evidence: strPtr("Germany")},
		"minimum_gpa": {Value: 3.5, Confidence: 0.80, Evidence: strPtr("minimum GPA of 3.5")},
	}
	cleaned, errs := Validate(ext, schemas.Scholarship)
	if len(errs) != 0 {
		t.Errorf("expected no validation errors, got %d: %v", len(errs), errs)
	}
	if cleaned["title"].Value != "DAAD EPOS 2027" {
		t.Errorf("title changed by validation: %v", cleaned["title"].Value)
	}
	if cleaned["minimum_gpa"].Value == nil {
		t.Error("minimum_gpa should not be reset when evidence is present")
	}
}

func TestValidate_EnumInvalidValue(t *testing.T) {
	ext := Extraction{
		"title":        {Value: "Test", Confidence: 0.9, Evidence: strPtr("excerpt")},
		"country":      {Value: []any{"USA"}, Confidence: 0.9, Evidence: strPtr("excerpt")},
		"degree_level": {Value: "associate", Confidence: 0.9, Evidence: strPtr("associate degree")}, // not in enum
	}
	cleaned, errs := Validate(ext, schemas.Scholarship)
	if cleaned["degree_level"].Value != nil {
		t.Errorf("expected degree_level reset to nil for invalid enum, got %v", cleaned["degree_level"].Value)
	}
	found := false
	for _, e := range errs {
		if e.Field == "degree_level" {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for invalid enum value")
	}
}

func TestValidate_DateCoercion(t *testing.T) {
	ext := Extraction{
		"title":    {Value: "Test", Confidence: 0.9, Evidence: strPtr("excerpt")},
		"country":  {Value: []any{"USA"}, Confidence: 0.9, Evidence: strPtr("excerpt")},
		"deadline": {Value: "2027-03-15", Confidence: 0.9, Evidence: strPtr("deadline March 15 2027")},
	}
	cleaned, errs := Validate(ext, schemas.Scholarship)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid date, got %v", errs)
	}
	if cleaned["deadline"].Value != "2027-03-15" {
		t.Errorf("deadline not preserved: %v", cleaned["deadline"].Value)
	}
}

func TestParseExtraction(t *testing.T) {
	raw := []byte(`{"title":{"value":"X","confidence":0.9,"evidence":"e"},"country":{"value":["DEU"],"confidence":0.9,"evidence":"e"}}`)
	ext, err := ParseExtraction(raw)
	if err != nil {
		t.Fatalf("ParseExtraction failed: %v", err)
	}
	if ext["title"].Value != "X" {
		t.Errorf("title not parsed: %v", ext["title"].Value)
	}
}

func TestParseExtraction_InvalidJSON(t *testing.T) {
	_, err := ParseExtraction([]byte(`{not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseExtraction_NullField(t *testing.T) {
	raw := []byte(`{"title":{"value":"X","confidence":0.9,"evidence":"e"},"country":null}`)
	ext, err := ParseExtraction(raw)
	if err != nil {
		t.Fatalf("ParseExtraction failed: %v", err)
	}
	if _, ok := ext["country"]; !ok {
		t.Error("expected null country field to be preserved, got missing")
	}
	if ext["country"].Value != nil {
		t.Errorf("expected country value to be nil, got %v", ext["country"].Value)
	}
}
