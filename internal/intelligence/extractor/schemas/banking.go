package schemas

// Banking is the entity schema for the banking & finance guide content
// type. Projection: contents row (content_type='guide',
// guide_category='banking') + custom_field_values.
var Banking = EntitySchema{
	EntityType: EntityTypeBanking,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "bank_name", Type: "string", Required: true},
		{Name: "account_type", Type: "string", Required: false},
		{Name: "minimum_balance_usd", Type: "numeric", Required: false},
		{Name: "monthly_fee_usd", Type: "numeric", Required: false},
		{Name: "international_transfer_fee", Type: "string", Required: false},
		{Name: "online_banking", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "debit_card_included", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "requirements", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
