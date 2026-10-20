package reviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
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

// ErrAlreadyReviewed is the typed one-per-user violation. Handlers map it
// to 409 already_reviewed; the DB unique guard (entity_type, entity_id,
// reviewer_user_id) is the authoritative backstop for concurrent submits.
var ErrAlreadyReviewed = errors.New("user has already reviewed this entity")

// ErrSelfReview — the reviewer authored the content being reviewed
// (content-backed entity types resolve entity_id to contents.id).
// Handlers map it to 409 self_review.
var ErrSelfReview = errors.New("cannot review your own content")

// CreateReview creates a new review, enforcing one-per-user-per-entity.
func (s *Service) CreateReview(ctx context.Context, reviewerUserID pgtype.UUID, input CreateReviewInput) (gen.Review, error) {
	if input.EntityType == "" {
		return gen.Review{}, errors.New("entity_type is required")
	}

	if input.Rating < RatingMin || input.Rating > RatingMax {
		return gen.Review{}, fmt.Errorf("rating must be between %d and %d", RatingMin, RatingMax)
	}

	if len([]rune(input.Title)) > MaxTitleRunes {
		return gen.Review{}, fmt.Errorf("title must be at most %d characters", MaxTitleRunes)
	}

	if len([]rune(input.Body)) > MaxBodyRunes {
		return gen.Review{}, fmt.Errorf("body must be at most %d characters", MaxBodyRunes)
	}

	if len(input.Body) > MaxBodyBytes {
		return gen.Review{}, errors.New("body exceeds size limit")
	}

	// Sanitize on write (mirror content/store.go:460): titles are plain
	// text, bodies go through the whitelist HTML sanitizer.
	input.Title = content.SanitizeText(input.Title)
	input.Body = content.SanitizeHTML(input.Body)

	// Self-review guard — for content-backed entities (service_listing,
	// …) entity_id is a contents id, and an author rating their own work
	// inflates the aggregate. Non-content entities (no contents row) skip
	// the guard; a lookup error is non-fatal so a flaky read never blocks
	// a legitimate review.
	authorID, err := s.store.contentAuthorID(ctx, input.EntityID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("reviews: content-author lookup failed", "entity_id", input.EntityID, "error", err)
	}
	if err == nil && authorID == reviewerUserID {
		return gen.Review{}, ErrSelfReview
	}

	// One-per-user enforcement.
	exists, err := s.store.hasUserReviewedEntity(ctx, input.EntityType, input.EntityID, reviewerUserID)
	if err != nil {
		return gen.Review{}, fmt.Errorf("checking existing review: %w", err)
	}

	if exists {
		return gen.Review{}, ErrAlreadyReviewed
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
		EntityType:     input.EntityType,
		EntityID:       input.EntityID,
		ReviewerUserID: reviewerUserID,
		Rating:         input.Rating,
		Title:          title,
		Body:           body,
	})
	if err != nil {
		// Concurrent double-submit can win the check-then-insert race; the
		// unique guard is authoritative — map its violation to the sentinel
		// so handlers return 409 instead of 500 (R1).
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.Review{}, ErrAlreadyReviewed
		}

		return gen.Review{}, err
	}

	// Best-effort aggregate update (non-blocking failure).
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		EntityType: input.EntityType,
		EntityID:   input.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", input.EntityType, "error", err)
	}

	s.emitWebhook(ctx, "review.created", map[string]any{
		"review_id":   review.ID,
		"reviewee_id": review.EntityID,
		"reviewer_id": review.ReviewerUserID,
		"rating":      review.Rating,
	})

	return review, nil
}

// GetReview retrieves a review by ID.
func (s *Service) GetReview(ctx context.Context, id pgtype.UUID) (gen.Review, error) {
	return s.store.getReviewByID(ctx, id)
}

// ListReviewsByEntity returns published reviews for an entity.
// sort controls ordering: "newest" (default), "highest", or "lowest" (spec §29).
func (s *Service) ListReviewsByEntity(ctx context.Context, entityType string, entityID pgtype.UUID, limit, offset int32, sort string) ([]gen.Review, error) {
	return s.store.listReviewsByEntity(ctx, entityType, entityID, limit, offset, sort)
}

// ListReviewsByEntityWithAuthor returns published reviews for an entity with the
// reviewer's display name/username attached.
func (s *Service) ListReviewsByEntityWithAuthor(ctx context.Context, entityType string, entityID pgtype.UUID, limit, offset int32) ([]gen.ListReviewsByEntityWithAuthorRow, error) {
	return s.store.listReviewsByEntityWithAuthor(ctx, entityType, entityID, limit, offset)
}

// ListReviewsByReviewer returns all reviews by a reviewer.
func (s *Service) ListReviewsByReviewer(ctx context.Context, reviewerUserID pgtype.UUID, limit, offset int32) ([]gen.Review, error) {
	return s.store.listReviewsByReviewer(ctx, reviewerUserID, limit, offset)
}

// ListReviews returns all reviews for a platform (admin moderation view),
// regardless of entity or reviewer and including all statuses.
func (s *Service) ListReviews(ctx context.Context, limit, offset int32) ([]gen.Review, error) {
	return s.store.listReviews(ctx, limit, offset)
}

// ListAdminReviews returns all reviews for a platform (admin moderation view)
// with resolved entity names.
func (s *Service) ListAdminReviews(ctx context.Context, limit, offset int32) ([]gen.ListAdminReviewsRow, error) {
	return s.store.listAdminReviews(ctx, limit, offset)
}

// ListAdminReviewsFiltered returns filtered reviews with entity names.
func (s *Service) ListAdminReviewsFiltered(ctx context.Context, status, entityType string, limit, offset int32) ([]gen.ListAdminReviewsFilteredRow, error) {
	return s.store.listAdminReviewsFiltered(ctx, status, entityType, limit, offset)
}

// CountAdminReviews returns the total review count for a platform.
func (s *Service) CountAdminReviews(ctx context.Context) (int32, error) {
	return s.store.countAdminReviews(ctx)
}

// CountAdminReviewsFiltered returns the filtered review count.
func (s *Service) CountAdminReviewsFiltered(ctx context.Context, status, entityType string) (int32, error) {
	return s.store.countAdminReviewsFiltered(ctx, status, entityType)
}

// UpdateReview updates an existing review (owner only).
func (s *Service) UpdateReview(ctx context.Context, id, reviewerUserID pgtype.UUID, rating int16, title, body string) (gen.Review, error) {
	if rating < RatingMin || rating > RatingMax {
		return gen.Review{}, fmt.Errorf("rating must be between %d and %d", RatingMin, RatingMax)
	}

	if len([]rune(title)) > MaxTitleRunes {
		return gen.Review{}, fmt.Errorf("title must be at most %d characters", MaxTitleRunes)
	}

	if len([]rune(body)) > MaxBodyRunes {
		return gen.Review{}, fmt.Errorf("body must be at most %d characters", MaxBodyRunes)
	}

	if len(body) > MaxBodyBytes {
		return gen.Review{}, errors.New("body exceeds size limit")
	}

	// Sanitize on write (mirror content/store.go:460): titles are plain
	// text, bodies go through the whitelist HTML sanitizer.
	title = content.SanitizeText(title)
	body = content.SanitizeHTML(body)

	var t *string
	if title != "" {
		t = &title
	}

	var b *string
	if body != "" {
		b = &body
	}

	review, err := s.store.updateReview(ctx, gen.UpdateReviewParams{
		ID: id,

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
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return review, nil
}

// DeleteReview removes a review by setting status to 'removed' (soft delete).
// The reviewerUserID parameter enforces owner-only deletion (BUG-001): the
// SQL query filters by reviewer_user_id so a user can only delete their own
// review, even if they hold the reviews.delete_own permission.
func (s *Service) DeleteReview(ctx context.Context, id, reviewerUserID pgtype.UUID) error {
	review, err := s.store.getReviewByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get review for delete: %w", err)
	}

	_, err = s.store.softDeleteOwnReview(ctx, gen.SoftDeleteOwnReviewParams{
		ID: id,

		ReviewerUserID: reviewerUserID,
	})
	if err != nil {
		return fmt.Errorf("soft delete own review: %w", err)
	}

	// Best-effort aggregate update.
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return nil
}

// HardDeleteReview permanently removes a review from the database (admin only).
func (s *Service) HardDeleteReview(ctx context.Context, id pgtype.UUID) error {
	review, err := s.store.getReviewByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get review for hard delete: %w", err)
	}

	if err := s.store.deleteReview(ctx, id); err != nil {
		return fmt.Errorf("hard delete review: %w", err)
	}

	// Best-effort aggregate update.
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return nil
}

// FlagReview sets a review's status to 'flagged' for moderation.
// A removed review cannot be flagged (it would un-remove it and re-include
// it in aggregate recalculation — spec §29 L33).
func (s *Service) FlagReview(ctx context.Context, id pgtype.UUID) error {
	review, err := s.store.getReviewByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get review for flag: %w", err)
	}

	if review.Status == ReviewStatusRemoved {
		return errors.New("cannot_flag_removed: review has been removed and cannot be flagged")
	}

	_, err = s.store.updateReviewStatus(ctx, gen.UpdateReviewStatusParams{
		ID: id,

		Status:    ReviewStatusFlagged,
		RemovedBy: pgtype.UUID{}, // flagging does not remove — clear removed_by
	})
	if err != nil {
		return fmt.Errorf("flag review: %w", err)
	}

	// Best-effort aggregate update.
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return nil
}

// ModerateReview changes a review's status (admin operation).
// The moderatorUserID and reason are recorded in audit_logs per spec §29
// (moderation reuses the Reporting & Appeals system, Section 22).
func (s *Service) ModerateReview(ctx context.Context, id, moderatorUserID pgtype.UUID, status, reason string) (gen.Review, error) {
	if status == "" {
		return gen.Review{}, errors.New("status is required")
	}

	if status != ReviewStatusPublished && status != ReviewStatusFlagged && status != ReviewStatusRemoved {
		return gen.Review{}, fmt.Errorf("invalid status: %s", status)
	}

	// Record who removed the review (spec §29 L55: removed_by).
	// Only set when status='removed'; clear for publish/flag.
	var removedBy pgtype.UUID
	if status == ReviewStatusRemoved {
		removedBy = moderatorUserID
	}

	review, err := s.store.updateReviewStatus(ctx, gen.UpdateReviewStatusParams{
		ID: id,

		Status:    status,
		RemovedBy: removedBy,
	})
	if err != nil {
		return gen.Review{}, fmt.Errorf("update review status: %w", err)
	}

	// Record the moderation action in audit_logs with the reason (spec §29/§22).
	targetType := "review"
	metadata, _ := json.Marshal(map[string]string{
		"entity_type": review.EntityType,
		"entity_id":   review.EntityID.String(),
		"old_status":  review.Status,
		"new_status":  status,
		"reason":      reason,
	})

	if _, auditErr := s.store.createAuditLog(ctx, gen.CreateAuditLogParams{
		ActorUserID: moderatorUserID,
		ActionType:  "review.moderate",
		TargetType:  &targetType,
		TargetID:    id,
		Metadata:    metadata,
	}); auditErr != nil {
		slog.Warn("reviews: failed to write moderation audit log", "review_id", id.String(), "error", auditErr)
	}

	// Best-effort aggregate update.
	if _, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		EntityType: review.EntityType,
		EntityID:   review.EntityID,
	}); err != nil {
		slog.Error("upsert review aggregate", "entity_type", review.EntityType, "error", err)
	}

	return review, nil
}

// GetAggregate returns the review aggregate for an entity.
func (s *Service) GetAggregate(ctx context.Context, entityType string, entityID pgtype.UUID) (gen.ReviewAggregate, error) {
	return s.store.getReviewAggregate(ctx, entityType, entityID)
}

// RecalculateAggregate deletes and re-upserts the aggregate from scratch.
func (s *Service) RecalculateAggregate(ctx context.Context, entityType string, entityID pgtype.UUID) error {
	if err := s.store.recalculateReviewAggregate(ctx, entityType, entityID); err != nil {
		return fmt.Errorf("recalculate review aggregate: %w", err)
	}

	_, err := s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return fmt.Errorf("upsert review aggregate: %w", err)
	}

	return nil
}
