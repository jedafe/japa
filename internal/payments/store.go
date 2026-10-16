package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for payment operations. The Service
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
// Payments CRUD
// ---------------------------------------------------------------------------

func (s *Store) createPayment(ctx context.Context, p gen.CreatePaymentParams) (gen.Payment, error) {
	row, err := s.write.CreatePayment(ctx, p)
	if err != nil {
		return gen.Payment{}, fmt.Errorf("creating payment: %w", err)
	}

	return row, nil
}

func (s *Store) getPaymentByID(ctx context.Context, id pgtype.UUID) (gen.Payment, error) {
	row, err := s.read.GetPaymentByID(ctx, id)
	if err != nil {
		return gen.Payment{}, fmt.Errorf("getting payment by ID: %w", err)
	}

	return row, nil
}

func (s *Store) listPaymentsByUser(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Payment, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPaymentsByUser(ctx, gen.ListPaymentsByUserParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing payments by user: %w", err)
	}

	return rows, nil
}

func (s *Store) listPaymentsByStatus(ctx context.Context, status string, limit, offset int32) ([]gen.Payment, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPaymentsByStatus(ctx, gen.ListPaymentsByStatusParams{
		Status:     status,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing payments by status: %w", err)
	}

	return rows, nil
}

func (s *Store) updatePaymentStatus(ctx context.Context, id pgtype.UUID, status string) (gen.Payment, error) {
	row, err := s.write.UpdatePaymentStatus(ctx, gen.UpdatePaymentStatusParams{
		ID: id,

		Status: status,
	})
	if err != nil {
		return gen.Payment{}, fmt.Errorf("updating payment status: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Webhook Events
// ---------------------------------------------------------------------------

// nolint:unused // reserved for future features
func (s *Store) insertWebhookEvent(ctx context.Context, p gen.InsertWebhookEventParams) (gen.PaymentWebhookEvent, error) {
	row, err := s.write.InsertWebhookEvent(ctx, p)
	if err != nil {
		return gen.PaymentWebhookEvent{}, fmt.Errorf("inserting webhook event: %w", err)
	}

	return row, nil
}

// insertWebhookEventIfNew atomically inserts a webhook event and returns
// true if this was the first insert (new event), false if the event already
// existed (ON CONFLICT DO NOTHING). This replaces the non-atomic
// read-check + insert pattern and prevents duplicate task enqueue under
// concurrent webhook delivery (BUG-011).
func (s *Store) insertWebhookEventIfNew(ctx context.Context, p gen.InsertWebhookEventParams) (bool, error) {
	row, err := s.write.InsertWebhookEvent(ctx, p)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ON CONFLICT DO NOTHING returned no rows — event already exists.
			return false, nil
		}

		return false, fmt.Errorf("inserting webhook event: %w", err)
	}

	_ = row

	return true, nil
}

func (s *Store) isWebhookEventProcessed(ctx context.Context, provider, eventID string) (bool, error) {
	processed, err := s.read.IsWebhookEventProcessed(ctx, gen.IsWebhookEventProcessedParams{
		Provider: provider,
		EventID:  eventID,
	})
	if err != nil {
		return false, fmt.Errorf("checking webhook event processed: %w", err)
	}

	return processed, nil
}

func (s *Store) markWebhookEventProcessed(ctx context.Context, provider, eventID string) error {
	if err := s.write.MarkWebhookEventProcessed(ctx, gen.MarkWebhookEventProcessedParams{
		Provider: provider,
		EventID:  eventID,
	}); err != nil {
		return fmt.Errorf("marking webhook event processed: %w", err)
	}

	return nil
}

// getPaymentByProviderRef looks up a payment by its provider reference
// (Stripe pi_.../cs_..., Paystack reference, NOWPayments order_id) using
// idx_payments_provider_ref. Replaces the full-scan match in the webhook
// worker (BUG-019).
func (s *Store) getPaymentByProviderRef(ctx context.Context, providerRef string) (gen.Payment, error) {
	row, err := s.read.GetPaymentByProviderRef(ctx,

		&providerRef,
	)
	if err != nil {
		return gen.Payment{}, fmt.Errorf("getting payment by provider_ref: %w", err)
	}

	return row, nil
}

// countDiscountCodeUses returns the total number of uses of a discount code
// across all users. Replaces the list+len pattern (BUG-018).
func (s *Store) countDiscountCodeUses(ctx context.Context, codeID pgtype.UUID) (int32, error) {
	count, err := s.read.CountDiscountCodeUses(ctx, codeID)
	if err != nil {
		return 0, fmt.Errorf("counting discount code uses: %w", err)
	}

	return count, nil
}

// listAdminPayments lists payments with admin filters (JEOS §17).
func (s *Store) listAdminPayments(ctx context.Context, p gen.ListAdminPaymentsParams) ([]gen.Payment, error) {
	rows, err := s.read.ListAdminPayments(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("listing admin payments: %w", err)
	}

	return rows, nil
}

// getAdminPaymentByID retrieves a payment for admin detail view (JEOS §17).
func (s *Store) getAdminPaymentByID(ctx context.Context, id pgtype.UUID) (gen.Payment, error) {
	row, err := s.read.GetAdminPaymentByID(ctx,
		id,
	)
	if err != nil {
		return gen.Payment{}, fmt.Errorf("getting admin payment: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Subscriptions
// ---------------------------------------------------------------------------

// nolint:unused // reserved for future features
func (s *Store) createSubscription(ctx context.Context, p gen.CreateSubscriptionParams) (gen.Subscription, error) {
	row, err := s.write.CreateSubscription(ctx, p)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("creating subscription: %w", err)
	}

	return row, nil
}

func (s *Store) getSubscriptionByID(ctx context.Context, id pgtype.UUID) (gen.Subscription, error) {
	row, err := s.read.GetSubscriptionByID(ctx, id)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("getting subscription by ID: %w", err)
	}

	return row, nil
}

func (s *Store) getActiveSubscriptionByUser(ctx context.Context, userID pgtype.UUID) (gen.Subscription, error) {
	row, err := s.read.GetActiveSubscriptionByUser(ctx,

		userID,
	)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("getting active subscription by user: %w", err)
	}

	return row, nil
}

func (s *Store) listSubscriptionsByUser(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Subscription, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListSubscriptionsByUser(ctx, gen.ListSubscriptionsByUserParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions by user: %w", err)
	}

	return rows, nil
}

// nolint:unused // reserved for future features
func (s *Store) listActiveSubscriptions(ctx context.Context, limit, offset int32) ([]gen.Subscription, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListActiveSubscriptions(ctx, gen.ListActiveSubscriptionsParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing active subscriptions: %w", err)
	}

	return rows, nil
}

// nolint:unused // reserved for future features
func (s *Store) updateSubscriptionStatus(ctx context.Context, id pgtype.UUID, status string) (gen.Subscription, error) {
	row, err := s.write.UpdateSubscriptionStatus(ctx, gen.UpdateSubscriptionStatusParams{
		ID: id,

		Status: status,
	})
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("updating subscription status: %w", err)
	}

	return row, nil
}

func (s *Store) cancelSubscription(ctx context.Context, id pgtype.UUID) (gen.Subscription, error) {
	row, err := s.write.CancelSubscription(ctx,
		id,
	)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("canceling subscription: %w", err)
	}

	return row, nil
}

// reactivateSubscription reactivates the caller's most recently canceled
// subscription for the platform. The subquery in the sqlc query already scopes
// by (user_id, , status='canceled'), so we only forward the params.
func (s *Store) reactivateSubscription(ctx context.Context, userID pgtype.UUID) (gen.Subscription, error) {
	row, err := s.write.ReactivateSubscription(ctx,
		userID,
	)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("reactivating subscription: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Recurring-billing lifecycle (JEOS §17, spec 2026-09-24-recurring-pro-billing)
// ---------------------------------------------------------------------------

func (s *Store) getSubscriptionByProviderSubID(ctx context.Context, subID string) (gen.Subscription, error) {
	row, err := s.read.GetSubscriptionByProviderSubID(ctx, &subID)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("getting subscription by provider_sub_id: %w", err)
	}

	return row, nil
}

func (s *Store) renewSubscriptionPeriod(ctx context.Context, id pgtype.UUID, start, end time.Time) (gen.Subscription, error) {
	row, err := s.write.RenewSubscriptionPeriod(ctx, gen.RenewSubscriptionPeriodParams{
		ID:                 id,
		CurrentPeriodStart: pgtype.Timestamptz{Time: start, Valid: true},
		CurrentPeriodEnd:   pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("renewing subscription period: %w", err)
	}

	return row, nil
}

func (s *Store) markSubscriptionPastDue(ctx context.Context, id pgtype.UUID) (gen.Subscription, error) {
	row, err := s.write.MarkSubscriptionPastDue(ctx, id)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("marking subscription past_due: %w", err)
	}

	return row, nil
}

func (s *Store) setSubscriptionCancelAtPeriodEnd(ctx context.Context, id pgtype.UUID, flag bool) error {
	_, err := s.write.SetSubscriptionCancelAtPeriodEnd(ctx, gen.SetSubscriptionCancelAtPeriodEndParams{
		ID:                id,
		CancelAtPeriodEnd: flag,
	})
	if err != nil {
		return fmt.Errorf("setting cancel_at_period_end: %w", err)
	}

	return nil
}

func (s *Store) updateSubscriptionPeriodBounds(ctx context.Context, id pgtype.UUID, start, end time.Time, cancelAtEnd bool) error {
	_, err := s.write.UpdateSubscriptionPeriodBounds(ctx, gen.UpdateSubscriptionPeriodBoundsParams{
		ID:                 id,
		CurrentPeriodStart: pgtype.Timestamptz{Time: start, Valid: !start.IsZero()},
		CurrentPeriodEnd:   pgtype.Timestamptz{Time: end, Valid: !end.IsZero()},
		CancelAtPeriodEnd:  cancelAtEnd,
	})
	if err != nil {
		return fmt.Errorf("updating subscription bounds: %w", err)
	}

	return nil
}

func (s *Store) expireSubscription(ctx context.Context, id pgtype.UUID) (gen.Subscription, error) {
	row, err := s.write.ExpireSubscription(ctx, id)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("expiring subscription: %w", err)
	}

	return row, nil
}

func (s *Store) listSubscriptionsDueForExpiry(ctx context.Context, graceCutoff time.Time, limit int32) ([]gen.Subscription, error) {
	rows, err := s.read.ListSubscriptionsDueForExpiry(ctx, gen.ListSubscriptionsDueForExpiryParams{
		GraceCutoff: pgtype.Timestamptz{Time: graceCutoff, Valid: true},
		PageLimit:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions due for expiry: %w", err)
	}

	return rows, nil
}

// getUserEmail resolves a subscriber's email for provider checkout calls.
func (s *Store) getUserEmail(ctx context.Context, userID pgtype.UUID) (string, error) {
	return s.read.GetUserEmail(ctx, userID)
}

// getUserIDByEmail resolves the owner for provider webhooks that carry the
// customer email rather than our metadata (Paystack subscription.create).
func (s *Store) getUserIDByEmail(ctx context.Context, email string) (pgtype.UUID, error) {
	return s.read.GetUserIDByEmail(ctx, email)
}

// getSubscriptionPlanBySlug resolves plan amounts/currency for checkout
// (subscription_plans is the pricing source of truth — Rule 3).
func (s *Store) getSubscriptionPlanBySlug(ctx context.Context, slug string) (gen.SubscriptionPlan, error) {
	return s.read.GetSubscriptionPlanBySlug(ctx, slug)
}

// ---------------------------------------------------------------------------
// Discount Codes
// ---------------------------------------------------------------------------

func (s *Store) createDiscountCode(ctx context.Context, p gen.CreateDiscountCodeParams) (gen.DiscountCode, error) {
	row, err := s.write.CreateDiscountCode(ctx, p)
	if err != nil {
		return gen.DiscountCode{}, fmt.Errorf("creating discount code: %w", err)
	}

	return row, nil
}

func (s *Store) getDiscountCodeByCode(ctx context.Context, code string) (gen.DiscountCode, error) {
	row, err := s.read.GetDiscountCodeByCode(ctx,

		code,
	)
	if err != nil {
		return gen.DiscountCode{}, fmt.Errorf("getting discount code by code: %w", err)
	}

	return row, nil
}

func (s *Store) getDiscountCodeByID(ctx context.Context, id pgtype.UUID) (gen.DiscountCode, error) {
	row, err := s.read.GetDiscountCodeByID(ctx,
		id,
	)
	if err != nil {
		return gen.DiscountCode{}, fmt.Errorf("getting discount code by ID: %w", err)
	}

	return row, nil
}

func (s *Store) listDiscountCodes(ctx context.Context, limit, offset int32) ([]gen.DiscountCode, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListDiscountCodes(ctx, gen.ListDiscountCodesParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing discount codes: %w", err)
	}

	return rows, nil
}

func (s *Store) deleteDiscountCode(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteDiscountCode(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting discount code: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Discount Code Uses
// ---------------------------------------------------------------------------

func (s *Store) insertDiscountCodeUse(ctx context.Context, p gen.InsertDiscountCodeUseParams) (gen.DiscountCodeUse, error) {
	row, err := s.write.InsertDiscountCodeUse(ctx, p)
	if err != nil {
		return gen.DiscountCodeUse{}, fmt.Errorf("inserting discount code use: %w", err)
	}

	return row, nil
}

// consumeDiscountCodeUse atomically consumes one use of a discount code.
// The guarded UPDATE serializes concurrent checkouts on the row lock: the
// counter only increments when max_uses is NULL (unlimited) or still below
// max_uses. Returns pgx.ErrNoRows when the code is exhausted — callers map
// that to "discount code usage limit reached" so max_uses can never be
// overshot, unlike the previous read-check (COUNT) + unguarded insert.
func (s *Store) consumeDiscountCodeUse(ctx context.Context, codeID pgtype.UUID) error {
	if _, err := s.write.ConsumeDiscountCodeUse(ctx, codeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("discount code usage limit reached")
		}

		return fmt.Errorf("consuming discount code use: %w", err)
	}

	return nil
}

// releaseDiscountCodeUse compensates a consumed use when the follow-up
// discount_code_uses insert fails (e.g. duplicate per-user use), so the
// counter stays accurate. Never drops below zero.
func (s *Store) releaseDiscountCodeUse(ctx context.Context, codeID pgtype.UUID) {
	if _, err := s.write.ReleaseDiscountCodeUse(ctx, codeID); err != nil {
		slog.Warn("payments: failed to release discount code use", "code_id", codeID, "error", err)
	}
}

func (s *Store) countDiscountCodeUsesByUser(ctx context.Context, codeID, userID pgtype.UUID) (int32, error) {
	count, err := s.read.CountDiscountCodeUsesByUser(ctx, gen.CountDiscountCodeUsesByUserParams{
		CodeID: codeID,
		UserID: userID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting discount code uses by user: %w", err)
	}

	return count, nil
}

func (s *Store) listDiscountCodeUses(ctx context.Context, codeID pgtype.UUID, limit, offset int32) ([]gen.DiscountCodeUse, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListDiscountCodeUses(ctx, gen.ListDiscountCodeUsesParams{
		CodeID:     codeID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing discount code uses: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Refunds
// ---------------------------------------------------------------------------

func (s *Store) createRefund(ctx context.Context, p gen.CreateRefundParams) (gen.PaymentRefund, error) {
	row, err := s.write.CreateRefund(ctx, p)
	if err != nil {
		return gen.PaymentRefund{}, fmt.Errorf("creating refund: %w", err)
	}

	return row, nil
}

// updateRefund settles a refund row after the provider refund call resolves
// (succeeded with the provider refund reference, failed, or left pending
// for manual review). The refund row stays pending until the provider
// confirms — it is never pre-marked refunded.
func (s *Store) updateRefund(ctx context.Context, id pgtype.UUID, status string, providerRef *string) (gen.PaymentRefund, error) {
	row, err := s.write.UpdateRefund(ctx, gen.UpdateRefundParams{
		Status:      status,
		ProviderRef: providerRef,
		ID:          id,
	})
	if err != nil {
		return gen.PaymentRefund{}, fmt.Errorf("updating refund: %w", err)
	}

	return row, nil
}

func (s *Store) listRefundsByPayment(ctx context.Context, paymentID pgtype.UUID) ([]gen.PaymentRefund, error) {
	rows, err := s.read.ListRefundsByPayment(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("listing refunds by payment: %w", err)
	}

	return rows, nil
}
