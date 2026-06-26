// Package payments implements the Payment Core system (Section 17).
//
// Handles payment processing, subscriptions, discount codes, and refunds.
// Supports multiple payment providers (Paystack, Stripe, NOWPayments, manual transfer)
// with a provider abstraction layer for extensibility.
package payments

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Payment providers.
const (
	ProviderPaystack         = "paystack"
	ProviderStripe           = "stripe"
	ProviderNOWPayments      = "nowpayments"
	ProviderManualTransfer   = "manual_transfer"
	ProviderCredit           = "credit"
)

// Payment purpose types.
const (
	PurposeProMembership = "pro_membership"
	PurposeOrgBoost      = "org_boost"
	PurposeDonation      = "donation"
	PurposeCourseAccess  = "course_access"
	PurposeCreditPurchase = "credit_purchase"
	PurposeAdSlot         = "ad_slot"
)

// Payment statuses.
const (
	PaymentStatusPending   = "pending"
	PaymentStatusProcessing = "processing"
	PaymentStatusSucceeded = "succeeded"
	PaymentStatusFailed    = "failed"
	PaymentStatusRefunded  = "refunded"
	PaymentStatusDisputed  = "disputed"
)

// Subscription statuses.
const (
	SubscriptionStatusActive  = "active"
	SubscriptionStatusCanceled = "canceled"
	SubscriptionStatusPastDue = "past_due"
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
	store  *Store
	db     *db.DB
	wallet CreditSpender
}

// NewService creates a Service backed by the given database pool.
// The wallet parameter is optional — pass nil if credit payments are not in use.
func NewService(database *db.DB, wallet CreditSpender) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:     database,
		wallet: wallet,
	}
}