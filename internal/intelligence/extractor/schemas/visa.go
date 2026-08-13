package schemas

// Visa is the entity schema for the visa domain — a UNIFIED domain that
// writes to both the visa_routes standalone table (structured projection)
// AND creates a contents guide row (guide_category='visa') so both the
// /visa-routes API and the guide content surface are fed.
//
// The schema fields cover the union of visa_routes columns and the guide
// visa custom fields (migration 000225). The promoter maps each field to
// the appropriate storage.
var Visa = EntitySchema{
	EntityType: EntityTypeVisa,
	Fields: []FieldSpec{
		{Name: "name", Type: "string", Required: true},
		{Name: "visa_category", Type: "string", Required: true},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "eligibility_summary", Type: "string", Required: false},
		{Name: "requirements", Type: "jsonb_object", Required: false},
		{Name: "typical_fee_usd", Type: "numeric", Required: false},
		{Name: "processing_time", Type: "string", Required: false},
		{Name: "application_link", Type: "string", Required: false},
		{Name: "required_documents", Type: "text_array", Required: false},
		{Name: "biometrics_required", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "interview_required", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "medical_exam_required", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "validity_period", Type: "string", Required: false},
		{Name: "path_to_pr", Type: "string", Required: false},
		{Name: "renewal_terms", Type: "string", Required: false},
		{Name: "restrictions", Type: "string", Required: false},
		{Name: "gov_source_url", Type: "string", Required: true},
	},
}
