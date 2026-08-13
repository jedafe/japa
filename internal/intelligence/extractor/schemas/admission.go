package schemas

// Admission is the entity schema for the admission process guide content
// type. Projection: contents row (content_type='guide',
// guide_category='admission') + custom_field_values.
var Admission = EntitySchema{
	EntityType: EntityTypeAdmission,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "institution_name", Type: "string", Required: true},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "degree_level", Type: "string", Required: false, EnumValues: []string{"bachelors", "masters", "phd", "postdoc", "diploma", "certificate"}},
		{Name: "application_deadline", Type: "date", Required: false},
		{Name: "application_fee_usd", Type: "numeric", Required: false},
		{Name: "required_exams", Type: "string", Required: false},
		{Name: "minimum_gpa", Type: "string", Required: false},
		{Name: "language_requirements", Type: "string", Required: false},
		{Name: "application_link", Type: "string", Required: false},
		{Name: "acceptance_rate", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
