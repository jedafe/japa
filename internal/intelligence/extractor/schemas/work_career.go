package schemas

// WorkCareer is the entity schema for the work & career guide content
// type. Projection: contents row (content_type='guide',
// guide_category='work_career') + custom_field_values.
var WorkCareer = EntitySchema{
	EntityType: EntityTypeWorkCareer,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "work_visa_required", Type: "string", Required: true, EnumValues: []string{"true", "false"}},
		{Name: "visa_type", Type: "string", Required: false},
		{Name: "average_salary_usd", Type: "numeric", Required: false},
		{Name: "in_demand_sectors", Type: "string", Required: false},
		{Name: "work_hours_limit", Type: "string", Required: false},
		{Name: "tax_rate", Type: "string", Required: false},
		{Name: "networking_tips", Type: "string", Required: false},
		{Name: "cv_format", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
