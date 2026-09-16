package payments

import (
	"context"
	"errors"
	"fmt"

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

func (s *Store) listRefundsByPayment(ctx context.Context, paymentID pgtype.UUID) ([]gen.PaymentRefund, error) {
	rows, err := s.read.ListRefundsByPayment(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("listing refunds by payment: %w", err)
	}

	return rows, nil
}
