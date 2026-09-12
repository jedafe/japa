package university_courses

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated queries for university course catalog and
// offerings.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates a new university_courses store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}

func toCourseOffering(row any) (CourseOffering, error) {
	switch r := row.(type) {
	case gen.ListCourseOfferingsByUniversityRow:
		return CourseOffering{
			ID: r.ID, CourseID: r.CourseID, UniversityID: r.UniversityID, Slug: r.Slug,
			Tuition: r.Tuition, Currency: r.Currency, DurationYears: r.DurationYears,
			StudyMode: r.StudyMode, IntakeMonths: r.IntakeMonths, EntryRequirements: r.EntryRequirements,
			Accreditation: r.Accreditation, IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			CourseTitle: r.CourseTitle, CourseSlug: r.CourseSlug, CourseDescription: r.CourseDescription,
			DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
			Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
			UniversityName: r.UniversityName, UniversitySlug: r.UniversitySlug,
		}, nil
	case gen.GetCourseOfferingByIDRow:
		return CourseOffering{
			ID: r.ID, CourseID: r.CourseID, UniversityID: r.UniversityID, Slug: r.Slug,
			Tuition: r.Tuition, Currency: r.Currency, DurationYears: r.DurationYears,
			StudyMode: r.StudyMode, IntakeMonths: r.IntakeMonths, EntryRequirements: r.EntryRequirements,
			Accreditation: r.Accreditation, IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			CourseTitle: r.CourseTitle, CourseSlug: r.CourseSlug, CourseDescription: r.CourseDescription,
			DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
			Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
			UniversityName: r.UniversityName, UniversitySlug: r.UniversitySlug,
		}, nil
	case gen.GetCourseOfferingBySlugRow:
		return CourseOffering{
			ID: r.ID, CourseID: r.CourseID, UniversityID: r.UniversityID, Slug: r.Slug,
			Tuition: r.Tuition, Currency: r.Currency, DurationYears: r.DurationYears,
			StudyMode: r.StudyMode, IntakeMonths: r.IntakeMonths, EntryRequirements: r.EntryRequirements,
			Accreditation: r.Accreditation, IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			CourseTitle: r.CourseTitle, CourseSlug: r.CourseSlug, CourseDescription: r.CourseDescription,
			DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
			Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
			UniversityName: r.UniversityName, UniversitySlug: r.UniversitySlug,
		}, nil
	default:
		return CourseOffering{}, fmt.Errorf("unknown course offering row type %T", row)
	}
}

// SearchOfferings returns published course offerings for a tenant, optionally
// filtered by university and/or search query.
func (s *Store) SearchOfferings(ctx context.Context, tenantID, universityID pgtype.UUID, query string, limit, offset int32) ([]CourseOffering, error) {
	rows, err := s.read.ListCourseOfferingsByUniversity(ctx, gen.ListCourseOfferingsByUniversityParams{

		UniversityID: universityID,
		Search:       query,
		LimitVal:     limit,
		OffsetVal:    offset,
	})
	if err != nil {
		return nil, fmt.Errorf("university_courses: search offerings: %w", err)
	}

	out := make([]CourseOffering, 0, len(rows))

	for _, r := range rows {
		o, err := toCourseOffering(r)
		if err != nil {
			return nil, err
		}

		out = append(out, o)
	}

	return out, nil
}

// GetOfferingByID returns a course offering by its ID.
func (s *Store) GetOfferingByID(ctx context.Context, tenantID, id pgtype.UUID) (CourseOffering, error) {
	row, err := s.read.GetCourseOfferingByID(ctx,
		id,
	)
	if err != nil {
		return CourseOffering{}, fmt.Errorf("university_courses: get offering by id: %w", err)
	}

	return toCourseOffering(row)
}

// GetOfferingBySlug returns a course offering by its tenant-scoped slug.
func (s *Store) GetOfferingBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (CourseOffering, error) {
	row, err := s.read.GetCourseOfferingBySlug(ctx,
		slug,
	)
	if err != nil {
		return CourseOffering{}, fmt.Errorf("university_courses: get offering by slug: %w", err)
	}

	return toCourseOffering(row)
}
