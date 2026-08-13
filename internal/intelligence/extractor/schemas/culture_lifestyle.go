package schemas

// CultureLifestyle is the entity schema for the culture & lifestyle guide
// content type. Projection: contents row (content_type='guide',
// guide_category='culture_lifestyle') + custom_field_values.
var CultureLifestyle = EntitySchema{
	EntityType: EntityTypeCultureLifestyle,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "city", Type: "string", Required: false},
		{Name: "language_spoken", Type: "string", Required: false},
		{Name: "currency", Type: "string", Required: false},
		{Name: "avg_living_cost_usd", Type: "numeric", Required: false},
		{Name: "cultural_etiquette", Type: "string", Required: false},
		{Name: "religious_considerations", Type: "string", Required: false},
		{Name: "food_dining", Type: "string", Required: false},
		{Name: "transportation", Type: "string", Required: false},
		{Name: "social_norms", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
