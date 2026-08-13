package schemas

// TravelRoutes is the entity schema for the travel routes guide content
// type. Projection: contents row (content_type='guide',
// guide_category='travel_routes') + custom_field_values. No standalone
// table — the promoter creates a guide contents row and populates the
// custom fields defined in migration 000225.
var TravelRoutes = EntitySchema{
	EntityType: EntityTypeTravelRoutes,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "origin_city", Type: "string", Required: true},
		{Name: "destination_city", Type: "string", Required: true},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "transit_hubs", Type: "string", Required: false},
		{Name: "estimated_cost_usd", Type: "numeric", Required: false},
		{Name: "travel_duration", Type: "string", Required: false},
		{Name: "recommended_airlines", Type: "string", Required: false},
		{Name: "best_travel_season", Type: "string", Required: false},
		{Name: "visa_on_arrival", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "layover_tips", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
