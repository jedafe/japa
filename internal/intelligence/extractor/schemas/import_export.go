package schemas

// ImportExport is the entity schema for the import and export guide
// content type. Projection: contents row (content_type='guide',
// guide_category='import_export') + custom_field_values. No standalone
// table — the promoter creates a guide contents row and populates the
// custom fields defined in migration 000678.
var ImportExport = EntitySchema{
	EntityType: EntityTypeImportExport,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "origin_country", Type: "country_array", Required: true},      // origin ISO-3, 1-element
		{Name: "destination_country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "product_category", Type: "string", Required: false, EnumValues: []string{"agriculture_food", "textiles_apparel", "electronics", "machinery", "cosmetics", "arts_crafts", "other"}},
		{Name: "hs_code", Type: "string", Required: false},
		{Name: "trade_direction", Type: "string", Required: false, EnumValues: []string{"import", "export", "both"}},
		{Name: "incoterms", Type: "string", Required: false, EnumValues: []string{"exw", "fca", "fob", "cfr", "cif", "dap", "ddp"}},
		{Name: "shipping_method", Type: "string", Required: false, EnumValues: []string{"sea", "air", "land", "courier"}},
		{Name: "estimated_timeline", Type: "string", Required: false},
		{Name: "customs_duty_rate_pct", Type: "numeric", Required: false},
		{Name: "regulatory_agency", Type: "string", Required: false},
		{Name: "required_licenses_permits", Type: "text_array", Required: false},
		{Name: "required_documents", Type: "text_array", Required: false},
		{Name: "official_portal_link", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
