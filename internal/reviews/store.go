package reviews

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for review operations. The Service
// layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Reviews CRUD
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createReview(ctx context.Context, p gen.CreateReviewParams) (gen.Review, error) {
	row, err := s.write.CreateReview(ctx, p)
	if err != nil {
		return gen.Review{}, fmt.Errorf("creating review: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getReviewByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.Review, error) {
	row, err := s.read.GetReviewByID(ctx, gen.GetReviewByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.Review{}, fmt.Errorf("getting review by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listReviewsByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, limit, offset int32) ([]gen.Review, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListReviewsByEntity(ctx, gen.ListReviewsByEntityParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing reviews by entity: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) listReviewsByReviewer(ctx context.Context, tenantID, reviewerUserID pgtype.UUID, limit, offset int32) ([]gen.Review, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListReviewsByReviewer(ctx, gen.ListReviewsByReviewerParams{
		TenantID:       tenantID,
		ReviewerUserID: reviewerUserID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing reviews by reviewer: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateReview(ctx context.Context, p gen.UpdateReviewParams) (gen.Review, error) {
	row, err := s.write.UpdateReview(ctx, p)
	if err != nil {
		return gen.Review{}, fmt.Errorf("updating review: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateReviewStatus(ctx context.Context, p gen.UpdateReviewStatusParams) (gen.Review, error) {
	row, err := s.write.UpdateReviewStatus(ctx, p)
	if err != nil {
		return gen.Review{}, fmt.Errorf("updating review status: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteReview(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteReview(ctx, gen.DeleteReviewParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting review: %w", err)
	}

	return nil
}

//nolint:unused // reserved for future features
func (s *Store) hasUserReviewedEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID, reviewerUserID pgtype.UUID) (bool, error) {
	exists, err := s.read.HasUserReviewedEntity(ctx, gen.HasUserReviewedEntityParams{
		TenantID:       tenantID,
		EntityType:     entityType,
		EntityID:       entityID,
		ReviewerUserID: reviewerUserID,
	})
	if err != nil {
		return false, fmt.Errorf("checking if user reviewed entity: %w", err)
	}

	return exists, nil
}

// ---------------------------------------------------------------------------
// Review Aggregates
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) upsertReviewAggregate(ctx context.Context, p gen.UpsertReviewAggregateParams) (gen.ReviewAggregate, error) {
	row, err := s.write.UpsertReviewAggregate(ctx, p)
	if err != nil {
		return gen.ReviewAggregate{}, fmt.Errorf("upserting review aggregate: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getReviewAggregate(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.ReviewAggregate, error) {
	row, err := s.read.GetReviewAggregate(ctx, gen.GetReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return gen.ReviewAggregate{}, fmt.Errorf("getting review aggregate: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) recalculateReviewAggregate(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.write.RecalculateReviewAggregate(ctx, gen.RecalculateReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("recalculating review aggregate: %w", err)
	}

	return nil
}
