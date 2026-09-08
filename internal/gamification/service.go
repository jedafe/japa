// Package gamification — service.go holds the business logic for badges,
// challenges, and the leaderboard. It reads gamification.* system_settings
// via the hot-reloadable config.Settings and caches leaderboard results in
// Valkey (jc:leaderboard:{tenant_id}:{scope}:{period}, 1hr TTL).
//
// Badge evaluation (EvaluateBadges / EvaluatePointsBadges) and challenge
// progress incrementing are called from the existing
// wallet:process_activity_event Asynq task — no separate gamification task
// for most operations. ExpireChallenges is called by the daily
// gamification:expire_challenges task.
//
// Ref: japaconnect-engineering/sections/64-gamification.md
//
//	japaconnect-engineering/adr/ADR-082-advanced-gamification.md
package gamification

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
)

// Trigger type constants — match the badge_definitions.trigger_type CHECK.
const (
	TriggerTypeLifetimePoints = "lifetime_points"
	TriggerTypeActivityCount  = "activity_count"
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

// leaderboardCacheTTL is the TTL for jc:leaderboard:{tenant_id}:{scope}:{period}
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
type PointsAwarder interface {
	AwardPoints(ctx context.Context, tenantID, userID pgtype.UUID, points int, reason string) error
}

// Notifier sends user notifications for badge awards and challenge
// completions (BUG-GAMMA-003, ADR-082 §2). Fire-and-forget per Rule 8.
type Notifier interface {
	Notify(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, title, body, actionURL string) error
}

// Service holds the business logic for the gamification layer.
type Service struct {
	store         *Store
	settings      *config.Settings
	cache         valkeycompat.Cmdable // nil = caching disabled
	pointsAwarder PointsAwarder        // nil = no wallet crediting
	notifier      Notifier             // nil = no notifications
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

// EvaluateForWallet is a thin wrapper around EvaluateBadges that satisfies
// the wallet.GamificationEvaluator interface (BUG-GAMMA-001). It discards
// the returned badge list — the wallet only needs the side effects (badge
// awards, notifications). It also re-checks lifetime_points badges on every
// activity — this is the only call site where points can change, so wiring
// EvaluatePointsBadges here is what makes lifetime_points triggers fire.
func (s *Service) EvaluateForWallet(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) error {
	if _, err := s.EvaluateBadges(ctx, tenantID, userID, activityKey); err != nil {
		return err
	}

	// lifetime_points badges — idempotent re-check of thresholds against the
	// current lifetime total (sum of positive points earns, Section 28).
	lifetime, err := s.store.GetUserLifetimePoints(ctx, tenantID, userID)
	if err != nil {
		return fmt.Errorf("gamification: lifetime points lookup: %w", err)
	}

	if _, err := s.EvaluatePointsBadges(ctx, tenantID, userID, lifetime); err != nil {
		return err
	}

	return nil
}
