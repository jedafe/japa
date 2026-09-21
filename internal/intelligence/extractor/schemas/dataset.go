package schemas

// Dataset is the entity schema for tabular data records produced by the
// deterministic tabular curator (CSV/TSV channels). One intel_record is
// written per data row: provider_config.column_map maps CSV columns onto
// these field names and unmapped columns are preserved verbatim under the
// `data` jsonb field. It is curate-only: no LLM prompt template exists for
// it, so intel:extract must never dispatch on this entity type — the worker
// guards that path via IsCurateOnly and routes review instead.
var Dataset = EntitySchema{
	EntityType: EntityTypeDataset,
	CurateOnly: true,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "description", Type: "string", Required: false},
		{Name: "category", Type: "string", Required: false},
		{Name: "value", Type: "string", Required: false},
		{Name: "unit", Type: "string", Required: false},
		{Name: "country", Type: "country_array", Required: false},
		{Name: "effective_date", Type: "date", Required: false},
		{Name: "data", Type: "jsonb_object", Required: false},
		{Name: "source_url", Type: "string", Required: false},
	},
}
