package schemas

// Grants is the entity schema for the grants application guide content
// type. Projection: contents row (content_type='guide',
// guide_category='grants') + custom_field_values.
var Grants = EntitySchema{
	EntityType: EntityTypeGrants,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "grant_name", Type: "string", Required: true},
		{Name: "granting_organization", Type: "string", Required: true},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "grant_amount_usd", Type: "numeric", Required: false},
		{Name: "application_deadline", Type: "date", Required: false},
		{Name: "application_link", Type: "string", Required: false},
		{Name: "eligibility", Type: "string", Required: true},
		{Name: "grant_type", Type: "string", Required: false},
		{Name: "reporting_required", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "match_funding_required", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "source_url", Type: "string", Required: true},
	},
}
