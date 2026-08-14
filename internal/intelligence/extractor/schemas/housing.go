package schemas

// Housing is the entity schema for the housing/accommodation guides
// domain. Projection table: housing_guides (ALTERed, migration 000445)
// — intelligence ingestion updates existing guide rows or creates new
// ones from official/university housing sources. The radar surfaces
// housing guide updates as item_type='housing'.
var Housing = EntitySchema{
	EntityType: EntityTypeHousing,
	Fields: []FieldSpec{
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "city", Type: "string", Required: false},          // nullable → country-level guide
		{Name: "accommodation_type", Type: "string", Required: false, EnumValues: []string{"private_rental", "flatshare", "student", "employer_provided", "all"}},
		{Name: "search_platforms", Type: "jsonb_object", Required: false}, // [{name, url, description}]
		{Name: "landlord_requirements", Type: "string", Required: false},
		{Name: "rent_ranges", Type: "jsonb_object", Required: false}, // {studio, 1bed, 2bed} × {city_centre, inner, outer}
		{Name: "scam_categories", Type: "text_array", Required: false},
		{Name: "tips", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
