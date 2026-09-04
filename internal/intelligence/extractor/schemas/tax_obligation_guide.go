package schemas

// TaxObligationGuide is the entity schema for the tax_obligation_guide
// domain.
// Projection table: tax_obligation_guides (ALTERed, migration 000469)
// — intelligence ingestion creates/updates a tax obligation guide row
// from official tax-authority sources. Under Approach B (ADR-076
// Amendment 1) the promoter writes ONLY the projection row with
// guide_id=NULL; the editorial contents guide is generated later by the
// AI Content Studio via intel_record_id. The radar surfaces tax-guide
// updates as item_type='tax_obligation_guide'.
var TaxObligationGuide = EntitySchema{
	EntityType: EntityTypeTaxObligation,
	Fields: []FieldSpec{
		{Name: "origin_country", Type: "country_array", Required: true}, // origin ISO-3, 1-element
		{Name: "country", Type: "country_array", Required: true},        // destination ISO-3, 1-element
		{Name: "employment_status", Type: "string", Required: false, EnumValues: []string{"employed", "self_employed", "student", "unemployed", "retired", "other"}},
		{Name: "residency_trigger", Type: "string", Required: false},      // e.g. "183-day rule"
		{Name: "worldwide_income_taxed", Type: "string", Required: false}, // boolean-ish; promoter uses boolPtrField
		{Name: "first_year_obligations", Type: "string", Required: false},
		{Name: "action_checklist", Type: "jsonb_object", Required: false}, // [{step, description}]
		{Name: "source_url", Type: "string", Required: true},
	},
}
