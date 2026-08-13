// Package reviewerctx carries the HITL reviewer user ID through the
// promotion pipeline (INTEL-026 fix, ADR-088). It is a standalone package
// to avoid import cycles between the worker and promoter packages.
//
// When a record is approved via the review queue, the worker sets the
// reviewer user ID in the context via WithReviewerID. The promoter reads
// it via ReviewerFromContext to stamp hitl_reviewed_by on the contents row.
package reviewerctx

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type ctxKey struct{}

// WithReviewerID returns a new context that carries the reviewer user ID.
func WithReviewerID(ctx context.Context, reviewerID pgtype.UUID) context.Context {
	return context.WithValue(ctx, ctxKey{}, reviewerID)
}

// ReviewerFromContext extracts the HITL reviewer user ID from the context.
// Returns (pgtype.UUID{}, false) when no reviewer is set (auto-published
// records have no human reviewer).
func ReviewerFromContext(ctx context.Context) (pgtype.UUID, bool) {
	v, ok := ctx.Value(ctxKey{}).(pgtype.UUID)
	if !ok || !v.Valid {
		return pgtype.UUID{}, false
	}
	return v, true
}
