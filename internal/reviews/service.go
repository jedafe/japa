package reviews

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CreateReviewInput holds parameters for creating a new review.
type CreateReviewInput struct {
	EntityType string
	EntityID   pgtype.UUID
	Rating     int16
	Title      string
	Body       string
}

// CreateReview creates a new review, enforcing one-per-user-per-entity.
func (s *Service) CreateReview(ctx context.Context, tenantID, reviewerUserID pgtype.UUID, input CreateReviewInput) (gen.Review, error) {
	if input.EntityType == "" {
		return gen.Review{}, errors.New("entity_type is required")
	}

	if input.Rating < RatingMin || input.Rating > RatingMax {
		return gen.Review{}, fmt.Errorf("rating must be between %d and %d", RatingMin, RatingMax)
	}

	// One-per-user enforcement.
	exists, err := s.store.hasUserReviewedEntity(ctx, tenantID, input.EntityType, input.EntityID, reviewerUserID)
	if err != nil {
		return gen.Review{}, fmt.Errorf("checking existing review: %w", err)
	}

	if exists {
		return gen.Review{}, errors.New("already_exists: user has already reviewed this entity")
	}

	var title *string
	if input.Title != "" {
		title = &input.Title
	}

	var body *string
	if input.Body != "" {
		body = &input.Body
	}

	review, err := s.store.createReview(ctx, gen.CreateReviewParams{
		TenantID:       tenantID,
		EntityType:     input.EntityType,
		EntityID:       input.EntityID,
		ReviewerUserID: reviewerUserID,
		Rating:         input.Rating,
		Title:          title,
		Body:           body,
	})
	if err != nil {
		return gen.Review{}, err
	}

	// Best-effort aggregate update (non-blocking failure).
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: input.EntityType,
		EntityID:   input.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", input.EntityType, "error", err)
	}

	return review, nil
}

// GetReview retrieves a review by ID.
func (s *Service) GetReview(ctx context.Context, id, tenantID pgtype.UUID) (gen.Review, error) {
	return s.store.getReviewByID(ctx, id, tenantID)
}

// ListReviewsByEntity returns published reviews for an entity.
func (s *Service) ListReviewsByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, limit, offset int32) ([]gen.Review, error) {
	return s.store.listReviewsByEntity(ctx, tenantID, entityType, entityID, limit, offset)
}

// ListReviewsByReviewer returns all reviews by a reviewer.
func (s *Service) ListReviewsByReviewer(ctx context.Context, tenantID, reviewerUserID pgtype.UUID, limit, offset int32) ([]gen.Review, error) {
	return s.store.listReviewsByReviewer(ctx, tenantID, reviewerUserID, limit, offset)
}

// UpdateReview updates an existing review (owner only).
func (s *Service) UpdateReview(ctx context.Context, id, tenantID, reviewerUserID pgtype.UUID, rating int16, title, body string) (gen.Review, error) {
	if rating < RatingMin || rating > RatingMax {
		return gen.Review{}, fmt.Errorf("rating must be between %d and %d", RatingMin, RatingMax)
	}

	var t *string
	if title != "" {
		t = &title
	}

	var b *string
	if body != "" {
		b = &body
	}

	review, err := s.store.updateReview(ctx, gen.UpdateReviewParams{
		ID:             id,
		TenantID:       tenantID,
		ReviewerUserID: reviewerUserID,
		Rating:         rating,
		Title:          t,
		Body:           b,
	})
	if err != nil {
		return gen.Review{}, err
	}

	// Best-effort aggregate update.
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return review, nil
}

// DeleteReview removes a review by setting status to 'removed' (soft delete).
func (s *Service) DeleteReview(ctx context.Context, id, tenantID pgtype.UUID) error {
	review, err := s.store.getReviewByID(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("get review for delete: %w", err)
	}

	_, err = s.store.updateReviewStatus(ctx, gen.UpdateReviewStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   ReviewStatusRemoved,
	})
	if err != nil {
		return fmt.Errorf("update review status: %w", err)
	}

	// Best-effort aggregate update.
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return nil
}

// FlagReview sets a review's status to 'flagged' for moderation.
func (s *Service) FlagReview(ctx context.Context, id, tenantID pgtype.UUID) error {
	review, err := s.store.getReviewByID(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("get review for flag: %w", err)
	}

	_, err = s.store.updateReviewStatus(ctx, gen.UpdateReviewStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   ReviewStatusFlagged,
	})
	if err != nil {
		return fmt.Errorf("flag review: %w", err)
	}

	// Best-effort aggregate update.
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return nil
}

// ModerateReview changes a review's status (admin operation).
func (s *Service) ModerateReview(ctx context.Context, id, tenantID pgtype.UUID, status string) (gen.Review, error) {
	if status == "" {
		return gen.Review{}, errors.New("status is required")
	}

	if status != ReviewStatusPublished && status != ReviewStatusFlagged && status != ReviewStatusRemoved {
		return gen.Review{}, fmt.Errorf("invalid status: %s", status)
	}

	review, err := s.store.updateReviewStatus(ctx, gen.UpdateReviewStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return gen.Review{}, fmt.Errorf("update review status: %w", err)
	}

	// Best-effort aggregate update.
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return review, nil
}

// GetAggregate returns the review aggregate for an entity.
func (s *Service) GetAggregate(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.ReviewAggregate, error) {
	return s.store.getReviewAggregate(ctx, tenantID, entityType, entityID)
}

// RecalculateAggregate deletes and re-upserts the aggregate from scratch.
func (s *Service) RecalculateAggregate(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.store.recalculateReviewAggregate(ctx, tenantID, entityType, entityID); err != nil {
		return fmt.Errorf("recalculate review aggregate: %w", err)
	}

	_, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return fmt.Errorf("upsert review aggregate: %w", err)
	}

	return nil
}
