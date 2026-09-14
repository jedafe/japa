package university_courses

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// Service provides university course catalog/offering business logic.
type Service struct {
	store *Store
}

// NewService creates a new Service backed by the given store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// SearchOfferings returns published course offerings for a tenant, optionally
// filtered by university and/or search query.
func (s *Service) SearchOfferings(ctx context.Context, universityID pgtype.UUID, query string, limit, offset int32) ([]CourseOffering, error) {
	return s.store.SearchOfferings(ctx, universityID, query, limit, offset)
}

// GetOfferingByID returns a course offering by its ID.
func (s *Service) GetOfferingByID(ctx context.Context, id pgtype.UUID) (CourseOffering, error) {
	offering, err := s.store.GetOfferingByID(ctx, id)
	if err != nil {
		return CourseOffering{}, fmt.Errorf("fetching course offering: %w", err)
	}

	return offering, nil
}

// GetOfferingBySlug returns a course offering by its tenant-scoped slug.
func (s *Service) GetOfferingBySlug(ctx context.Context, slug string) (CourseOffering, error) {
	offering, err := s.store.GetOfferingBySlug(ctx, slug)
	if err != nil {
		return CourseOffering{}, fmt.Errorf("fetching course offering: %w", err)
	}

	return offering, nil
}
