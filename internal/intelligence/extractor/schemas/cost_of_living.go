package schemas

// CostOfLiving is the entity schema for the cost-of-living domain.
// Projection table: comparison_items (ALTERed, migration 000444) —
// intelligence ingestion creates/updates a comparison_item for a city
// under the cost_of_living comparison type, storing the extracted cost
// breakdown in the intel_cost_data jsonb column added by the migration.
// The radar surfaces cost-of-living updates as
// item_type='cost_of_living'.
var CostOfLiving = EntitySchema{
	EntityType: EntityTypeCostOfLiving,
	Fields: []FieldSpec{
		{Name: "city", Type: "string", Required: true},            // city name
		{Name: "country", Type: "country_array", Required: true},  // ISO-3, 1-element
		{Name: "cost_data", Type: "jsonb_object", Required: true}, // {rent_1br_city_centre, groceries_monthly, ...}
		{Name: "source_url", Type: "string", Required: true},
		{Name: "as_of_date", Type: "date", Required: true},
	},
}
