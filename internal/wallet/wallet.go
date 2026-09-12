// Package wallet implements the Wallet & Credits System (Section 28).
//
// Two-tier internal currency: Points (soft, activity-earned, non-monetary)
// and Credits (spendable, monetary-equivalent). Conversion is one-way only
// (Points → Credits, never reversed), at a fixed but admin-configurable global rate.
package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/wallet/payout"
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
	OperationWithdraw    = "withdraw"
)

// Activity keys — the canonical dot-notation keys written into
// activity_events (Section 28, §64). Rules in activity_point_rules are
// admin-configurable, but they only fire when the key matches what an
// emitter actually produces, so emitters reference these constants rather
// than string literals.
const (
	// Keys emitted by domain services today.
	ActivityKeyContentCreated      = "content.created"
	ActivityKeyCommentCreated      = "comment.created"
	ActivityKeyFollowCreated       = "follow.created"
	ActivityKeySpaceJoined         = "space.joined"
	ActivityKeyAssessmentCompleted = "assessment.completed"
	ActivityKeyScamAlertConfirmed  = "scam_alert.confirmed"
	ActivityKeyAchievementCreated  = "achievement.created"

	// Reserved keys for modules without emitters yet — kept so admins can
	// pre-configure activity_point_rules against stable names.
	ActivityKeyReactionAdded     = "reaction.added"
	ActivityKeyContentFlagged    = "content.flagged_removed"
	ActivityKeySpamDetected      = "spam.detected"
	ActivityKeyReferralCompleted = "referral.completed"
	ActivityKeyDailyLogin        = "daily.login"
	ActivityKeyProfileCompleted  = "profile.completed"
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
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
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
	// payoutProviders holds the configured outbound payout providers for
	// credit withdrawals (Section 58 Credit Withdrawal amendment).
	payoutProviders map[string]payout.Provider
	// cacheInvalidator flushes the jc:wallet:currencies:{tenant_id} cache
	// after any currency CRUD operation (Section 58, Rule 4). Optional —
	// when nil, cache invalidation is skipped (tests).
	cacheInvalidator CacheInvalidator
	// gamificationEvaluator evaluates badges after ledger earns
	// (BUG-GAMMA-001). Optional — nil = skip evaluation.
	gamificationEvaluator GamificationEvaluator
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
	ID          string
	Provider    string
	ProviderRef string
	CheckoutURL string
	Status      string
	Amount      int64
	Currency    string
}

// SetSettings injects the global system_settings reader (AUDIT-BE2-SETTING-007/008).
func (s *Service) SetSettings(r SettingsReader) { s.settings = r }

// SetCurrencyResolver injects the currency resolver for geo-aware currency
// resolution at checkout (Section 58, ADR-065).
func (s *Service) SetCurrencyResolver(r *CurrencyResolver) { s.currencyResolver = r }

// SetPaymentsService injects the payments service for creating credit-purchase
// payment rows (Section 17 + Section 58).
func (s *Service) SetPaymentsService(p PaymentCreator) { s.paymentsService = p }

// SetPayoutProvider registers an outbound payout provider for credit withdrawals.
func (s *Service) SetPayoutProvider(provider payout.Provider) {
	if s.payoutProviders == nil {
		s.payoutProviders = make(map[string]payout.Provider)
	}

	s.payoutProviders[provider.Type()] = provider
}

// SetCacheInvalidator injects the Valkey cache invalidator for the currency
// cache (Section 58, Rule 4). Called after any currency CRUD operation.
func (s *Service) SetCacheInvalidator(c CacheInvalidator) { s.cacheInvalidator = c }

// GamificationEvaluator evaluates badges and challenge progress after a
// wallet ledger earn (BUG-GAMMA-001, ADR-082 §2). Implemented by
// *gamification.Service; nil-safe on the Service struct.
type GamificationEvaluator interface {
	EvaluateForWallet(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) error
}

// SetGamificationEvaluator injects the gamification service for post-earn
// badge evaluation (BUG-GAMMA-001). Optional — when nil, evaluation is
// skipped.
func (s *Service) SetGamificationEvaluator(e GamificationEvaluator) {
	s.gamificationEvaluator = e
}

// ActivityEmitter records a user activity event and enqueues its async
// processing so §64 badge and challenge triggers can fire. Satisfied by
// *Service — declared here so domain services (content, comments, follows,
// spaces, assessment, scamalert) can hold it as a narrow dependency without
// each defining their own adapter interface.
type ActivityEmitter interface {
	EmitActivityEvent(ctx context.Context, tenantID, userID pgtype.UUID, input EmitActivityEventInput) (gen.ActivityEvent, error)
	EnqueueProcessActivityEvent(ctx context.Context, eventID, tenantID pgtype.UUID) error
}

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
		db:              database,
		payoutProviders: make(map[string]payout.Provider),
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

// AwardPoints credits reward_points to a user's wallet by inserting a
// ledger entry of type "points" with operation "earn" (BUG-GAMMA-002).
// Used by the gamification service when a challenge is completed.
//
// Follows the AUDIT-BE-005 pattern: acquires a per-user advisory lock,
// reads the latest balance via the sqlc-generated query, and inserts the
// ledger entry with a deterministic idempotency key so duplicate calls
// (e.g. gamination retries) are safe (Rule 9).
func (s *Service) AwardPoints(ctx context.Context, tenantID, userID pgtype.UUID, points int, reason string) error {
	if points <= 0 {
		return nil
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// Serialize concurrent balance mutations for this user (AUDIT-BE-005).
	lockKey := "wallet:" + tenantID.String() + ":" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("acquiring wallet lock: %w", err)
	}

	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{

		UserID:     userID,
		LedgerType: LedgerTypePoints,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reading current points balance: %w", err)
		}

		currentBalance = 0
	}

	newBalance := currentBalance + int64(points)
	sourceStr := "gamification"
	reasonStr := reason
	idempotencyKey := fmt.Sprintf("gamification_award:%s:%s", userID.String(), reason)

	if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{

		UserID:           userID,
		LedgerType:       LedgerTypePoints,
		Operation:        OperationEarn,
		Amount:           int64(points),
		ResultingBalance: newBalance,
		Source:           &sourceStr,
		Reference:        &reasonStr,
		IdempotencyKey:   &idempotencyKey,
		Reason:           &reasonStr,
	}); err != nil {
		return fmt.Errorf("inserting points ledger entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}
