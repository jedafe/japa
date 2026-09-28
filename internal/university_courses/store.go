package university_courses

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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
	case gen.GetPublishedCourseOfferingBySlugsRow:
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
	case gen.ListCourseOfferingsAdminRow:
		return CourseOffering{
			ID: r.ID, CourseID: r.CourseID, UniversityID: r.UniversityID, Slug: r.Slug,
			Tuition: r.Tuition, Currency: r.Currency, DurationYears: r.DurationYears,
			StudyMode: r.StudyMode, IntakeMonths: r.IntakeMonths, EntryRequirements: r.EntryRequirements,
			Accreditation: r.Accreditation, IsPublished: r.IsPublished, DeletedAt: r.DeletedAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			CourseTitle: r.CourseTitle, CourseSlug: r.CourseSlug,
			DegreeType:     r.DegreeType,
			UniversityName: r.UniversityName, UniversitySlug: r.UniversitySlug,
		}, nil
	default:
		return CourseOffering{}, fmt.Errorf("unknown course offering row type %T", row)
	}
}

// SearchOfferings returns published course offerings for a platform, optionally
// filtered by university, search query, and/or degree type.
func (s *Store) SearchOfferings(ctx context.Context, universityID pgtype.UUID, query, degreeType string, limit, offset int32) ([]CourseOffering, error) {
	rows, err := s.read.ListCourseOfferingsByUniversity(ctx, gen.ListCourseOfferingsByUniversityParams{

		UniversityID: universityID,
		Search:       query,
		DegreeType:   degreeType,
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
func (s *Store) GetOfferingByID(ctx context.Context, id pgtype.UUID) (CourseOffering, error) {
	row, err := s.read.GetCourseOfferingByID(ctx,
		id,
	)
	if err != nil {
		return CourseOffering{}, fmt.Errorf("university_courses: get offering by id: %w", err)
	}

	return toCourseOffering(row)
}

// GetOfferingBySlug returns a course offering by its platform-scoped slug.
func (s *Store) GetOfferingBySlug(ctx context.Context, slug string) (CourseOffering, error) {
	row, err := s.read.GetCourseOfferingBySlug(ctx,
		slug,
	)
	if err != nil {
		return CourseOffering{}, fmt.Errorf("university_courses: get offering by slug: %w", err)
	}

	return toCourseOffering(row)
}

// CountOfferingsByUniversity returns the total published offerings matching
// the same filters as SearchOfferings — used for pagination meta.
func (s *Store) CountOfferingsByUniversity(ctx context.Context, universityID pgtype.UUID, query, degreeType string) (int64, error) {
	total, err := s.read.CountCourseOfferingsByUniversity(ctx, gen.CountCourseOfferingsByUniversityParams{
		UniversityID: universityID,
		Search:       query,
		DegreeType:   degreeType,
	})
	if err != nil {
		return 0, fmt.Errorf("university_courses: count offerings: %w", err)
	}

	return total, nil
}

// GetPublishedOfferingBySlugs returns a published offering scoped to a
// university slug — the canonical nested-route lookup.
func (s *Store) GetPublishedOfferingBySlugs(ctx context.Context, universitySlug, offeringSlug string) (CourseOffering, error) {
	row, err := s.read.GetPublishedCourseOfferingBySlugs(ctx, gen.GetPublishedCourseOfferingBySlugsParams{
		UniversitySlug: universitySlug,
		OfferingSlug:   offeringSlug,
	})
	if err != nil {
		return CourseOffering{}, fmt.Errorf("university_courses: get published offering: %w", err)
	}

	return toCourseOffering(row)
}

func toCourseCatalog(row any) (CourseCatalog, error) {
	switch r := row.(type) {
	case gen.ListCourseCatalogAdminRow:
		return CourseCatalog{
			ID: r.ID, Title: r.Title, Slug: r.Slug, Description: r.Description,
			DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
			Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
			IsPublished: r.IsPublished, DeletedAt: r.DeletedAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			OfferingCount: r.OfferingCount,
		}, nil
	case gen.GetCourseCatalogByIDRow:
		return CourseCatalog{
			ID: r.ID, Title: r.Title, Slug: r.Slug, Description: r.Description,
			DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
			Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
			IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			OfferingCount: r.OfferingCount,
		}, nil
	case gen.CreateCourseCatalogRow:
		return CourseCatalog{
			ID: r.ID, Title: r.Title, Slug: r.Slug, Description: r.Description,
			DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
			Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
			IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, nil
	case gen.UpdateCourseCatalogRow:
		return CourseCatalog{
			ID: r.ID, Title: r.Title, Slug: r.Slug, Description: r.Description,
			DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
			Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
			IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, nil
	default:
		return CourseCatalog{}, fmt.Errorf("unknown course catalog row type %T", row)
	}
}

// CatalogListFilters are the admin list filters for the course catalog.
// Deleted is '' (live only), 'only' (soft-deleted), or 'all'.
type CatalogListFilters struct {
	Search      string
	DegreeType  string
	IsPublished *bool
	Deleted     string
	Sort        string
	Limit       int32
	Offset      int32
}

// ListCatalogAdmin returns catalog rows for the admin DataTable (all publish
// states; soft-deleted excluded unless f.Deleted is 'only' or 'all').
func (s *Store) ListCatalogAdmin(ctx context.Context, f CatalogListFilters) ([]CourseCatalog, error) {
	rows, err := s.read.ListCourseCatalogAdmin(ctx, gen.ListCourseCatalogAdminParams{
		Search:      f.Search,
		DegreeType:  f.DegreeType,
		IsPublished: f.IsPublished,
		Deleted:     f.Deleted,
		Sort:        f.Sort,
		LimitVal:    f.Limit,
		OffsetVal:   f.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("university_courses: list catalog admin: %w", err)
	}

	out := make([]CourseCatalog, 0, len(rows))
	for _, r := range rows {
		c, err := toCourseCatalog(r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}

	return out, nil
}

// CountCatalogAdmin returns the total catalog rows matching the admin filters.
func (s *Store) CountCatalogAdmin(ctx context.Context, f CatalogListFilters) (int64, error) {
	total, err := s.read.CountCourseCatalogAdmin(ctx, gen.CountCourseCatalogAdminParams{
		Search:      f.Search,
		DegreeType:  f.DegreeType,
		IsPublished: f.IsPublished,
		Deleted:     f.Deleted,
	})
	if err != nil {
		return 0, fmt.Errorf("university_courses: count catalog admin: %w", err)
	}

	return total, nil
}

// GetCatalogByID returns a catalog row (any publish state, not deleted).
func (s *Store) GetCatalogByID(ctx context.Context, id pgtype.UUID) (CourseCatalog, error) {
	row, err := s.read.GetCourseCatalogByID(ctx, id)
	if err != nil {
		return CourseCatalog{}, fmt.Errorf("university_courses: get catalog by id: %w", err)
	}

	return toCourseCatalog(row)
}

// CreateCatalog inserts a catalog row.
func (s *Store) CreateCatalog(ctx context.Context, p gen.CreateCourseCatalogParams) (CourseCatalog, error) {
	row, err := s.write.CreateCourseCatalog(ctx, p)
	if err != nil {
		return CourseCatalog{}, fmt.Errorf("university_courses: create catalog: %w", err)
	}

	return toCourseCatalog(row)
}

// UpdateCatalog updates a catalog row (full-row update; slug uniqueness
// violations surface as 23505 which the service maps to ErrDuplicate).
func (s *Store) UpdateCatalog(ctx context.Context, p gen.UpdateCourseCatalogParams) (CourseCatalog, error) {
	row, err := s.write.UpdateCourseCatalog(ctx, p)
	if err != nil {
		return CourseCatalog{}, fmt.Errorf("university_courses: update catalog: %w", err)
	}

	return toCourseCatalog(row)
}

// SoftDeleteCatalog marks a catalog row deleted. Its offerings remain for
// admin visibility but drop out of public joins (c.deleted_at IS NULL).
func (s *Store) SoftDeleteCatalog(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.SoftDeleteCourseCatalog(ctx, id); err != nil {
		return fmt.Errorf("university_courses: delete catalog: %w", err)
	}

	return nil
}

// HardDeleteCatalog permanently removes a soft-deleted catalog row. The
// query refuses while any offering still references it (live or deleted) —
// the FK cascade would otherwise silently remove live children. Returns
// pgx.ErrNoRows when missing, errpkg.ErrInUse when offerings remain, and
// errpkg.ErrNotDeleted when the row is still live.
func (s *Store) HardDeleteCatalog(ctx context.Context, id pgtype.UUID) error {
	affected, err := s.write.HardDeleteCourseCatalog(ctx, id)
	if err != nil {
		return fmt.Errorf("university_courses: hard delete catalog: %w", err)
	}

	if affected == 0 {
		exists, err := s.read.CourseCatalogExists(ctx, id)
		if err != nil {
			return fmt.Errorf("university_courses: check catalog exists: %w", err)
		}
		if !exists {
			return pgx.ErrNoRows
		}
		hasOfferings, err := s.read.CourseCatalogHasOfferings(ctx, id)
		if err != nil {
			return fmt.Errorf("university_courses: check catalog offerings: %w", err)
		}
		if hasOfferings {
			return errpkg.ErrInUse
		}
		return errpkg.ErrNotDeleted
	}

	return nil
}

// OfferingListFilters are the admin list filters for course offerings.
// Deleted is '' (live only), 'only' (soft-deleted), or 'all'.
type OfferingListFilters struct {
	Search       string
	UniversityID pgtype.UUID
	CourseID     pgtype.UUID
	IsPublished  *bool
	Deleted      string
	Sort         string
	Limit        int32
	Offset       int32
}

// ListOfferingsAdmin returns offering rows for the admin DataTable (all
// publish states; soft-deleted excluded unless f.Deleted is 'only' or
// 'all'), joined with catalog + university.
func (s *Store) ListOfferingsAdmin(ctx context.Context, f OfferingListFilters) ([]CourseOffering, error) {
	rows, err := s.read.ListCourseOfferingsAdmin(ctx, gen.ListCourseOfferingsAdminParams{
		Search:       f.Search,
		UniversityID: f.UniversityID,
		CourseID:     f.CourseID,
		IsPublished:  f.IsPublished,
		Deleted:      f.Deleted,
		Sort:         f.Sort,
		LimitVal:     f.Limit,
		OffsetVal:    f.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("university_courses: list offerings admin: %w", err)
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

// CountOfferingsAdmin returns the total offerings matching the admin filters.
func (s *Store) CountOfferingsAdmin(ctx context.Context, f OfferingListFilters) (int64, error) {
	total, err := s.read.CountCourseOfferingsAdmin(ctx, gen.CountCourseOfferingsAdminParams{
		Search:       f.Search,
		UniversityID: f.UniversityID,
		CourseID:     f.CourseID,
		IsPublished:  f.IsPublished,
		Deleted:      f.Deleted,
	})
	if err != nil {
		return 0, fmt.Errorf("university_courses: count offerings admin: %w", err)
	}

	return total, nil
}

// CreateOffering inserts an offering row.
func (s *Store) CreateOffering(ctx context.Context, p gen.CreateCourseOfferingParams) (pgtype.UUID, error) {
	row, err := s.write.CreateCourseOffering(ctx, p)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("university_courses: create offering: %w", err)
	}

	return row.ID, nil
}

// UpdateOffering updates an offering row (full-row update).
func (s *Store) UpdateOffering(ctx context.Context, p gen.UpdateCourseOfferingParams) (pgtype.UUID, error) {
	row, err := s.write.UpdateCourseOffering(ctx, p)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("university_courses: update offering: %w", err)
	}

	return row.ID, nil
}

// SoftDeleteOffering marks an offering row deleted.
func (s *Store) SoftDeleteOffering(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.SoftDeleteCourseOffering(ctx, id); err != nil {
		return fmt.Errorf("university_courses: delete offering: %w", err)
	}

	return nil
}

// HardDeleteOffering permanently removes a soft-deleted offering. Returns
// pgx.ErrNoRows when missing and errpkg.ErrNotDeleted when still live.
func (s *Store) HardDeleteOffering(ctx context.Context, id pgtype.UUID) error {
	affected, err := s.write.HardDeleteCourseOffering(ctx, id)
	if err != nil {
		return fmt.Errorf("university_courses: hard delete offering: %w", err)
	}

	if affected == 0 {
		exists, err := s.read.CourseOfferingExists(ctx, id)
		if err != nil {
			return fmt.Errorf("university_courses: check offering exists: %w", err)
		}
		if !exists {
			return pgx.ErrNoRows
		}
		return errpkg.ErrNotDeleted
	}

	return nil
}

// BulkSetOfferingsPublished sets is_published on a set of offering ids.
func (s *Store) BulkSetOfferingsPublished(ctx context.Context, ids []pgtype.UUID, published bool) error {
	if err := s.write.BulkSetCourseOfferingsPublished(ctx, gen.BulkSetCourseOfferingsPublishedParams{
		IsPublished: published,
		Ids:         ids,
	}); err != nil {
		return fmt.Errorf("university_courses: bulk set published: %w", err)
	}

	return nil
}

// BulkSoftDeleteOfferings marks a set of offering ids deleted.
func (s *Store) BulkSoftDeleteOfferings(ctx context.Context, ids []pgtype.UUID) error {
	if err := s.write.BulkSoftDeleteCourseOfferings(ctx, ids); err != nil {
		return fmt.Errorf("university_courses: bulk delete: %w", err)
	}

	return nil
}
