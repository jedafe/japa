// Package wallet implements the Wallet & Credits System (Section 28).
// // Two independent internal currencies (ADR-136): Points (activity-earned
// reputation — leaderboard rank, badge thresholds, profile boost; also
// deducted by negative-activity rules) and Credits (spendable,
// monetary-equivalent). No conversion exists in either direction.
package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/wallet/payout"
	"github.com/jedafe/japa/internal/worker"
)

// EmittedActivityKeys is the canonical catalog of activity_keys the
// platform emits — the admin rules registry validates against this list so
// a rule for a key nothing emits is never silently inert.
var EmittedActivityKeys = []string{
	ActivityKeyUserRegistered, ActivityKeyDailyLogin, ActivityKeyProfileCompleted,
	ActivityKeyReferralCompleted, ActivityKeyContentCreated, ActivityKeyCommentCreated,
	ActivityKeyReactionAdded, ActivityKeyReactionNegative, ActivityKeyReactionReverted,
	ActivityKeyReactionNegativeReverted, ActivityKeyFollowCreated, ActivityKeySpaceJoined,
	ActivityKeyAssessmentCompleted, ActivityKeyAcademyLessonCompleted,
	ActivityKeyAcademyTrackCompleted, ActivityKeyForumSolutionMarked, ActivityKeyTipSent,
	ActivityKeySpamDetected, ActivityKeySpamFlagged, ActivityKeyContentFlagged,
	ActivityKeyReportUpheld, ActivityKeyScamAlertConfirmed, ActivityKeyAchievementCreated,
	// Gamification v2 — catalogue emitters.
	ActivityKeyForumThreadCreated, ActivityKeyForumPollVoted, ActivityKeyForumPollCreated,
	ActivityKeyJobSaved, ActivityKeyScholarshipSaved, ActivityKeyServiceSaved,
	ActivityKeyComparisonCompleted, ActivityKeyWikiPageViewed,
	ActivityKeyWikiCountryViewed, ActivityKeyWikiCityViewed, ActivityKeyContentRead,
	ActivityKeyMentorshipRequested, ActivityKeyMentorshipConnected,
	ActivityKeyMentorshipCompleted, ActivityKeyMentorshipMentorCompleted,
	ActivityKeyChallengeCompleted,
	ActivityKeyJourneyMilestoneApplication, ActivityKeyJourneyMilestoneDecision,
	ActivityKeyJourneyMilestoneVisaApproved, ActivityKeyJourneyMilestoneJobOffer,
	ActivityKeyJourneyMilestoneUniAccepted, ActivityKeyJourneyMilestoneArrived,
	ActivityKeyJourneyMilestoneOneYear, ActivityKeyJourneyMilestoneCitizenship,
	ActivityKeyJourneyMilestoneOther,
	ActivityKeyVisaTimelineMilestone,
	ActivityKeyProfilePhotoAdded, ActivityKeyProfileBioAdded, ActivityKeyProfileDestinationSet,
	ActivityKeyEmailVerified, ActivityKeyIdentityVerified, ActivityKeySpaceCreated,
	ActivityKeyScamReportSubmitted, ActivityKeyChecklistItemChecked,
	ActivityKeyServiceReviewSubmitted, ActivityKeyEventRSVP,
	ActivityKeyAIConversationStarted, ActivityKeyReferralCreated,
	ActivityKeyScholarshipAppStarted, ActivityKeyScholarshipAppSubmitted,
	ActivityKeyAcademyQuizPassed, ActivityKeyAcademyQuizPerfect,
}

// emittedActivityKeyPrefixes are dynamic key families emitted by route-map
// middleware whose full enumeration lives in the emitting config rather
// than this list: tool.<slug>.used (ToolUsageRoutes slugs).
var emittedActivityKeyPrefixes = []string{
	"tool.",
}

// IsEmittedActivityKey reports whether a key is one the platform can emit —
// an exact catalog match or a member of a dynamic prefix family (tool.*).
// The activity-rules registry uses this to reject rules that can never fire
// (PTS-019).
func IsEmittedActivityKey(key string) bool {
	for _, k := range EmittedActivityKeys {
		if k == key {
			return true
		}
	}
	for _, p := range emittedActivityKeyPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

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

	// Emitted by domain services across the site (ADR-136 — full activity
	// coverage; negative point_value rules make the *_negative/spam/report
	// keys penalties).
	ActivityKeyUserRegistered         = "user.registered"
	ActivityKeyDailyLogin             = "daily.login"
	ActivityKeyProfileCompleted       = "profile.completed"
	ActivityKeyReferralCompleted      = "referral.completed"
	ActivityKeyReactionAdded          = "reaction.added"
	ActivityKeyAcademyLessonCompleted = "academy.lesson_completed"
	ActivityKeyAcademyTrackCompleted  = "academy.track_completed"
	ActivityKeyForumSolutionMarked    = "forum.solution_marked"
	ActivityKeyTipSent                = "tip.sent"
	ActivityKeyReactionNegative       = "reaction.negative"
	ActivityKeySpamDetected           = "spam.detected"
	ActivityKeyContentFlagged         = "content.flagged_removed"
	ActivityKeyReportUpheld           = "report.upheld"
	// ADR-136: revert keys undo a prior vote award when the vote flips or
	// is removed — keep magnitudes mirrored with the direction they undo.
	ActivityKeyReactionReverted         = "reaction.reverted"
	ActivityKeyReactionNegativeReverted = "reaction.negative.reverted"
	ActivityKeySpamFlagged              = "spam.flagged"

	// Gamification v2 — catalogue emitters (migration 001016).
	ActivityKeyForumThreadCreated           = "forum.thread_created"
	ActivityKeyForumPollVoted               = "forum.poll_voted"
	ActivityKeyJobSaved                     = "job.saved"
	ActivityKeyScholarshipSaved             = "scholarship.saved"
	ActivityKeyServiceSaved                 = "service.saved"
	ActivityKeyComparisonCompleted          = "comparison.completed"
	ActivityKeyWikiPageViewed               = "wiki.page_viewed"
	ActivityKeyWikiCountryViewed            = "wiki.country_viewed"
	ActivityKeyWikiCityViewed               = "wiki.city_viewed"
	ActivityKeyContentRead                  = "content.read"
	ActivityKeyMentorshipRequested          = "mentorship.requested"
	ActivityKeyMentorshipConnected          = "mentorship.connected"
	ActivityKeyMentorshipCompleted          = "mentorship.completed"
	ActivityKeyMentorshipMentorCompleted    = "mentorship.mentor_completed"
	ActivityKeyChallengeCompleted           = "challenge.completed"
	ActivityKeyJourneyMilestoneApplication  = "journey.milestone.application_submitted"
	ActivityKeyJourneyMilestoneDecision     = "journey.milestone.decision_received"
	ActivityKeyJourneyMilestoneVisaApproved = "journey.milestone.visa_approved"
	ActivityKeyJourneyMilestoneJobOffer     = "journey.milestone.job_offer"
	ActivityKeyJourneyMilestoneUniAccepted  = "journey.milestone.university_accepted"
	ActivityKeyJourneyMilestoneArrived      = "journey.milestone.arrived"
	ActivityKeyJourneyMilestoneOneYear      = "journey.milestone.one_year"
	ActivityKeyJourneyMilestoneCitizenship  = "journey.milestone.citizenship"
	ActivityKeyVisaTimelineMilestone        = "visa_timeline.milestone"
	ActivityKeyProfilePhotoAdded            = "profile.photo_added"
	ActivityKeyProfileBioAdded              = "profile.bio_added"
	ActivityKeyProfileDestinationSet        = "profile.destination_set"
	ActivityKeyEmailVerified                = "email.verified"
	ActivityKeyIdentityVerified             = "identity.verified"
	ActivityKeySpaceCreated                 = "space.created"
	ActivityKeyScamReportSubmitted          = "scam_report.submitted"
	ActivityKeyChecklistItemChecked         = "checklist.item_checked"
	ActivityKeyServiceReviewSubmitted       = "service.review_submitted"
	ActivityKeyEventRSVP                    = "event.rsvp"
	ActivityKeyAIConversationStarted        = "ai.conversation_started"
	ActivityKeyReferralCreated              = "referral.created"
	ActivityKeyScholarshipAppStarted        = "scholarship.application_started"
	ActivityKeyScholarshipAppSubmitted      = "scholarship.application_submitted"

	// Gamification v2 additions — emitted but previously uncatalogued
	// (PTS-019): quiz badges, poll creation, the 'other' journey milestone.
	ActivityKeyForumPollCreated      = "forum.poll_created"
	ActivityKeyAcademyQuizPassed     = "academy.quiz_passed"
	ActivityKeyAcademyQuizPerfect    = "academy.quiz_perfect"
	ActivityKeyJourneyMilestoneOther = "journey.milestone.other"
)

// SettingsReader is the subset of the config.Settings API that wallet uses to
// read global system_settings with fallbacks (AUDIT-BE2-SETTING-007/008,
// Section 58 wallet.credit_purchase_enabled + wallet.default_currency).
type SettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetFloatOr(ctx context.Context, key string, fallback float64) float64
	GetStringOr(ctx context.Context, key, fallback string) string
	GetBoolOr(ctx context.Context, key string, fallback bool) bool
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
	// wallet.max_daily_earn_points and wallet.default_currency
	// (AUDIT-BE2-SETTING-007/008). When nil, compiled-in defaults are used.
	settings SettingsReader
	// tierResolver resolves the member's tier for the tier-scoped daily earn
	// cap (wallet.max_daily_earn_points.{regular,pro,business}). Optional —
	// when nil, the global cap applies to everyone.
	tierResolver TierResolver
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
	// cacheInvalidator flushes the jc:wallet:currencies:{} cache
	// after any currency CRUD operation (Section 58, Rule 4). Optional —
	// when nil, cache invalidation is skipped (tests).
	cacheInvalidator CacheInvalidator
	// gamificationEvaluator evaluates badges after ledger earns
	// (BUG-GAMMA-001). Optional — nil = skip evaluation.
	gamificationEvaluator GamificationEvaluator
	// payoutCallbackURL is the public endpoint providers POST signed payout
	// status callbacks to (SEC-W-008). Optional — empty means no IPN
	// callback URL is sent and status relies on reconcile polling.
	payoutCallbackURL string

	// SEC-W-015: validates scoped "step_up" tokens proving a fresh TOTP or
	// password check (POST /auth/step-up). Required for withdrawals and
	// payout-method creation when wallet.require_step_up is enabled.
	stepUpVerifier func(ctx context.Context, userID pgtype.UUID, token string) error
}

// CacheInvalidator flushes Valkey cache entries for the wallet currency
// system (Section 58). The implementation deletes the
// jc:wallet:currencies:{} key after any currency CRUD operation.
type CacheInvalidator interface {
	InvalidateCurrenciesCache(ctx context.Context) error
}

// PaymentCreator is the subset of the payments.Service API that wallet needs to
// create credit-purchase payment rows (Section 17 + Section 58). Defined as a
// local interface so the wallet package does not import payments.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, userID pgtype.UUID, input PaymentCreateInput) (PaymentCreateResult, error)
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

// TierResolver resolves a user's membership tier — satisfied by
// *tiers.Service. Optional (ADR-136: tier-scoped daily earn caps).
type TierResolver interface {
	ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error)
}

// SetTierResolver injects the tier resolver for tier-scoped earn caps.
func (s *Service) SetTierResolver(t TierResolver) { s.tierResolver = t }

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

// SetPayoutCallbackURL sets the public URL sent to payout providers as their
// signed status-callback target (SEC-W-008).
func (s *Service) SetPayoutCallbackURL(url string) { s.payoutCallbackURL = url }

// GamificationEvaluator evaluates badges and challenge progress after a
// wallet ledger earn (BUG-GAMMA-001, ADR-082 §2). Implemented by
// *gamification.Service; nil-safe on the Service struct.
type GamificationEvaluator interface {
	EvaluateForWallet(ctx context.Context, userID pgtype.UUID, activityKey string) error
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
	EmitActivityEvent(ctx context.Context, userID pgtype.UUID, input EmitActivityEventInput) (gen.ActivityEvent, error)
	EnqueueProcessActivityEvent(ctx context.Context, eventID pgtype.UUID) error
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
// Routes through creditWallet so the per-user advisory lock, balance read,
// deterministic idempotency key, and the earn gate apply — exemptCap=true
// (PTS-026): rewards are committed payouts, exempt from the daily cap but
// still subject to the EnableEarning killswitch.
func (s *Service) AwardPoints(ctx context.Context, userID pgtype.UUID, points int, reason string) error {
	if points <= 0 {
		return nil
	}

	idempotencyKey := fmt.Sprintf("gamification_award:%s:%s", userID.String(), reason)

	return s.creditWallet(ctx, userID, LedgerTypePoints, OperationEarn,
		int64(points), "gamification", reason, idempotencyKey, true)
}

// EnqueuePointsAward records a points award via the retryable
// wallet:award_points Asynq task (PTS-006) — durable against transient
// failures unlike a synchronous insert. Falls back to synchronous
// processing when the enqueuer is unavailable so an award can never strand.
func (s *Service) EnqueuePointsAward(ctx context.Context, userID pgtype.UUID, points int, reason, referenceID string) error {
	payload := AwardPointsPayload{
		UserID:      userID.String(),
		Points:      int64(points),
		LedgerType:  LedgerTypePoints,
		Reason:      reason,
		ReferenceID: referenceID,
	}
	if referenceID == "" {
		// Stamped once at enqueue — stable across Asynq retries, unique per
		// logical award so same-reason same-amount grants don't collapse.
		payload.Nonce = uuid.NewString()
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling award points payload: %w", err)
	}

	if s.enqueuer != nil {
		if err := s.enqueuer.Enqueue(worker.TypeWalletAwardPoints, b); err == nil {
			return nil
		} else {
			slog.Warn("wallet: award points enqueue failed, processing inline",
				"user_id", userID.String(), "reason", reason, "error", err)
		}
	}

	return s.AwardPointsTask(ctx, b)
}

// SetStepUpVerifier wires the scoped step-up token validator (auth's
// ValidateStepUpToken adapted to compare claims.UserID) — SEC-W-015.
func (s *Service) SetStepUpVerifier(fn func(ctx context.Context, userID pgtype.UUID, token string) error) {
	s.stepUpVerifier = fn
}
