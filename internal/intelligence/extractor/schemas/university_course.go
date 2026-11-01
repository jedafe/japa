package schemas

// UniversityCourse is the entity schema for the university course-offering
// domain. Projection: a university_course_offerings row (plus a
// resolve-or-create university_course_catalog row). One intel record = one
// offering at one university; the (course_id, university_id) unique index
// is the projection's dedupe key.
var UniversityCourse = EntitySchema{
	EntityType: EntityTypeUniversityCourse,
	Fields: []FieldSpec{
		{Name: "course_title", Type: "string", Required: true},
		{Name: "university_name", Type: "string", Required: true},
		{Name: "university_slug", Type: "string", Required: false}, // fast-path resolution
		{Name: "country", Type: "country_array", Required: false},  // disambiguates university_name
		{Name: "degree_type", Type: "string", Required: false, EnumValues: []string{"bachelors", "masters", "phd", "postdoc", "diploma", "certificate", "associate"}},
		{Name: "field_of_study", Type: "string", Required: false},
		{Name: "department", Type: "string", Required: false},
		{Name: "language", Type: "string", Required: false},
		{Name: "duration_years", Type: "numeric", Required: false},
		{Name: "tuition", Type: "numeric", Required: false},
		{Name: "currency", Type: "string", Required: false},
		{Name: "study_mode", Type: "string", Required: false, EnumValues: []string{"full_time", "part_time", "online", "hybrid"}},
		{Name: "intake_months", Type: "text_array", Required: false},
		{Name: "entry_requirements", Type: "string", Required: false},
		{Name: "accreditation", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
