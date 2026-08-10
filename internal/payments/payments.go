// Package payments implements the Payment Core system (Section 17).
//
// Handles payment processing, subscriptions, discount codes, and refunds.
// Supports multiple payment providers (Paystack, Stripe, NOWPayments, manual transfer)
// with a provider abstraction layer for extensibility.
package payments

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Payment providers.
const (
	ProviderPaystack       = "paystack"
	ProviderStripe         = "stripe"
	ProviderNOWPayments    = "nowpayments"
	ProviderManualTransfer = "manual_transfer"
	ProviderCredit         = "credit"
)

// Payment purpose types.
const (
	PurposeProMembership  = "pro_membership"
	PurposeOrgBoost       = "org_boost"
	PurposeDonation       = "donation"
	PurposeCourseAccess   = "course_access"
	PurposeCreditPurchase = "credit_purchase"
	PurposeAdSlot         = "ad_slot"
)

// Payment statuses.
const (
	PaymentStatusPending    = "pending"
	PaymentStatusProcessing = "processing"
	PaymentStatusSucceeded  = "succeeded"
	PaymentStatusFailed     = "failed"
	PaymentStatusRefunded   = "refunded"
	PaymentStatusDisputed   = "disputed"
)

// Subscription statuses.
const (
	SubscriptionStatusActive   = "active"
	SubscriptionStatusCanceled = "canceled"
	SubscriptionStatusPastDue  = "past_due"
	SubscriptionStatusTrialing = "trialing"
)

// Subscription plans.
const (
	PlanProMonthly = "pro_monthly"
	PlanProYearly  = "pro_yearly"
)

// Refund statuses.
const (
	RefundStatusPending   = "pending"
	RefundStatusSucceeded = "succeeded"
	RefundStatusFailed    = "failed"
)

// Discount types.
const (
	DiscountTypePercent = "percent"
	DiscountTypeFixed   = "fixed"
)

// Discount applicability.
const (
	ApplicableToCourseAccess  = "course_access"
	ApplicableToProMembership = "pro_membership"
	ApplicableToAll           = "all"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Service provides payment operations with reader/writer pool separation.
type Service struct {
	store     *Store
	db        *db.DB
	wallet    CreditSpender
	enqueuer  paymentsEnqueuer
	providers map[string]Provider
	settings  paymentSettingsReader
	webhooks  webhookEmitter
}

// webhookEmitter fans a payment.* event out to all subscribed webhook
// endpoints for the tenant (fire-and-forget). Ref: Section 12 (webhooks).
type webhookEmitter interface {
	EmitEvent(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any)
}

// paymentSettingsReader is the subset of config.Settings the payments service
// uses for hot-reloadable defaults (currency, etc.). Defined as a local
// interface so the payments package does not import config.
type paymentSettingsReader interface {
	GetSetting(ctx context.Context, tenantID pgtype.UUID, key string) (string, error)
}

// paymentsEnqueuer is the subset of worker.Enqueuer that payments needs.
type paymentsEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// NewService creates a Service backed by the given database pool.
// The wallet parameter is optional — pass nil if credit payments are not in use.
func NewService(database *db.DB, wallet CreditSpender) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:        database,
		wallet:    wallet,
		providers: make(map[string]Provider),
	}
}

// SetEnqueuer injects the Asynq enqueuer for async webhook processing.
func (s *Service) SetEnqueuer(e paymentsEnqueuer) {
	s.enqueuer = e
}

// SetSettings injects the hot-reloadable settings reader for payment defaults.
func (s *Service) SetSettings(sr paymentSettingsReader) {
	s.settings = sr
}

// SetWebhookEmitter injects the payment.* event fan-out (Section 12).
func (s *Service) SetWebhookEmitter(w webhookEmitter) { s.webhooks = w }

// emitWebhook fires a payment.* event fire-and-forget (Rule 8). Errors are
// logged but never returned to the caller — webhook delivery is async.
func (s *Service) emitWebhook(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any) {
	if s.webhooks == nil {
		return
	}
	s.webhooks.EmitEvent(ctx, tenantID, eventType, payload)
}

// SetProvider registers a payment provider by name.
func (s *Service) SetProvider(p Provider) {
	s.providers[p.Name()] = p
}
