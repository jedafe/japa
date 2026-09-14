package gamification

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/valkey"
)

// ---------------------------------------------------------------------------
// Public read operations
// ---------------------------------------------------------------------------

// ListBadges returns all active badge definitions for a tenant. Returns
// ErrGamificationDisabled when gamification.badges_enabled is off.
func (s *Service) ListBadges(ctx context.Context) ([]BadgeInfo, error) {
	if !s.badgesEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	badges, err := s.store.ListBadgeDefinitions(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("gamification: list badges: %w", err)
	}

	results := make([]BadgeInfo, 0, len(badges))
	for _, b := range badges {
		results = append(results, badgeToInfo(b))
	}

	return results, nil
}

// ListActiveChallenges returns active challenges within their date range.
// Returns ErrGamificationDisabled when gamification.challenges_enabled is off.
func (s *Service) ListActiveChallenges(ctx context.Context) ([]ChallengeInfo, error) {
	if !s.challengesEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	challenges, err := s.store.ListActiveChallenges(ctx)
	if err != nil {
		return nil, fmt.Errorf("gamification: list active challenges: %w", err)
	}

	results := make([]ChallengeInfo, 0, len(challenges))
	for _, c := range challenges {
		results = append(results, challengeToInfo(c))
	}

	return results, nil
}

// ValidateLeaderboardScope validates a leaderboard scope string. Returns the
// parsed scope type and optional space ID. Valid scopes are "global" or
// "space:{uuid}" — the space segment must be a well-formed UUID.
func ValidateLeaderboardScope(scope string) (scopeType, spaceID string, ok bool) {
	if scope == ScopeGlobal {
		return ScopeGlobal, "", true
	}

	if strings.HasPrefix(scope, ScopeSpace+":") {
		id := strings.TrimPrefix(scope, ScopeSpace+":")
		var parsed pgtype.UUID
		if err := parsed.Scan(id); err != nil || !parsed.Valid {
			return "", "", false
		}

		return ScopeSpace, id, true
	}

	return "", "", false
}

// ValidateLeaderboardPeriod validates a leaderboard period string.
func ValidateLeaderboardPeriod(period string) bool {
	switch period {
	case PeriodAllTime, PeriodMonthly, PeriodWeekly:
		return true
	default:
		return false
	}
}

// leaderboardCacheKey builds the Valkey key for a cached leaderboard.
func leaderboardCacheKey(scope, period string) string {
	return valkey.MustKey("leaderboard:%s:%s", scope, period)
}

// periodStartDate returns the UTC start-of-period time for weekly/monthly.
func periodStartDate(period string) time.Time {
	now := time.Now().UTC()

	switch period {
	case PeriodWeekly:
		return now.AddDate(0, 0, -7)
	case PeriodMonthly:
		return now.AddDate(0, -1, 0)
	default:
		return time.Time{}
	}
}

// GetLeaderboard returns the leaderboard for a scope and period. Results are
// cached in Valkey (jc:leaderboard:{}:{scope}:{period}, 1hr TTL) at
// the full top-N size; the requested limit slices the cached/queried set.
// Scope: "global" or "space:{uuid}". Period: "all_time", "monthly", "weekly".
// Returns ErrGamificationDisabled when gamification.leaderboard_enabled is off.
func (s *Service) GetLeaderboard(ctx context.Context, scope, period string, limit int32) ([]LeaderboardEntry, error) {
	if !s.leaderboardEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	scopeType, spaceIDStr, ok := ValidateLeaderboardScope(scope)
	if !ok {
		return nil, fmt.Errorf("gamification: invalid scope %q", scope)
	}

	if !ValidateLeaderboardPeriod(period) {
		return nil, fmt.Errorf("gamification: invalid period %q", period)
	}

	var spaceID pgtype.UUID
	if scopeType == ScopeSpace {
		if err := spaceID.Scan(spaceIDStr); err != nil {
			return nil, fmt.Errorf("gamification: invalid space scope %q", scope)
		}
	}

	key := leaderboardCacheKey(scope, period)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, key).Result(); err == nil && cached != "" {
			var entries []LeaderboardEntry
			if err := json.Unmarshal([]byte(cached), &entries); err == nil {
				return sliceLeaderboard(entries, limit), nil
			}
		}
	}

	topN := int32(defaultLeaderboardTopN)

	var entries []LeaderboardEntry

	if period == PeriodAllTime {
		rows, err := s.store.LeaderboardAllTime(ctx, spaceID, topN)
		if err != nil {
			return nil, fmt.Errorf("gamification: leaderboard all_time: %w", err)
		}

		entries = make([]LeaderboardEntry, 0, len(rows))
		for i, r := range rows {
			entries = append(entries, LeaderboardEntry{
				UserID:      uuidStr(r.UserID),
				Username:    r.Username,
				DisplayName: ptrStr(r.DisplayName),
				AvatarURL:   r.AvatarUrl,
				Points:      r.LifetimePoints,
				Rank:        i + 1,
			})
		}
	} else {
		fromDate := periodStartDate(period)

		rows, err := s.store.LeaderboardByPeriod(ctx, spaceID, pgtype.Timestamptz{Time: fromDate, Valid: true}, topN)
		if err != nil {
			return nil, fmt.Errorf("gamification: leaderboard by period: %w", err)
		}

		entries = make([]LeaderboardEntry, 0, len(rows))
		for i, r := range rows {
			entries = append(entries, LeaderboardEntry{
				UserID:      uuidStr(r.UserID),
				Username:    r.Username,
				DisplayName: ptrStr(r.DisplayName),
				AvatarURL:   r.AvatarUrl,
				Points:      r.PeriodPoints,
				Rank:        i + 1,
			})
		}
	}

	if s.cache != nil && len(entries) > 0 {
		if data, err := json.Marshal(entries); err == nil {
			if err := s.cache.Set(ctx, key, data, leaderboardCacheTTL).Err(); err != nil {
				slog.Warn("gamification: leaderboard cache set failed", "error", err)
			}
		}
	}

	return sliceLeaderboard(entries, limit), nil
}

// sliceLeaderboard trims entries to the requested limit. A non-positive or
// over-cap limit falls back to the full cached set.
func sliceLeaderboard(entries []LeaderboardEntry, limit int32) []LeaderboardEntry {
	if limit <= 0 || int(limit) >= len(entries) {
		return entries
	}

	return entries[:limit]
}

// ListUserBadges returns a user's earned badges with badge definition
// details. Returns ErrGamificationDisabled when gamification.badges_enabled
// is off.
func (s *Service) ListUserBadges(ctx context.Context, userID pgtype.UUID) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	rows, err := s.store.ListUserBadges(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("gamification: list user badges: %w", err)
	}

	results := make([]BadgeWithDetails, 0, len(rows))

	for _, r := range rows {
		var iconID *string

		if r.BadgeIconMediaID.Valid {
			s := r.BadgeIconMediaID.String()
			iconID = &s
		}

		results = append(results, BadgeWithDetails{
			ID:          uuidStr(r.BadgeID),
			Slug:        r.BadgeSlug,
			Name:        r.BadgeName,
			Description: ptrStr(r.BadgeDescription),
			IconMediaID: iconID,
			AwardedAt:   rfc3339(r.AwardedAt),
		})
	}

	return results, nil
}

// ListUserBadgesByUsername returns a user's earned badges by username — used
// for the public profile badge display. Returns ErrGamificationDisabled when
// gamification.badges_enabled is off.
func (s *Service) ListUserBadgesByUsername(ctx context.Context, username string) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	rows, err := s.store.ListUserBadgesByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("gamification: list user badges by username: %w", err)
	}

	results := make([]BadgeWithDetails, 0, len(rows))
	for _, r := range rows {
		results = append(results, BadgeWithDetails{
			ID:          uuidStr(r.BadgeID),
			Slug:        r.BadgeSlug,
			Name:        r.BadgeName,
			Description: ptrStr(r.BadgeDescription),
			AwardedAt:   rfc3339(r.AwardedAt),
		})
	}

	return results, nil
}

// ListUserChallengeProgress returns a user's challenge progress with
// details. Returns ErrGamificationDisabled when
// gamification.challenges_enabled is off.
func (s *Service) ListUserChallengeProgress(ctx context.Context, userID pgtype.UUID) ([]ChallengeWithProgress, error) {
	if !s.challengesEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	rows, err := s.store.ListUserChallengeProgress(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("gamification: list user challenge progress: %w", err)
	}

	results := make([]ChallengeWithProgress, 0, len(rows))

	for _, r := range rows {
		var completedAt *string

		if r.CompletedAt.Valid {
			ts := rfc3339(r.CompletedAt)
			completedAt = &ts
		}

		var endsAt *string

		if r.EndsAt.Valid {
			ts := rfc3339(r.EndsAt)
			endsAt = &ts
		}

		results = append(results, ChallengeWithProgress{
			ID:           uuidStr(r.ChallengeID),
			Slug:         r.ChallengeSlug,
			Name:         r.ChallengeName,
			Description:  ptrStr(r.ChallengeDescription),
			TargetCount:  int(r.TargetCount),
			CurrentCount: int(r.CurrentCount),
			Completed:    r.Completed,
			CompletedAt:  completedAt,
			EndsAt:       endsAt,
		})
	}

	return results, nil
}
