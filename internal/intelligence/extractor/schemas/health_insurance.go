package schemas

// HealthInsurance is the entity schema for the health & insurance guide
// content type. Projection: contents row (content_type='guide',
// guide_category='health_insurance') + custom_field_values.
var HealthInsurance = EntitySchema{
	EntityType: EntityTypeHealthInsurance,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "insurance_type", Type: "string", Required: false},
		{Name: "monthly_premium_usd", Type: "numeric", Required: false},
		{Name: "coverage_details", Type: "string", Required: false},
		{Name: "dental_covered", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "vision_covered", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "mental_health_covered", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "pre_existing_conditions", Type: "string", Required: false},
		{Name: "registration_link", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
