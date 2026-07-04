package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides database access for AI HITL review operations.
// Uses raw SQL instead of sqlc because the ai_hitl_reviews table is simple
// enough that generated code would add indirection without value.
type Store struct {
	write *pgxpool.Pool
	read  *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pools.
func NewStore(read, write *pgxpool.Pool) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// HITL Review CRUD
// ---------------------------------------------------------------------------

// HITLReview represents a human-in-the-loop review record.
type HITLReview struct {
	ID            pgtype.UUID
	TenantID      pgtype.UUID
	ContentID     pgtype.UUID
	ContentType   string
	AIOutput      []byte // JSONB
	HumanFeedback []byte // JSONB, nullable
	Status        string
	ReviewerID    pgtype.UUID // nullable
	Reason        string      // nullable
	CreatedAt     time.Time
	ReviewedAt    *time.Time
}

// CreateHITLReview inserts a new HITL review record and returns it.
func (s *Store) CreateHITLReview(ctx context.Context, review HITLReview) (HITLReview, error) {
	var result HITLReview

	err := s.write.QueryRow(
		ctx, `
		INSERT INTO ai_hitl_reviews
			(tenant_id, content_id, content_type, ai_output, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, content_id, content_type, ai_output, status, created_at
	`, review.TenantID, review.ContentID, review.ContentType, review.AIOutput, review.Status,
	).Scan(&result.ID, &result.TenantID, &result.ContentID, &result.ContentType,
		&result.AIOutput, &result.Status, &result.CreatedAt)
	if err != nil {
		return HITLReview{}, fmt.Errorf("creating HITL review: %w", err)
	}

	return result, nil
}

// ApproveHITLReview sets the review status to 'approved' with the reviewer identity.
func (s *Store) ApproveHITLReview(ctx context.Context, reviewID, reviewerID pgtype.UUID) error {
	tag, err := s.write.Exec(ctx, `
		UPDATE ai_hitl_reviews
		SET status = 'approved', reviewer_id = $2, reviewed_at = now()
		WHERE id = $1 AND status = 'pending'
	`, reviewID, reviewerID)
	if err != nil {
		return fmt.Errorf("approving HITL review: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errors.New("not_found: review not found or already processed")
	}

	return nil
}

// RejectHITLReview sets the review status to 'rejected' with a reason.
func (s *Store) RejectHITLReview(ctx context.Context, reviewID, reviewerID pgtype.UUID, reason string) error {
	tag, err := s.write.Exec(ctx, `
		UPDATE ai_hitl_reviews
		SET status = 'rejected', reviewer_id = $2, reviewed_at = now(), reason = $3
		WHERE id = $1 AND status = 'pending'
	`, reviewID, reviewerID, reason)
	if err != nil {
		return fmt.Errorf("rejecting HITL review: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errors.New("not_found: review not found or already processed")
	}

	return nil
}

// ListPendingHITLReviews returns pending reviews for a tenant with offset pagination.
func (s *Store) ListPendingHITLReviews(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]HITLReview, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.read.Query(ctx, `
		SELECT id, tenant_id, content_id, content_type, ai_output, status, created_at
		FROM ai_hitl_reviews
		WHERE tenant_id = $1 AND status = 'pending'
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing pending HITL reviews: %w", err)
	}

	defer rows.Close()

	var reviews []HITLReview

	for rows.Next() {
		var r HITLReview
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ContentID, &r.ContentType,
			&r.AIOutput, &r.Status, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning HITL review: %w", err)
		}

		reviews = append(reviews, r)
	}

	return reviews, nil
}
