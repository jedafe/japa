package payments

import (
	"context"
	"fmt"

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

//nolint:unused // reserved for future features
func (s *Store) createPayment(ctx context.Context, p gen.CreatePaymentParams) (gen.Payment, error) {
	row, err := s.write.CreatePayment(ctx, p)
	if err != nil {
		return gen.Payment{}, fmt.Errorf("creating payment: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getPaymentByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.Payment, error) {
	row, err := s.read.GetPaymentByID(ctx, gen.GetPaymentByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.Payment{}, fmt.Errorf("getting payment by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listPaymentsByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.Payment, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPaymentsByUser(ctx, gen.ListPaymentsByUserParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing payments by user: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) listPaymentsByStatus(ctx context.Context, tenantID pgtype.UUID, status string, limit, offset int32) ([]gen.Payment, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPaymentsByStatus(ctx, gen.ListPaymentsByStatusParams{
		TenantID:   tenantID,
		Status:     status,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing payments by status: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) updatePaymentStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (gen.Payment, error) {
	row, err := s.write.UpdatePaymentStatus(ctx, gen.UpdatePaymentStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return gen.Payment{}, fmt.Errorf("updating payment status: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Webhook Events
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) insertWebhookEvent(ctx context.Context, p gen.InsertWebhookEventParams) (gen.PaymentWebhookEvent, error) {
	row, err := s.write.InsertWebhookEvent(ctx, p)
	if err != nil {
		return gen.PaymentWebhookEvent{}, fmt.Errorf("inserting webhook event: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) isWebhookEventProcessed(ctx context.Context, tenantID pgtype.UUID, provider, eventID string) (bool, error) {
	processed, err := s.read.IsWebhookEventProcessed(ctx, gen.IsWebhookEventProcessedParams{
		TenantID: tenantID,
		Provider: provider,
		EventID:  eventID,
	})
	if err != nil {
		return false, fmt.Errorf("checking webhook event processed: %w", err)
	}

	return processed, nil
}

//nolint:unused // reserved for future features
func (s *Store) markWebhookEventProcessed(ctx context.Context, tenantID pgtype.UUID, provider, eventID string) error {
	if err := s.write.MarkWebhookEventProcessed(ctx, gen.MarkWebhookEventProcessedParams{
		TenantID: tenantID,
		Provider: provider,
		EventID:  eventID,
	}); err != nil {
		return fmt.Errorf("marking webhook event processed: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Subscriptions
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createSubscription(ctx context.Context, p gen.CreateSubscriptionParams) (gen.Subscription, error) {
	row, err := s.write.CreateSubscription(ctx, p)
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("creating subscription: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getSubscriptionByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.Subscription, error) {
	row, err := s.read.GetSubscriptionByID(ctx, gen.GetSubscriptionByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("getting subscription by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getActiveSubscriptionByUser(ctx context.Context, tenantID, userID pgtype.UUID) (gen.Subscription, error) {
	row, err := s.read.GetActiveSubscriptionByUser(ctx, gen.GetActiveSubscriptionByUserParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("getting active subscription by user: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listSubscriptionsByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.Subscription, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListSubscriptionsByUser(ctx, gen.ListSubscriptionsByUserParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions by user: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) listActiveSubscriptions(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.Subscription, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListActiveSubscriptions(ctx, gen.ListActiveSubscriptionsParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing active subscriptions: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateSubscriptionStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (gen.Subscription, error) {
	row, err := s.write.UpdateSubscriptionStatus(ctx, gen.UpdateSubscriptionStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("updating subscription status: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) cancelSubscription(ctx context.Context, id, tenantID pgtype.UUID) (gen.Subscription, error) {
	row, err := s.write.CancelSubscription(ctx, gen.CancelSubscriptionParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("canceling subscription: %w", err)
	}

	return row, nil
}

// reactivateSubscription reactivates the caller's most recently canceled
// subscription for the tenant. The subquery in the sqlc query already scopes
// by (user_id, tenant_id, status='canceled'), so we only forward the params.
func (s *Store) reactivateSubscription(ctx context.Context, userID, tenantID pgtype.UUID) (gen.Subscription, error) {
	row, err := s.write.ReactivateSubscription(ctx, gen.ReactivateSubscriptionParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.Subscription{}, fmt.Errorf("reactivating subscription: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Discount Codes
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createDiscountCode(ctx context.Context, p gen.CreateDiscountCodeParams) (gen.DiscountCode, error) {
	row, err := s.write.CreateDiscountCode(ctx, p)
	if err != nil {
		return gen.DiscountCode{}, fmt.Errorf("creating discount code: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getDiscountCodeByCode(ctx context.Context, tenantID pgtype.UUID, code string) (gen.DiscountCode, error) {
	row, err := s.read.GetDiscountCodeByCode(ctx, gen.GetDiscountCodeByCodeParams{
		TenantID: tenantID,
		Code:     code,
	})
	if err != nil {
		return gen.DiscountCode{}, fmt.Errorf("getting discount code by code: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getDiscountCodeByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.DiscountCode, error) {
	row, err := s.read.GetDiscountCodeByID(ctx, gen.GetDiscountCodeByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.DiscountCode{}, fmt.Errorf("getting discount code by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listDiscountCodes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.DiscountCode, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListDiscountCodes(ctx, gen.ListDiscountCodesParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing discount codes: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteDiscountCode(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteDiscountCode(ctx, gen.DeleteDiscountCodeParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting discount code: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Discount Code Uses
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) insertDiscountCodeUse(ctx context.Context, p gen.InsertDiscountCodeUseParams) (gen.DiscountCodeUse, error) {
	row, err := s.write.InsertDiscountCodeUse(ctx, p)
	if err != nil {
		return gen.DiscountCodeUse{}, fmt.Errorf("inserting discount code use: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
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

//nolint:unused // reserved for future features
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

//nolint:unused // reserved for future features
func (s *Store) createRefund(ctx context.Context, p gen.CreateRefundParams) (gen.PaymentRefund, error) {
	row, err := s.write.CreateRefund(ctx, p)
	if err != nil {
		return gen.PaymentRefund{}, fmt.Errorf("creating refund: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listRefundsByPayment(ctx context.Context, paymentID pgtype.UUID) ([]gen.PaymentRefund, error) {
	rows, err := s.read.ListRefundsByPayment(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("listing refunds by payment: %w", err)
	}

	return rows, nil
}
