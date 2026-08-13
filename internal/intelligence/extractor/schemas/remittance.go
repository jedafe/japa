package schemas

// Remittance is the entity schema for the remittance rates domain.
// Projection table: remittance_rates (ALTERed, migration 000443) —
// intelligence ingestion appends rate rows for a corridor from
// central-bank / provider sources. The promoter resolves the
// remittance_provider by name (extracted) and stamps provider_id. The
// radar surfaces remittance rate moves as item_type='remittance_update'.
var Remittance = EntitySchema{
	EntityType: EntityTypeRemittance,
	Fields: []FieldSpec{
		{Name: "provider_name", Type: "string", Required: true},
		{Name: "send_currency", Type: "string", Required: true},  // ISO 4217
		{Name: "receive_currency", Type: "string", Required: true}, // ISO 4217
		{Name: "rate", Type: "numeric", Required: true},
		{Name: "fee_flat", Type: "numeric", Required: false},
		{Name: "fee_pct", Type: "numeric", Required: false},
		{Name: "transfer_time", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
		{Name: "as_of_date", Type: "date", Required: true},
	},
}
