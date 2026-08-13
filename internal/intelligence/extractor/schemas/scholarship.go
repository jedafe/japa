// Package schemas defines the entity schemas used by the extractor to
// validate LLM output. Each schema declares the expected fields for one
// entity_type, including the field's type, whether it is required, and
// (for enum-typed fields) the set of allowed values.
package schemas

// FieldSpec declares one field in an entity schema.
type FieldSpec struct {
	Name       string   // canonical field name
	Type       string   // "string"|"date"|"text_array"|"country_array"|"jsonb_object"|"numeric"
	Required   bool     // whether the field must be present with a non-null value
	EnumValues []string // for degree_level etc.; empty means no enum constraint
}

// EntitySchema declares the expected fields for one entity_type.
type EntitySchema struct {
	EntityType string
	Fields     []FieldSpec
}

// Scholarship is the v1 entity schema for the scholarship domain.
var Scholarship = EntitySchema{
	EntityType: "scholarship",
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "description_summary", Type: "string", Required: false},
		{Name: "country", Type: "country_array", Required: true}, // single ISO-3, stored as 1-element
		{Name: "university", Type: "string", Required: false},
		{Name: "degree_level", Type: "string", Required: false, EnumValues: []string{"bachelors", "masters", "phd", "postdoc", "diploma", "certificate"}},
		{Name: "fields_of_study", Type: "text_array", Required: false},
		{Name: "eligible_countries", Type: "country_array", Required: false},
		{Name: "funding", Type: "jsonb_object", Required: false},
		{Name: "deadline", Type: "date", Required: false},
		{Name: "application_url", Type: "string", Required: false},
		{Name: "required_documents", Type: "text_array", Required: false},
		{Name: "minimum_gpa", Type: "numeric", Required: false},
		{Name: "language_requirements", Type: "jsonb_object", Required: false},
	},
}
