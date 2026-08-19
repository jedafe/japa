package schemas

// CredentialPathway is the entity schema for the professional credential
// recognition checker domain. Projection table: credential_pathways
// (migration 000521 adds the intelligence provenance columns).
//
// The extraction produces a profession × issuing country × destination country
// pathway. The promoter resolves profession_slug/name to credential_professions.id
// and ISO-3 country codes to spaces.id (level='country').
var CredentialPathway = EntitySchema{
	EntityType: EntityTypeCredentialPathway,
	Fields: []FieldSpec{
		{Name: "profession_slug", Type: "string", Required: false}, // e.g. "doctor", "nurse", "engineer"
		{Name: "profession_name", Type: "string", Required: false}, // human-readable fallback
		{Name: "profession_category", Type: "string", Required: false, EnumValues: []string{"medical", "engineering", "legal", "finance", "education", "other"}},
		{Name: "issuing_country", Type: "country_array", Required: true},     // ISO-3, 1-element
		{Name: "destination_country", Type: "country_array", Required: true}, // ISO-3, 1-element
		{Name: "regulatory_body", Type: "string", Required: true},
		{Name: "regulatory_body_url", Type: "string", Required: false},
		{Name: "pathway_type", Type: "string", Required: true, EnumValues: []string{"direct", "bridging_exam", "supervised_practice", "full_requalification"}},
		{Name: "timeline_estimate", Type: "string", Required: false}, // e.g. "6–12 months"
		{Name: "cost_estimate_usd", Type: "string", Required: false}, // e.g. "$500–$2,000"
		{Name: "pathway_description", Type: "string", Required: true},
		{Name: "official_application_url", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
