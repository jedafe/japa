package schemas

// University is the entity schema for the university domain.
// Projection: a spaces row on the 'education' space type with
// level='university' plus custom_field_values on the seeded
// wiki_university_overview group (§39, consolidated 001060); the university_intel_links table ties the
// space to the intel record and carries the editorial_overrides set.
// Field names mirror the group's custom-field keys so the promoter maps
// extraction fields 1:1 onto custom field values.
var University = EntitySchema{
	EntityType: EntityTypeUniversity,
	Fields: []FieldSpec{
		{Name: "name", Type: "string", Required: true},
		{Name: "slug", Type: "string", Required: false}, // derived from name when absent
		{Name: "initials", Type: "string", Required: false},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "city", Type: "string", Required: false},
		{Name: "official_website_url", Type: "string", Required: false},
		{Name: "contact_email", Type: "string", Required: false},
		{Name: "accreditation_body", Type: "string", Required: false},
		{Name: "world_ranking", Type: "numeric", Required: false},
		{Name: "country_ranking", Type: "numeric", Required: false},
		{Name: "founded_year", Type: "numeric", Required: false},
		{Name: "type", Type: "string", Required: false, EnumValues: []string{"public", "private"}},
		{Name: "programs_offered", Type: "text_array", Required: false},
		{Name: "tuition_range", Type: "string", Required: false, EnumValues: []string{"free", "<5k", "5k-15k", "15k-30k", ">30k"}},
		{Name: "student_population", Type: "numeric", Required: false},
		{Name: "acceptance_rate", Type: "numeric", Required: false},
		{Name: "international_support", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "scholarship_available", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "application_url", Type: "string", Required: false},
		{Name: "description", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
