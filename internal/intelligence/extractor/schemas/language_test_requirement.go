package schemas

// LanguageTestRequirement is the entity schema for the
// language_test_requirement domain.
// Projection table: language_test_requirements (ALTERed, migration
// 000469) — intelligence ingestion creates/updates a language test
// requirement row from official immigration/university sources. Under
// Approach B (ADR-076 Amendment 1) the promoter writes ONLY the
// projection row with guide_id=NULL; the editorial contents guide is
// generated later by the AI Content Studio via intel_record_id.
// The radar surfaces language-test updates as
// item_type='language_test_requirement'.
var LanguageTestRequirement = EntitySchema{
	EntityType: EntityTypeLanguageTest,
	Fields: []FieldSpec{
		{Name: "country", Type: "country_array", Required: true},         // destination ISO-3, 1-element
		{Name: "test_id", Type: "string", Required: false},               // UUID of a language_tests row; falls back to test_name lookup
		{Name: "test_name", Type: "string", Required: false},             // human-readable test name; resolved to test_id by the promoter
		{Name: "visa_category", Type: "string", Required: false},         // e.g. "skilled_worker", "student"
		{Name: "minimum_score", Type: "string", Required: false},         // e.g. "6.5" (IELTS), "100" (TOEFL)
		{Name: "score_breakdown", Type: "jsonb_object", Required: false}, // {reading, writing, listening, speaking}
		{Name: "notes", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
