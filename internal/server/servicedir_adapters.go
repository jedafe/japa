// Package server — servicedir_adapters.go provides the wiring-layer adapter
// that bridges the §29 reviews subsystem to the servicedir.ReviewFetcher
// interface. The adapter exists because the servicedir package defines its
// own ReviewFetcher interface (returning servicedir.ReviewItem /
// servicedir.ReviewAggregate) to avoid importing internal/reviews, while
// the reviews subsystem returns gen.Review / gen.ReviewAggregate rows.
//
// Ref: japaconnect-engineering/sections/98-service-directory.md (BUG-014:
// GET /services/:slug must return "service detail + reviews" per §98).
// Ref: japaconnect-engineering/sections/29-ratings-and-reviews.md
package server

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/payments"
	"github.com/jedafe/japa/internal/servicedir"
)

// serviceReviewAdapter adapts a pair of *gen.Queries (read pool) to the
// servicedir.ReviewFetcher interface. It queries the polymorphic reviews
// table (§29) with entity_type='service_listing' and joins users +
// user_profiles so author display info is populated without N+1 lookups.
type serviceReviewAdapter struct {
	read *gen.Queries
}

// ListByEntity returns up to `limit` published reviews for the given
// service_listing content_id, ordered by created_at DESC (latest first).
// The `sort` param is accepted for interface compatibility; the underlying
// query always returns latest-first (matching the §29 default sort).
func (a serviceReviewAdapter) ListByEntity(ctx context.Context, entityType string, entityID pgtype.UUID, limit, offset int32, sort string) ([]servicedir.ReviewItem, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := a.read.ListReviewsByEntityWithAuthor(ctx, gen.ListReviewsByEntityWithAuthorParams{

		EntityType: entityType,
		EntityID:   entityID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, err
	}

	items := make([]servicedir.ReviewItem, 0, len(rows))

	for _, r := range rows {
		item := servicedir.ReviewItem{
			ID:                r.ID.String(),
			AuthorID:          r.ReviewerUserID.String(),
			AuthorUsername:    derefStr(r.AuthorUsername),
			AuthorDisplayName: r.AuthorDisplayName,
			AuthorAvatarURL:   r.AuthorAvatarUrl,
			Rating:            r.Rating,
			Title:             r.Title,
			Body:              r.Body,
			CreatedAt:         r.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}
		items = append(items, item)
	}

	return items, nil
}

// Aggregate returns the review count + average rating for the given
// service_listing content_id from the review_aggregates rollup table (§29).
// Returns zeros when no aggregate row exists yet (no reviews posted).
func (a serviceReviewAdapter) Aggregate(ctx context.Context, entityType string, entityID pgtype.UUID) (servicedir.ReviewAggregate, error) {
	agg, err := a.read.GetReviewAggregate(ctx, gen.GetReviewAggregateParams{

		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		// No aggregate row = no reviews yet. Return zeros.
		return servicedir.ReviewAggregate{}, nil
	}

	avg, _ := agg.AverageRating.Float64Value()

	return servicedir.ReviewAggregate{
		Count: agg.ReviewCount,
		Avg:   avg.Float64,
	}, nil
}

// serviceDirPaymentAdapter adapts *payments.Service to the
// servicedir.PaymentCreator interface (§111 platform-fee intents). Field
// mapping is one-to-one; the Idempotency-Key header propagates via ctx
// (mw.IdempotencyKeyFromContext → payments.idempotency_key, BUG-015).
type serviceDirPaymentAdapter struct {
	svc *payments.Service
}

// CreatePayment converts the servicedir input to payments.CreatePaymentInput
// and projects the result into servicedir.PaymentResult.
func (a *serviceDirPaymentAdapter) CreatePayment(ctx context.Context, userID pgtype.UUID, in servicedir.PaymentInput) (servicedir.PaymentResult, error) {
	res, err := a.svc.CreatePayment(ctx, userID, payments.CreatePaymentInput{
		Provider:    in.Provider,
		PurposeType: in.PurposeType,
		PurposeID:   in.PurposeID,
		Amount:      in.Amount,
		Currency:    in.Currency,
		Email:       in.Email,
		Metadata:    in.Metadata,
		ReturnURL:   in.ReturnURL,
	})
	if err != nil {
		return servicedir.PaymentResult{}, err
	}

	return servicedir.PaymentResult{
		PaymentID:   res.Payment.ID.String(),
		CheckoutURL: res.CheckoutURL,
		Status:      res.Payment.Status,
	}, nil
}

// ResumePayment adapts payments.ResumePayment for the servicedir seam
// (§111 checkout resume). Payments-layer sentinels are translated to
// servicedir ones so the handler's error mapping stays self-contained.
func (a *serviceDirPaymentAdapter) ResumePayment(ctx context.Context, paymentID, userID pgtype.UUID, email, returnURL string) (servicedir.PaymentResult, error) {
	res, err := a.svc.ResumePayment(ctx, paymentID, userID, email, returnURL)
	if err != nil {
		switch {
		case errors.Is(err, payments.ErrPaymentNotPending):
			return servicedir.PaymentResult{}, servicedir.ErrInvalidBookingTransition
		case errors.Is(err, payments.ErrPaymentNotOwned):
			return servicedir.PaymentResult{}, servicedir.ErrNotOwner
		default:
			return servicedir.PaymentResult{}, err
		}
	}

	return servicedir.PaymentResult{
		PaymentID:   res.Payment.ID.String(),
		CheckoutURL: res.CheckoutURL,
		Status:      res.Payment.Status,
	}, nil
}
