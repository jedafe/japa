// Package wallet implements the Wallet & Credits System (Section 28).
//
// Two-tier internal currency: Points (soft, activity-earned, non-monetary)
// and Credits (spendable, monetary-equivalent). Conversion is one-way only
// (Points → Credits, never reversed), at a fixed but admin-configurable global rate.
package wallet

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Ledger types.
const (
	LedgerTypePoints  = "points"
	LedgerTypeCredits = "credits"
)

// Ledger operations.
const (
	OperationEarn        = "earn"
	OperationSpend       = "spend"
	OperationPurchase    = "purchase"
	OperationConvert     = "convert"
	OperationRefund      = "refund"
	OperationClawback    = "clawback"
	OperationAdminAdjust = "admin_adjust"
)

// Activity keys (examples — actual keys are admin-configurable).
const (
	ActivityKeyPostCreated       = "post_created"
	ActivityKeyCommentCreated    = "comment_created"
	ActivityKeyReactionAdded     = "reaction_added"
	ActivityKeyContentFlagged    = "content_flagged_removed"
	ActivityKeySpamDetected      = "spam_detected"
	ActivityKeyReferralCompleted = "referral_completed"
	ActivityKeyDailyLogin        = "daily_login"
	ActivityKeyProfileCompleted  = "profile_completed"
)

// Default wallet settings.
const (
	DefaultPointsToCreditsRate = 100 // 100 points = 1 credit
	// DefaultMinConvertPoints is the fallback minimum points required for a
	// single conversion when wallet.min_convert_points is unset in
	// system_settings (AUDIT-BE2-SETTING-008).
	DefaultMinConvertPoints = 100
)

// SettingsReader is the subset of the config.Settings API that wallet uses to
// read global system_settings with fallbacks (AUDIT-BE2-SETTING-007/008,
// Section 58 wallet.credit_purchase_enabled + wallet.default_currency).
type SettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback string) string
	GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool
}

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Service provides wallet operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer walletEnqueuer
	// settings is the optional global system_settings reader used for
	// wallet.points_to_credit_rate and wallet.min_convert_points
	// (AUDIT-BE2-SETTING-007/008). When nil, compiled-in defaults are used.
	settings SettingsReader
	// currencyResolver resolves the active fiat currency for a user at
	// checkout time (Section 58, ADR-065). Optional — when nil, the system
	// default (wallet.default_currency) is used.
	currencyResolver *CurrencyResolver
	// paymentsService creates payment rows for credit purchases (Section 17).
	// Optional — when nil, purchaseCredits returns computed pricing without
	// creating a payment row (used in tests).
	paymentsService PaymentCreator
	// cacheInvalidator flushes the jc:wallet:currencies:{tenant_id} cache
	// after any currency CRUD operation (Section 58, Rule 4). Optional —
	// when nil, cache invalidation is skipped (tests).
	cacheInvalidator CacheInvalidator
}

// CacheInvalidator flushes Valkey cache entries for the wallet currency
// system (Section 58). The implementation deletes the
// jc:wallet:currencies:{tenant_id} key after any currency CRUD operation.
type CacheInvalidator interface {
	InvalidateCurrenciesCache(ctx context.Context, tenantID pgtype.UUID) error
}

// PaymentCreator is the subset of the payments.Service API that wallet needs to
// create credit-purchase payment rows (Section 17 + Section 58). Defined as a
// local interface so the wallet package does not import payments.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, tenantID, userID pgtype.UUID, input PaymentCreateInput) (PaymentCreateResult, error)
}

// PaymentCreateInput mirrors payments.CreatePaymentInput (Section 17).
type PaymentCreateInput struct {
	Provider     string
	PurposeType  string
	PurposeID    string
	Amount       int64
	Currency     string
	Metadata     map[string]interface{}
	DiscountCode string
}

// PaymentCreateResult mirrors gen.Payment fields needed by the wallet.
type PaymentCreateResult struct {
	ID           string
	Provider     string
	ProviderRef  string
	CheckoutURL  string
	Status       string
	Amount       int64
	Currency     string
}

// SetSettings injects the global system_settings reader (AUDIT-BE2-SETTING-007/008).
func (s *Service) SetSettings(r SettingsReader) { s.settings = r }

// SetCurrencyResolver injects the currency resolver for geo-aware currency
// resolution at checkout (Section 58, ADR-065).
func (s *Service) SetCurrencyResolver(r *CurrencyResolver) { s.currencyResolver = r }

// SetPaymentsService injects the payments service for creating credit-purchase
// payment rows (Section 17 + Section 58).
func (s *Service) SetPaymentsService(p PaymentCreator) { s.paymentsService = p }

// SetCacheInvalidator injects the Valkey cache invalidator for the currency
// cache (Section 58, Rule 4). Called after any currency CRUD operation.
func (s *Service) SetCacheInvalidator(c CacheInvalidator) { s.cacheInvalidator = c }

// walletEnqueuer is the subset of worker.Enqueuer that wallet needs.
type walletEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
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

// SetEnqueuer injects the Asynq enqueuer for async activity event processing.
func (s *Service) SetEnqueuer(e walletEnqueuer) {
	s.enqueuer = e
}

// Store returns the wallet store. Used by the currency resolver which needs
// the same DB pool for currency lookups (Section 58, ADR-065).
func (s *Service) Store() *Store {
	return s.store
}
