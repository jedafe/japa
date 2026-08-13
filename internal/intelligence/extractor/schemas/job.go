package schemas

// Job is the entity schema for the jobs domain. Projection table:
// intel_jobs (new, migration 000440). Fields mirror the job listing
// shape surfaced by the §63 jobs board consumer, plus the standard
// intelligence provenance columns (confidence, freshness, editorial
// overrides) carried by the projection table.
var Job = EntitySchema{
	EntityType: EntityTypeJob,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "description_summary", Type: "string", Required: false},
		{Name: "country", Type: "country_array", Required: true}, // destination ISO-3, 1-element
		{Name: "city", Type: "string", Required: false},
		{Name: "employer", Type: "string", Required: false},
		{Name: "job_type", Type: "string", Required: false, EnumValues: []string{"full_time", "part_time", "contract", "internship", "temporary", "remote"}},
		{Name: "experience_level", Type: "string", Required: false, EnumValues: []string{"entry", "mid", "senior", "lead", "executive"}},
		{Name: "salary_min", Type: "numeric", Required: false},
		{Name: "salary_max", Type: "numeric", Required: false},
		{Name: "currency", Type: "string", Required: false}, // ISO 4217, e.g. USD
		{Name: "skills", Type: "text_array", Required: false},
		{Name: "application_url", Type: "string", Required: false},
		{Name: "deadline", Type: "date", Required: false},
		{Name: "remote", Type: "string", Required: false, EnumValues: []string{"true", "false", "hybrid"}}, // bool-as-enum for LLM stability
	},
}
