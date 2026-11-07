// Package gamification — service.go holds the business logic for badges,
// challenges, and the leaderboard. It reads gamification.* system_settings
// via the hot-reloadable config.Settings and caches leaderboard results in
// Valkey (jc:leaderboard:{}:{scope}:{period}, 1hr TTL).
// // Badge evaluation (EvaluateBadges / EvaluatePointsBadges) and challenge
// progress incrementing are called from the existing
// wallet:process_activity_event Asynq task — no separate gamification task
// for most operations. ExpireChallenges is called by the daily
// gamification:expire_challenges task.
// // Ref: japaconnect-engineering/sections/64-gamification.md
// //	japaconnect-engineering/adr/ADR-082-advanced-gamification.md
package gamification

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/features"
)

// Trigger type constants — match the badge_definitions.trigger_type CHECK.
const (
	TriggerTypeLifetimePoints = "lifetime_points"
	TriggerTypeActivityCount  = "activity_count"
	TriggerTypeStreakDays     = "streak_days"
	TriggerTypeManual         = "manual"
)

// Leaderboard scope constants.
const (
	ScopeGlobal = "global"
	// ScopeSpace is the prefix for space-scoped leaderboards: "space:{id}".
	ScopeSpace = "space"
)

// Leaderboard period constants.
const (
	PeriodAllTime = "all_time"
	PeriodMonthly = "monthly"
	PeriodWeekly  = "weekly"
)

// leaderboardCacheTTL is the TTL for jc:leaderboard:{}:{scope}:{period}
// (ADR-082: 1 hour).
const leaderboardCacheTTL = 1 * time.Hour

// defaultLeaderboardTopN is the default number of leaderboard entries returned.
const defaultLeaderboardTopN = 100

// ValkeyCache is the interface for the leaderboard cache. The
// valkeycompat.Cmdable satisfies this.
type ValkeyCache interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
	Set(ctx context.Context, key string, value any, ttl time.Duration) *valkeycompat.StatusCmd
}

// PointsAwarder credits reward points to a user's wallet (BUG-GAMMA-002).
// Implemented by *wallet.Service; nil-safe on the Service struct.
// EnqueuePointsAward routes through the retryable wallet:award_points
// Asynq task (PTS-006) so a transient failure cannot permanently lose a
// challenge reward.
type PointsAwarder interface {
	EnqueuePointsAward(ctx context.Context, userID pgtype.UUID, points int, reason, referenceID string) error
}

// Notifier sends user notifications for badge awards and challenge
// completions (BUG-GAMMA-003, ADR-082 §2). Fire-and-forget per Rule 8.
type Notifier interface {
	Notify(ctx context.Context, recipientID pgtype.UUID, nType, title, body, actionURL string) error
}

// ActivityEmitter records a wallet activity event and enqueues its
// processing. Implemented by *wallet.Service; used to emit
// challenge.completed events so challenge-count badges can trigger.
// Nil-safe — when unset the emission is skipped.
type ActivityEmitter interface {
	EmitActivityEvent(ctx context.Context, userID pgtype.UUID, input EmitActivityInput) (EmitActivityEvent, error)
	EnqueueProcessActivityEvent(ctx context.Context, eventID pgtype.UUID) error
}

// EmitActivityInput mirrors wallet.EmitActivityEventInput. Declared here to
// keep the gamification package decoupled from wallet (matching the
// achievement emitter adapter pattern in server.go).
type EmitActivityInput struct {
	ActivityKey string
	Reference   string
	Payload     map[string]interface{}
}

// EmitActivityEvent mirrors gen.ActivityEvent for the emitter contract.
type EmitActivityEvent struct {
	ID pgtype.UUID
}

// MediaResolver resolves a badge icon_media_id to a displayable URL — the
// same interface shape as users.MediaResolver / verification.MediaResolver
// (mediaURLResolver in server.go).
type MediaResolver interface {
	ResolveMediaURL(ctx context.Context, mediaID pgtype.UUID) (string, error)
	// ResolveMediaURLs resolves a batch of ids in one round-trip — the
	// list-endpoint counterpart of ResolveMediaURL.
	ResolveMediaURLs(ctx context.Context, mediaIDs []pgtype.UUID) (map[string]string, error)
}

// MediaImageProcessor applies artwork transforms to an attached media
// object (edge-background knockout + downscale to fit its frame).
// Implemented by *media.Service.
type MediaImageProcessor interface {
	RemoveEdgeBackground(ctx context.Context, mediaID pgtype.UUID, maxDim int) error
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// implemented by *features.Checker (§42). Used to hide badge/challenge
// definitions linked to a disabled feature (badge_definitions.feature_flag,
// challenge_definitions.feature_flag — migration 001086). EvalFlag is used
// for public reads so surface/geo/audience rules apply, not just the global
// toggle (same contract as menu_items.feature_flag); IsEnabled is the right
// semantics for background evaluation, which has no caller scope.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
	EvalFlag(ctx context.Context, key string, sc features.CallerScope) features.EvalResult
}

// Service holds the business logic for the gamification layer.
type Service struct {
	store           *Store
	settings        *config.Settings
	cache           valkeycompat.Cmdable // nil = caching disabled
	pointsAwarder   PointsAwarder        // nil = no wallet crediting
	notifier        Notifier             // nil = no notifications
	activityEmitter ActivityEmitter      // nil = no challenge.completed events
	mediaResolver   MediaResolver        // nil = icon_url omitted from responses
	mediaProcessor  MediaImageProcessor  // nil = no artwork background knockout
	checker         FeatureChecker       // nil = flagged items hidden (fail closed)
}

// NewService creates a gamification service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetCache injects the Valkey client for leaderboard caching.
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// SetPointsAwarder injects the wallet service for reward-point crediting
// (BUG-GAMMA-002). Optional — when nil, challenge completion skips crediting.
func (s *Service) SetPointsAwarder(p PointsAwarder) { s.pointsAwarder = p }

// SetNotifier injects the notification adapter for badge/challenge
// notifications (BUG-GAMMA-003). Optional — when nil, notifications are
// skipped.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SetActivityEmitter injects the wallet service so challenge completions
// can emit challenge.completed activity events (which themselves feed
// activity_count badges like Challenge Hunter). Optional.
func (s *Service) SetActivityEmitter(e ActivityEmitter) { s.activityEmitter = e }

// SetMediaResolver injects the media URL resolver so badge artwork
// (icon_media_id) serializes as a displayable icon_url. Optional — when nil
// the field is omitted from responses.
func (s *Service) SetMediaResolver(r MediaResolver) { s.mediaResolver = r }

// SetMediaImageProcessor injects the media service so newly attached
// badge/challenge artwork gets its solid edge background knocked out.
// Optional — when nil, attachments keep the original pixels.
func (s *Service) SetMediaImageProcessor(p MediaImageProcessor) { s.mediaProcessor = p }

// SetFeatureChecker injects the feature flag checker (§42) so badge and
// challenge definitions linked to a feature flag hide while that flag is
// disabled. When unset, flagged items stay hidden — fail closed, matching
// the menu_items.feature_flag contract.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// CallerScope builds the request-scoped flag scope (validated X-JC-Surface,
// geo context, RBAC audience) for feature-flagged item filtering. Returns a
// zero scope — global flag behavior — when the checker can't build scopes.
// Same pattern as menus.Service.CallerScope.
func (s *Service) CallerScope(r *http.Request) features.CallerScope {
	return features.ScopeForRequest(s.checker, r)
}

// emitActivity records a points activity event — fire-and-forget (Rule 8).
// Nil-safe: no-op when no emitter is wired.
func (s *Service) emitActivity(ctx context.Context, userID pgtype.UUID, key, reference string, payload map[string]interface{}) {
	if s.activityEmitter == nil {
		return
	}

	event, err := s.activityEmitter.EmitActivityEvent(ctx, userID, EmitActivityInput{
		ActivityKey: key,
		Reference:   reference,
		Payload:     payload,
	})
	if err != nil {
		slog.Warn("gamification: emit activity event failed", "activity_key", key, "error", err)
		return
	}

	if err := s.activityEmitter.EnqueueProcessActivityEvent(ctx, event.ID); err != nil {
		slog.Warn("gamification: enqueue activity event failed", "activity_key", key, "error", err)
	}
}

// EvaluateForWallet is a thin wrapper around EvaluateBadges that satisfies
// the wallet.GamificationEvaluator interface (BUG-GAMMA-001). It discards
// the returned badge list — the wallet only needs the side effects (badge
// awards, notifications). It also re-checks lifetime_points badges on every
// activity — this is the only call site where points can change, so wiring
// EvaluatePointsBadges here is what makes lifetime_points triggers fire.
func (s *Service) EvaluateForWallet(ctx context.Context, userID pgtype.UUID, activityKey string) error {
	if _, err := s.EvaluateBadges(ctx, userID, activityKey); err != nil {
		return err
	}

	// lifetime_points badges — idempotent re-check of thresholds against the
	// current lifetime total (sum of positive points earns, Section 28).
	lifetime, err := s.store.GetUserLifetimePoints(ctx, userID)
	if err != nil {
		return fmt.Errorf("gamification: lifetime points lookup: %w", err)
	}

	if _, err := s.EvaluatePointsBadges(ctx, userID, lifetime); err != nil {
		return err
	}

	return nil
}
