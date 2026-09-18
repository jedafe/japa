package schemas

// Internship is the entity schema for the internship guide content
// type. Projection: contents row (content_type='guide',
// guide_category='internship') + custom_field_values. No standalone
// table — the promoter creates a guide contents row and populates the
// custom fields defined in migration 000678.
var Internship = EntitySchema{
	EntityType: EntityTypeInternship,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "destination_country", Type: "country_array", Required: true}, // internship location ISO-3, 1-element
		{Name: "host_organization", Type: "string", Required: false},
		{Name: "internship_type", Type: "string", Required: false, EnumValues: []string{"summer", "industrial_attachment", "graduate", "research", "remote", "coop"}},
		{Name: "industry", Type: "string", Required: false, EnumValues: []string{"technology", "finance", "engineering", "healthcare", "media", "law", "ngo", "other"}},
		{Name: "duration", Type: "string", Required: false},
		{Name: "is_paid", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "monthly_stipend_usd", Type: "numeric", Required: false},
		{Name: "visa_sponsorship", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "eligibility_criteria", Type: "string", Required: false},
		{Name: "application_deadline", Type: "date", Required: false},
		{Name: "application_link", Type: "string", Required: false},
		{Name: "required_documents", Type: "text_array", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
