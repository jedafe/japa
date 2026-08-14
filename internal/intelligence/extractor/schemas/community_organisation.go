package schemas

// CommunityOrganisation is the entity schema for the
// community_organisation domain.
// Projection table: community_organisations (ALTERed, migration
// 000469) — intelligence ingestion creates/updates a community
// organisation row from diaspora-community directories and event
// listings. Under Approach B (ADR-076 Amendment 1) the promoter writes
// ONLY the projection row with guide_id=NULL; the editorial contents
// guide is generated later by the AI Content Studio via
// intel_record_id. The radar surfaces community-org updates as
// item_type='community_organisation'.
var CommunityOrganisation = EntitySchema{
	EntityType: EntityTypeCommunityOrganisation,
	Fields: []FieldSpec{
		{Name: "name", Type: "string", Required: true},
		{Name: "slug", Type: "string", Required: false},            // derived from name when absent
		{Name: "org_type", Type: "string", Required: false, EnumValues: []string{"diaspora_association", "cultural_society", "religious", "professional", "student_club", "sports", "charity", "other"}},
		{Name: "country", Type: "country_array", Required: true},   // destination ISO-3, 1-element
		{Name: "city", Type: "string", Required: false},            // nullable → country-level org
		{Name: "origin_community", Type: "string", Required: false}, // e.g. "Nigerian diaspora in Berlin"
		{Name: "address", Type: "string", Required: false},
		{Name: "latitude", Type: "numeric", Required: false},
		{Name: "longitude", Type: "numeric", Required: false},
		{Name: "website_url", Type: "string", Required: false},
		{Name: "meeting_frequency", Type: "string", Required: false}, // e.g. "monthly", "weekly"
		{Name: "contact_info", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
