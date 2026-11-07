package gamification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/valkey"
)

// ---------------------------------------------------------------------------
// Public read operations
// ---------------------------------------------------------------------------

// ListBadges returns all active badge definitions for a platform. Returns
// ErrGamificationDisabled when gamification.badges_enabled is off.
func (s *Service) ListBadges(ctx context.Context, sc features.CallerScope) ([]BadgeInfo, error) {
	if !s.badgesEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	badges, err := s.store.ListBadgeDefinitions(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("gamification: list badges: %w", err)
	}

	results := make([]BadgeInfo, 0, len(badges))
	iconIDs := make([]pgtype.UUID, 0, len(badges))
	for _, b := range badges {
		// §42 conformance — a badge linked to a disabled feature flag is
		// hidden from the public catalogue entirely (leaks nothing about
		// the unactivated feature).
		if !s.featureVisible(ctx, b.FeatureFlag, sc) {
			continue
		}
		info := s.badgeToInfo(ctx, b)
		// Public catalogue — unearned secret badges are masked the same way
		// the collection endpoint masks them (name/icon hidden, ??? slot).
		if b.IsSecret {
			info.Name = "???"
			info.Description = ""
			info.IconMediaID = ""
			// Trigger fields would reveal the unlock criteria — mask them.
			// Slug stays so clients can dedupe against the earned list.
			info.TriggerType = ""
			info.TriggerValue = 0
			info.TriggerActivityKey = ""
			// feature_flag reveals which feature unlocks it — mask that too.
			info.FeatureFlag = nil
		} else if b.IconMediaID.Valid {
			iconIDs = append(iconIDs, b.IconMediaID)
		}
		results = append(results, info)
	}

	urls := s.resolveIconURLs(ctx, iconIDs)
	for i := range results {
		results[i].IconURL = urls[results[i].IconMediaID]
	}

	return results, nil
}

// ListActiveChallenges returns active challenges within their date range.
// Returns ErrGamificationDisabled when gamification.challenges_enabled is off.
func (s *Service) ListActiveChallenges(ctx context.Context, sc features.CallerScope) ([]ChallengeInfo, error) {
	if !s.challengesEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	challenges, err := s.store.ListActiveChallenges(ctx)
	if err != nil {
		return nil, fmt.Errorf("gamification: list active challenges: %w", err)
	}

	results := make([]ChallengeInfo, 0, len(challenges))
	imgIDs := make([]pgtype.UUID, 0, len(challenges))
	for _, c := range challenges {
		// §42 conformance — challenges linked to a disabled feature flag
		// are hidden from the public list.
		if !s.featureVisible(ctx, c.FeatureFlag, sc) {
			continue
		}
		if c.ImageMediaID.Valid {
			imgIDs = append(imgIDs, c.ImageMediaID)
		}
		results = append(results, s.challengeToInfo(ctx, c))
	}

	urls := s.resolveIconURLs(ctx, imgIDs)
	for i := range results {
		results[i].ImageURL = urls[results[i].ImageMediaID]
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

// CanViewSpaceLeaderboard reports whether the viewer may read a space-scoped
// leaderboard (PTS-017). Global scope is always allowed. Public spaces are
// visible to everyone; non-public spaces require space membership —
// guests and non-members get false (handler maps to 404 so space privacy
// can't be probed through this route).
func (s *Service) CanViewSpaceLeaderboard(ctx context.Context, scope string, viewer pgtype.UUID) (bool, error) {
	scopeType, spaceIDStr, ok := ValidateLeaderboardScope(scope)
	if !ok || scopeType != ScopeSpace {
		return true, nil
	}

	var spaceID pgtype.UUID
	if err := spaceID.Scan(spaceIDStr); err != nil {
		return false, nil
	}

	vis, err := s.store.GetSpaceVisibility(ctx, spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Unknown space — deny; the handler maps this to 404 so the
			// endpoint can't be used to probe which UUIDs exist.
			return false, nil
		}
		return false, fmt.Errorf("gamification: space visibility: %w", err)
	}
	if vis == "public" {
		return true, nil
	}

	if !viewer.Valid {
		return false, nil
	}
	return s.store.IsSpaceMember(ctx, spaceID, viewer)
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
				Points:      r.PointsBalance,
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
	iconIDs := make([]pgtype.UUID, 0, len(rows))

	for _, r := range rows {
		var iconID *string

		if r.BadgeIconMediaID.Valid {
			s := r.BadgeIconMediaID.String()
			iconID = &s
			iconIDs = append(iconIDs, r.BadgeIconMediaID)
		}

		results = append(results, BadgeWithDetails{
			ID:          uuidStr(r.BadgeID),
			Slug:        r.BadgeSlug,
			Name:        r.BadgeName,
			Description: ptrStr(r.BadgeDescription),
			IconMediaID: iconID,
			AccentColor: ptrStr(r.BadgeAccentColor),
			Category:    ptrStr(r.BadgeCategory),
			Rarity:      r.BadgeRarity,
			Tier:        ptrStr(r.BadgeTier),
			BadgeGroup:  ptrStr(r.BadgeGroup),
			AwardedAt:   rfc3339(r.AwardedAt),
		})
	}

	urls := s.resolveIconURLs(ctx, iconIDs)
	for i := range results {
		if results[i].IconMediaID != nil {
			results[i].IconURL = urls[*results[i].IconMediaID]
		}
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
	iconIDs := make([]pgtype.UUID, 0, len(rows))
	for _, r := range rows {
		var iconID *string

		if r.BadgeIconMediaID.Valid {
			s := r.BadgeIconMediaID.String()
			iconID = &s
			iconIDs = append(iconIDs, r.BadgeIconMediaID)
		}

		results = append(results, BadgeWithDetails{
			ID:          uuidStr(r.BadgeID),
			Slug:        r.BadgeSlug,
			Name:        r.BadgeName,
			Description: ptrStr(r.BadgeDescription),
			IconMediaID: iconID,
			AccentColor: ptrStr(r.BadgeAccentColor),
			Category:    ptrStr(r.BadgeCategory),
			Rarity:      r.BadgeRarity,
			Tier:        ptrStr(r.BadgeTier),
			BadgeGroup:  ptrStr(r.BadgeGroup),
			AwardedAt:   rfc3339(r.AwardedAt),
		})
	}

	urls := s.resolveIconURLs(ctx, iconIDs)
	for i := range results {
		if results[i].IconMediaID != nil {
			results[i].IconURL = urls[*results[i].IconMediaID]
		}
	}

	return results, nil
}

// GetBadgeCollection returns the achievement-centre collection for the
// authenticated user: the active catalogue grouped by category, each badge
// carrying earned state and progress toward its goal. Unearned secret
// badges are masked — name/description and progress are hidden so the
// member knows a slot exists without revealing what unlocks it.
// Returns ErrGamificationDisabled when gamification.badges_enabled is off.
func (s *Service) GetBadgeCollection(ctx context.Context, userID pgtype.UUID, sc features.CallerScope) (BadgeCollection, error) {
	if !s.badgesEnabled(ctx) {
		return BadgeCollection{}, ErrGamificationDisabled
	}

	rows, err := s.store.ListBadgeCollectionForUser(ctx, userID)
	if err != nil {
		return BadgeCollection{}, fmt.Errorf("gamification: list badge collection: %w", err)
	}

	collection := BadgeCollection{
		Categories: make([]BadgeCollectionCategory, 0),
	}
	byCategory := make(map[string]int) // category slug → index in Categories

	// Live-progress caches — one query per distinct counter across all
	// locked badges in the collection.
	liveCounts := make(map[string]int64)
	lifetimePts := int64(-1)
	iconIDs := make([]pgtype.UUID, 0, len(rows))

	for _, r := range rows {
		// §42 conformance — definitions linked to a disabled feature flag
		// are excluded from the collection entirely (earned or not), so
		// category totals and stats only count attainable badges. Earned
		// awards still appear via the /me/badges + profile badge lists.
		if !s.featureVisible(ctx, r.FeatureFlag, sc) {
			continue
		}

		category := ptrStr(r.Category)
		if category == "" {
			category = "general"
		}

		cb := CollectionBadge{
			ID:           uuidStr(r.ID),
			Slug:         r.Slug,
			Name:         r.Name,
			Description:  ptrStr(r.Description),
			IconMediaID:  uuidStr(r.IconMediaID),
			AccentColor:  ptrStr(r.AccentColor),
			Category:     category,
			Rarity:       r.Rarity,
			Tier:         ptrStr(r.Tier),
			BadgeGroup:   ptrStr(r.BadgeGroup),
			IsSecret:     r.IsSecret,
			IsLimited:    r.IsLimited,
			IsFeatured:   r.IsFeatured,
			PointsReward: int(r.PointsReward),
			Earned:       r.UserBadgeID.Valid,
			FeatureFlag:  r.FeatureFlag,
		}

		if r.UserAwardedAt.Valid {
			ts := rfc3339(r.UserAwardedAt)
			cb.AwardedAt = &ts
		}

		if r.TriggerValue != nil {
			cb.Goal = int(*r.TriggerValue)
		}

		if r.ProgressCount != nil {
			cb.Progress = int(*r.ProgressCount)
		} else if !r.UserBadgeID.Valid && !r.IsSecret && r.TriggerValue != nil {
			// No badge_progress row yet — the triggering event simply hasn't
			// fired since v2 shipped. Compute the live counter so locked
			// lifetime_points/streak_days/activity_count badges still show a
			// progress bar (display-only; nothing is written).
			cb.Progress = s.liveBadgeProgress(ctx, userID, r, liveCounts, &lifetimePts)
		}

		// Secret badges stay masked until earned — the catalogue lists the
		// slot without revealing what unlocks it.
		if r.IsSecret && !r.UserBadgeID.Valid {
			cb.Name = "???"
			cb.Description = ""
			cb.IconMediaID = ""
			cb.Progress = 0
			cb.Goal = 0
			// feature_flag reveals which feature unlocks it — mask that too.
			cb.FeatureFlag = nil
		} else if r.IconMediaID.Valid {
			iconIDs = append(iconIDs, r.IconMediaID)
		}

		idx, ok := byCategory[category]
		if !ok {
			collection.Categories = append(collection.Categories, BadgeCollectionCategory{
				Category: category,
				Badges:   make([]CollectionBadge, 0),
			})
			idx = len(collection.Categories) - 1
			byCategory[category] = idx
		}

		cat := &collection.Categories[idx]
		cat.Total++
		collection.Stats.Total++

		if r.UserBadgeID.Valid {
			cat.Earned++
			collection.Stats.Earned++
		}

		cat.Badges = append(cat.Badges, cb)
	}

	collection.Stats.Remaining = collection.Stats.Total - collection.Stats.Earned
	if collection.Stats.Total > 0 {
		collection.Stats.Percent = collection.Stats.Earned * 100 / collection.Stats.Total
	}

	urls := s.resolveIconURLs(ctx, iconIDs)
	for ci := range collection.Categories {
		for bi := range collection.Categories[ci].Badges {
			b := &collection.Categories[ci].Badges[bi]
			b.IconURL = urls[b.IconMediaID]
		}
	}

	return collection, nil
}

// liveBadgeProgress computes an on-the-fly counter for a locked badge that
// has no badge_progress row — lifetime_points reads the wallet ledger once,
// streak_days and activity_count read one counter per distinct activity key
// (both memoised in `counts`). Returns the counter capped at the goal.
func (s *Service) liveBadgeProgress(ctx context.Context, userID pgtype.UUID, r gen.ListBadgeCollectionForUserRow, counts map[string]int64, lifetime *int64) int {
	goal := int64(*r.TriggerValue)
	var v int64
	switch r.TriggerType {
	case TriggerTypeLifetimePoints:
		if *lifetime < 0 {
			pts, err := s.store.GetUserLifetimePoints(ctx, userID)
			if err != nil {
				slog.Warn("gamification: lifetime points for collection failed", "error", err)
				pts = 0
			}
			*lifetime = pts
		}
		v = *lifetime
	case TriggerTypeStreakDays, TriggerTypeActivityCount:
		actKey := ptrStr(r.TriggerActivityKey)
		key := r.TriggerType + ":" + actKey
		cached, ok := counts[key]
		if !ok {
			if r.TriggerType == TriggerTypeStreakDays {
				days, err := s.store.GetActivityDayStreak(ctx, userID, actKey)
				if err == nil {
					cached = int64(days)
				}
			} else {
				n, err := s.store.CountUserActivityByKey(ctx, userID, actKey)
				if err == nil {
					cached = n
				}
			}
			counts[key] = cached
		}
		v = cached
	default:
		return 0 // manual / unknown triggers carry no computable progress
	}
	if v > goal {
		v = goal
	}
	if v < 0 {
		v = 0
	}
	return int(v)
}

// ListUserChallengeProgress returns a user's challenge progress with
// details. Returns ErrGamificationDisabled when
// gamification.challenges_enabled is off.
func (s *Service) ListUserChallengeProgress(ctx context.Context, userID pgtype.UUID, sc features.CallerScope) ([]ChallengeWithProgress, error) {
	if !s.challengesEnabled(ctx) {
		return nil, ErrGamificationDisabled
	}

	rows, err := s.store.ListUserChallengeProgress(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("gamification: list user challenge progress: %w", err)
	}

	results := make([]ChallengeWithProgress, 0, len(rows))

	for _, r := range rows {
		// §42 conformance — in-progress rows for challenges whose feature
		// flag is disabled are hidden; completed rows stay visible as
		// earned history.
		if !r.Completed && !s.featureVisible(ctx, r.ChallengeFeatureFlag, sc) {
			continue
		}

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
