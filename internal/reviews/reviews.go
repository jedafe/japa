// Package reviews implements the Universal Ratings & Reviews system (Section 29).
//
// Polymorphic ratings/reviews shared across Organizations, Universities/Mentors,
// Services, and any future rateable entity. One review per user per entity,
// maintained aggregate rollup via UpsertReviewAggregate. Reviews reuse the
// existing Reporting & Appeals system (Section 22) for moderation rather than
// a review-specific moderation flow.
package reviews

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Review statuses.
const (
	ReviewStatusPublished = "published"
	ReviewStatusFlagged   = "flagged"
	ReviewStatusRemoved   = "removed"
)

// Rating bounds.
const (
	RatingMin int16 = 1
	RatingMax int16 = 5
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// webhookEmitter fans a review.* event out to all subscribed webhook
// endpoints for the tenant (fire-and-forget). Ref: Section 12 (webhooks).
type webhookEmitter interface {
	EmitEvent(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any)
}

// Service provides review operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	webhooks webhookEmitter
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db: database,
	}
}

// SetWebhookEmitter injects the review.* event fan-out (Section 12).
func (s *Service) SetWebhookEmitter(w webhookEmitter) { s.webhooks = w }

// emitWebhook fires a review.* event fire-and-forget (Rule 8). Errors are
// logged but never returned to the caller — webhook delivery is async.
func (s *Service) emitWebhook(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any) {
	if s.webhooks == nil {
		return
	}

	s.webhooks.EmitEvent(ctx, tenantID, eventType, payload)
}
