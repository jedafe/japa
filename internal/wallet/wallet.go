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
// read global system_settings with fallbacks (AUDIT-BE2-SETTING-007/008).
type SettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64
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
}

// SetSettings injects the global system_settings reader (AUDIT-BE2-SETTING-007/008).
func (s *Service) SetSettings(r SettingsReader) { s.settings = r }

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
