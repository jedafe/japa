// Package wallet implements the Wallet & Credits System (Section 28).
//
// Two-tier internal currency: Points (soft, activity-earned, non-monetary)
// and Credits (spendable, monetary-equivalent). Conversion is one-way only
// (Points → Credits, never reversed), at a fixed but admin-configurable global rate.
package wallet

import (
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
	OperationEarn         = "earn"
	OperationSpend        = "spend"
	OperationPurchase     = "purchase"
	OperationConvert      = "convert"
	OperationRefund       = "refund"
	OperationClawback     = "clawback"
	OperationAdminAdjust  = "admin_adjust"
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
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Service provides wallet operations with reader/writer pool separation.
type Service struct {
	store *Store
	db    *db.DB
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