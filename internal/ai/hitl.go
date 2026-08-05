package ai

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

// HITL status constants.
const (
	HITLStatusPending  = "pending"
	HITLStatusApproved = "approved"
	HITLStatusRejected = "rejected"
)

// CreateHITLReview inserts a new HITL review, validating required fields.
// content_id is optional (nullable per migration 000275) — admin-generated
// reviews may not have a backing content row.
func CreateHITLReview(ctx context.Context, store *Store, review HITLReview) (HITLReview, error) {
	if review.ContentType == "" {
		return HITLReview{}, errors.New("content_type is required")
	}

	if len(review.AIOutput) == 0 {
		return HITLReview{}, errors.New("ai_output is required")
	}

	review.Status = HITLStatusPending

	return store.CreateHITLReview(ctx, review)
}

// ApproveReview approves a pending HITL review. The tenantID scopes the
// update to the caller's tenant (Section 09 multi-tenancy).
func ApproveReview(ctx context.Context, store *Store, tenantID, reviewID, reviewerID pgtype.UUID) error {
	if reviewID == (pgtype.UUID{}) {
		return errors.New("review_id is required")
	}

	if reviewerID == (pgtype.UUID{}) {
		return errors.New("reviewer_id is required")
	}

	return store.ApproveHITLReview(ctx, tenantID, reviewID, reviewerID)
}

// RejectReview rejects a pending HITL review with a reason. The tenantID
// scopes the update to the caller's tenant (Section 09 multi-tenancy).
func RejectReview(ctx context.Context, store *Store, tenantID, reviewID, reviewerID pgtype.UUID, reason string) error {
	if reviewID == (pgtype.UUID{}) {
		return errors.New("review_id is required")
	}

	if reviewerID == (pgtype.UUID{}) {
		return errors.New("reviewer_id is required")
	}

	if reason == "" {
		return errors.New("rejection reason is required")
	}

	return store.RejectHITLReview(ctx, tenantID, reviewID, reviewerID, reason)
}

// ListPendingReviews returns pending HITL reviews for a tenant.
func ListPendingReviews(ctx context.Context, store *Store, tenantID pgtype.UUID, limit, offset int32) ([]HITLReview, error) {
	if tenantID == (pgtype.UUID{}) {
		return nil, errors.New("tenant_id is required")
	}

	return store.ListPendingHITLReviews(ctx, tenantID, limit, offset)
}

// CountPendingReviews returns the total count of pending HITL reviews for
// pagination metadata.
func CountPendingReviews(ctx context.Context, store *Store, tenantID pgtype.UUID) (int64, error) {
	if tenantID == (pgtype.UUID{}) {
		return 0, errors.New("tenant_id is required")
	}

	return store.CountPendingHITLReviews(ctx, tenantID)
}
