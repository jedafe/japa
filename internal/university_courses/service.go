package university_courses

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Service provides university course catalog/offering business logic.
type Service struct {
	store *Store
}

// NewService creates a new Service backed by the given store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// mapStoreError translates low-level store errors into the shared sentinels
// (pgx.ErrNoRows → ErrNotFound, 23505 → ErrDuplicate) so handlers can run
// every error through errpkg.HandleServiceError.
func mapStoreError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %v", errpkg.ErrNotFound, err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %v", errpkg.ErrDuplicate, err)
	}

	return err
}

// SearchOfferings returns published course offerings for a platform, optionally
// filtered by university, search query, and/or degree type.
func (s *Service) SearchOfferings(ctx context.Context, universityID pgtype.UUID, query, degreeType string, limit, offset int32) ([]CourseOffering, error) {
	return s.store.SearchOfferings(ctx, universityID, query, degreeType, limit, offset)
}

// CountOfferingsByUniversity returns the total published offerings matching
// the SearchOfferings filters — used for pagination meta.
func (s *Service) CountOfferingsByUniversity(ctx context.Context, universityID pgtype.UUID, query, degreeType string) (int64, error) {
	return s.store.CountOfferingsByUniversity(ctx, universityID, query, degreeType)
}

// GetOfferingByID returns a course offering by its ID.
func (s *Service) GetOfferingByID(ctx context.Context, id pgtype.UUID) (CourseOffering, error) {
	offering, err := s.store.GetOfferingByID(ctx, id)
	if err != nil {
		return CourseOffering{}, mapStoreError(fmt.Errorf("fetching course offering: %w", err))
	}

	return offering, nil
}

// GetOfferingBySlug returns a course offering by its platform-scoped slug.
func (s *Service) GetOfferingBySlug(ctx context.Context, slug string) (CourseOffering, error) {
	offering, err := s.store.GetOfferingBySlug(ctx, slug)
	if err != nil {
		return CourseOffering{}, mapStoreError(fmt.Errorf("fetching course offering: %w", err))
	}

	return offering, nil
}

// GetPublishedOfferingBySlugs returns a published offering scoped to a
// university slug — the canonical nested-route lookup for
// /universities/{slug}/courses/{offeringSlug}.
func (s *Service) GetPublishedOfferingBySlugs(ctx context.Context, universitySlug, offeringSlug string) (CourseOffering, error) {
	offering, err := s.store.GetPublishedOfferingBySlugs(ctx, universitySlug, offeringSlug)
	if err != nil {
		return CourseOffering{}, mapStoreError(fmt.Errorf("fetching course offering: %w", err))
	}

	return offering, nil
}

// ListCatalogAdmin returns catalog rows for the admin DataTable.
func (s *Service) ListCatalogAdmin(ctx context.Context, f CatalogListFilters) ([]CourseCatalog, error) {
	return s.store.ListCatalogAdmin(ctx, f)
}

// CountCatalogAdmin returns the total catalog rows matching the admin filters.
func (s *Service) CountCatalogAdmin(ctx context.Context, f CatalogListFilters) (int64, error) {
	return s.store.CountCatalogAdmin(ctx, f)
}

// GetCatalogByID returns a catalog row (any publish state, not deleted).
func (s *Service) GetCatalogByID(ctx context.Context, id pgtype.UUID) (CourseCatalog, error) {
	c, err := s.store.GetCatalogByID(ctx, id)
	if err != nil {
		return CourseCatalog{}, mapStoreError(err)
	}

	return c, nil
}

// CreateCatalog inserts a catalog row.
func (s *Service) CreateCatalog(ctx context.Context, p gen.CreateCourseCatalogParams) (CourseCatalog, error) {
	c, err := s.store.CreateCatalog(ctx, p)
	if err != nil {
		return CourseCatalog{}, mapStoreError(err)
	}

	return c, nil
}

// UpdateCatalog updates a catalog row (full-row update).
func (s *Service) UpdateCatalog(ctx context.Context, p gen.UpdateCourseCatalogParams) (CourseCatalog, error) {
	c, err := s.store.UpdateCatalog(ctx, p)
	if err != nil {
		return CourseCatalog{}, mapStoreError(err)
	}

	return c, nil
}

// SoftDeleteCatalog marks a catalog row deleted.
func (s *Service) SoftDeleteCatalog(ctx context.Context, id pgtype.UUID) error {
	return mapStoreError(s.store.SoftDeleteCatalog(ctx, id))
}

// HardDeleteCatalog permanently removes a soft-deleted catalog row.
// Offerings must be force-deleted first (ErrInUse when any remain).
func (s *Service) HardDeleteCatalog(ctx context.Context, id pgtype.UUID) error {
	return mapStoreError(s.store.HardDeleteCatalog(ctx, id))
}

// ListOfferingsAdmin returns offering rows for the admin DataTable.
func (s *Service) ListOfferingsAdmin(ctx context.Context, f OfferingListFilters) ([]CourseOffering, error) {
	return s.store.ListOfferingsAdmin(ctx, f)
}

// CountOfferingsAdmin returns the total offerings matching the admin filters.
func (s *Service) CountOfferingsAdmin(ctx context.Context, f OfferingListFilters) (int64, error) {
	return s.store.CountOfferingsAdmin(ctx, f)
}

// CreateOffering inserts an offering row and returns its ID.
func (s *Service) CreateOffering(ctx context.Context, p gen.CreateCourseOfferingParams) (pgtype.UUID, error) {
	id, err := s.store.CreateOffering(ctx, p)
	if err != nil {
		return pgtype.UUID{}, mapStoreError(err)
	}

	return id, nil
}

// UpdateOffering updates an offering row and returns its ID.
func (s *Service) UpdateOffering(ctx context.Context, p gen.UpdateCourseOfferingParams) (pgtype.UUID, error) {
	id, err := s.store.UpdateOffering(ctx, p)
	if err != nil {
		return pgtype.UUID{}, mapStoreError(err)
	}

	return id, nil
}

// SoftDeleteOffering marks an offering row deleted.
func (s *Service) SoftDeleteOffering(ctx context.Context, id pgtype.UUID) error {
	return mapStoreError(s.store.SoftDeleteOffering(ctx, id))
}

// HardDeleteOffering permanently removes a soft-deleted offering.
func (s *Service) HardDeleteOffering(ctx context.Context, id pgtype.UUID) error {
	return mapStoreError(s.store.HardDeleteOffering(ctx, id))
}

// BulkSetOfferingsPublished sets is_published on a set of offering ids and
// returns the number of rows actually updated.
func (s *Service) BulkSetOfferingsPublished(ctx context.Context, ids []pgtype.UUID, published bool) (int64, error) {
	affected, err := s.store.BulkSetOfferingsPublished(ctx, ids, published)
	if err != nil {
		return 0, mapStoreError(err)
	}

	return affected, nil
}

// BulkSoftDeleteOfferings marks a set of offering ids deleted and returns
// the number of rows actually updated.
func (s *Service) BulkSoftDeleteOfferings(ctx context.Context, ids []pgtype.UUID) (int64, error) {
	affected, err := s.store.BulkSoftDeleteOfferings(ctx, ids)
	if err != nil {
		return 0, mapStoreError(err)
	}

	return affected, nil
}
