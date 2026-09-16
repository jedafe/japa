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

func (s *Store) createReview(ctx context.Context, p gen.CreateReviewParams) (gen.Review, error) {
	row, err := s.write.CreateReview(ctx, p)
	if err != nil {
		return gen.Review{}, fmt.Errorf("creating review: %w", err)
	}

	return row, nil
}

func (s *Store) getReviewByID(ctx context.Context, id pgtype.UUID) (gen.Review, error) {
	row, err := s.read.GetReviewByID(ctx, id)
	if err != nil {
		return gen.Review{}, fmt.Errorf("getting review by ID: %w", err)
	}

	return row, nil
}

func (s *Store) listReviewsByEntity(ctx context.Context, entityType string, entityID pgtype.UUID, limit, offset int32, sort string) ([]gen.Review, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	params := gen.ListReviewsByEntityParams{

		EntityType: entityType,
		EntityID:   entityID,
		PageLimit:  limit,
		PageOffset: offset,
	}

	switch sort {
	case "highest":
		rows, err := s.read.ListReviewsByEntityHighestRating(ctx, gen.ListReviewsByEntityHighestRatingParams{

			EntityType: entityType,
			EntityID:   entityID,
			PageLimit:  limit,
			PageOffset: offset,
		})
		if err != nil {
			return nil, fmt.Errorf("listing reviews by entity (highest): %w", err)
		}

		return rows, nil
	case "lowest":
		rows, err := s.read.ListReviewsByEntityLowestRating(ctx, gen.ListReviewsByEntityLowestRatingParams{

			EntityType: entityType,
			EntityID:   entityID,
			PageLimit:  limit,
			PageOffset: offset,
		})
		if err != nil {
			return nil, fmt.Errorf("listing reviews by entity (lowest): %w", err)
		}

		return rows, nil
	default: // "newest" or unspecified
		rows, err := s.read.ListReviewsByEntity(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("listing reviews by entity: %w", err)
		}

		return rows, nil
	}
}

func (s *Store) listReviewsByReviewer(ctx context.Context, reviewerUserID pgtype.UUID, limit, offset int32) ([]gen.Review, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListReviewsByReviewer(ctx, gen.ListReviewsByReviewerParams{

		ReviewerUserID: reviewerUserID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing reviews by reviewer: %w", err)
	}

	return rows, nil
}

// listReviewsByEntityWithAuthor returns published reviews for an entity joined
// with the reviewer's user profile so the UI can display author_name without
// extra round-trips (used by GET /spaces/{id}/reviews and learning tracks).
func (s *Store) listReviewsByEntityWithAuthor(ctx context.Context, entityType string, entityID pgtype.UUID, limit, offset int32) ([]gen.ListReviewsByEntityWithAuthorRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListReviewsByEntityWithAuthor(ctx, gen.ListReviewsByEntityWithAuthorParams{

		EntityType: entityType,
		EntityID:   entityID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing reviews by entity with author: %w", err)
	}

	return rows, nil
}

// listReviews returns all reviews for a platform (admin moderation view).
// Unlike ListReviewsByEntity/ListReviewsByReviewer it is not scoped to an
// entity or reviewer and includes all statuses, so moderators can see
// pending/flagged reviews too.
func (s *Store) listReviews(ctx context.Context, limit, offset int32) ([]gen.Review, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListReviews(ctx, gen.ListReviewsParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing reviews: %w", err)
	}

	return rows, nil
}

// listAdminReviews returns all reviews for a platform (admin moderation view)
// with resolved entity names via polymorphic lookup.
func (s *Store) listAdminReviews(ctx context.Context, limit, offset int32) ([]gen.ListAdminReviewsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListAdminReviews(ctx, gen.ListAdminReviewsParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing admin reviews: %w", err)
	}

	return rows, nil
}

// listAdminReviewsFiltered returns filtered reviews with entity names.
func (s *Store) listAdminReviewsFiltered(ctx context.Context, status, entityType string, limit, offset int32) ([]gen.ListAdminReviewsFilteredRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListAdminReviewsFiltered(ctx, gen.ListAdminReviewsFilteredParams{

		Status:     status,
		EntityType: entityType,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing filtered admin reviews: %w", err)
	}

	return rows, nil
}

// countAdminReviews returns the total count of reviews for a platform.
func (s *Store) countAdminReviews(ctx context.Context) (int32, error) {
	return s.read.CountAdminReviews(ctx)
}

// countAdminReviewsFiltered returns the filtered count of reviews.
func (s *Store) countAdminReviewsFiltered(ctx context.Context, status, entityType string) (int32, error) {
	return s.read.CountAdminReviewsFiltered(ctx, gen.CountAdminReviewsFilteredParams{

		Status:     status,
		EntityType: entityType,
	})
}

func (s *Store) updateReview(ctx context.Context, p gen.UpdateReviewParams) (gen.Review, error) {
	row, err := s.write.UpdateReview(ctx, p)
	if err != nil {
		return gen.Review{}, fmt.Errorf("updating review: %w", err)
	}

	return row, nil
}

func (s *Store) updateReviewStatus(ctx context.Context, p gen.UpdateReviewStatusParams) (gen.Review, error) {
	row, err := s.write.UpdateReviewStatus(ctx, p)
	if err != nil {
		return gen.Review{}, fmt.Errorf("updating review status: %w", err)
	}

	return row, nil
}

func (s *Store) softDeleteOwnReview(ctx context.Context, p gen.SoftDeleteOwnReviewParams) (gen.Review, error) {
	row, err := s.write.SoftDeleteOwnReview(ctx, p)
	if err != nil {
		return gen.Review{}, fmt.Errorf("soft deleting own review: %w", err)
	}

	return row, nil
}

func (s *Store) deleteReview(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteReview(ctx, id); err != nil {
		return fmt.Errorf("deleting review: %w", err)
	}

	return nil
}

func (s *Store) hasUserReviewedEntity(ctx context.Context, entityType string, entityID, reviewerUserID pgtype.UUID) (bool, error) {
	exists, err := s.read.HasUserReviewedEntity(ctx, gen.HasUserReviewedEntityParams{

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

func (s *Store) upsertReviewAggregate(ctx context.Context, p gen.UpsertReviewAggregateParams) (gen.ReviewAggregate, error) {
	row, err := s.write.UpsertReviewAggregate(ctx, p)
	if err != nil {
		return gen.ReviewAggregate{}, fmt.Errorf("upserting review aggregate: %w", err)
	}

	return row, nil
}

func (s *Store) getReviewAggregate(ctx context.Context, entityType string, entityID pgtype.UUID) (gen.ReviewAggregate, error) {
	row, err := s.read.GetReviewAggregate(ctx, gen.GetReviewAggregateParams{

		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return gen.ReviewAggregate{}, fmt.Errorf("getting review aggregate: %w", err)
	}

	return row, nil
}

func (s *Store) recalculateReviewAggregate(ctx context.Context, entityType string, entityID pgtype.UUID) error {
	if err := s.write.RecalculateReviewAggregate(ctx, gen.RecalculateReviewAggregateParams{

		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("recalculating review aggregate: %w", err)
	}

	return nil
}

// createAuditLog writes a moderation audit log entry (spec §29/§22).
func (s *Store) createAuditLog(ctx context.Context, p gen.CreateAuditLogParams) (gen.AuditLog, error) {
	row, err := s.write.CreateAuditLog(ctx, p)
	if err != nil {
		return gen.AuditLog{}, fmt.Errorf("creating audit log: %w", err)
	}

	return row, nil
}
