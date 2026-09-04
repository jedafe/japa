package schemas

// WikiPage is the entity schema for the wiki_page domain.
// Projection table: wiki_pages (ALTERed, migration 000469) — intelligence
// ingestion creates/updates a wiki page from external documentation
// sources (embassy sites, government portals, community-maintained
// guides). Under Approach B (ADR-076 Amendment 1) the promoter writes
// ONLY the projection row with guide_id=NULL; the editorial contents
// guide is generated later by the AI Content Studio via intel_record_id.
// The radar surfaces wiki updates as item_type='wiki_page'.
var WikiPage = EntitySchema{
	EntityType: EntityTypeWikiPage,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},        // page title; falls back to "name"
		{Name: "name", Type: "string", Required: false},        // alias for title
		{Name: "slug", Type: "string", Required: false},        // derived from title when absent
		{Name: "category", Type: "string", Required: false},    // defaults to "general"
		{Name: "space_id", Type: "string", Required: false},    // optional UUID for space scoping
		{Name: "is_active", Type: "string", Required: false},   // boolean-ish; promoter uses boolField
		{Name: "is_featured", Type: "string", Required: false}, // boolean-ish; promoter uses boolField
		{Name: "source_url", Type: "string", Required: true},   // provenance for the wiki page
	},
}
