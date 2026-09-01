package university_courses

import (
	"github.com/jackc/pgx/v5/pgtype"
)

// CourseCatalog is a canonical university course definition, independent of
// the offering university.
type CourseCatalog struct {
	ID                   pgtype.UUID
	TenantID             pgtype.UUID
	Title                string
	Slug                 string
	Description          *string
	DegreeType           *string
	Department           *string
	FieldOfStudy         *string
	Language             string
	DefaultDurationYears pgtype.Numeric
	IsPublished          bool
	DeletedAt            pgtype.Timestamptz
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
}

// CourseOffering links a CourseCatalog entry to a specific university with
// per-university details such as tuition and duration.
type CourseOffering struct {
	ID                  pgtype.UUID
	TenantID            pgtype.UUID
	CourseID            pgtype.UUID
	UniversityID        pgtype.UUID
	Slug                string
	Tuition             pgtype.Numeric
	Currency            *string
	DurationYears       pgtype.Numeric
	StudyMode           *string
	IntakeMonths        []string
	EntryRequirements   *string
	Accreditation       *string
	IsPublished         bool
	DeletedAt           pgtype.Timestamptz
	CreatedAt           pgtype.Timestamptz
	UpdatedAt           pgtype.Timestamptz

	// Joined fields from the catalog and the university space.
	CourseTitle          string
	CourseSlug           string
	CourseDescription    *string
	DegreeType           *string
	Department           *string
	FieldOfStudy         *string
	Language             string
	DefaultDurationYears pgtype.Numeric
	UniversityName       string
	UniversitySlug       string
}
