package gamification

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
)

// ---------------------------------------------------------------------------
// Settings helpers
// ---------------------------------------------------------------------------

// badgesEnabled returns whether the badge system is enabled.
func (s *Service) badgesEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "gamification.badges_enabled", true)
}

// challengesEnabled returns whether the challenge system is enabled.
func (s *Service) challengesEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "gamification.challenges_enabled", true)
}

// leaderboardEnabled returns whether the leaderboard is visible.
func (s *Service) leaderboardEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "gamification.leaderboard_enabled", true)
}

// ---------------------------------------------------------------------------
// Feature-flag helpers (§42) — badge_definitions.feature_flag and
// challenge_definitions.feature_flag (migration 001086).
// ---------------------------------------------------------------------------

// featureAvailable reports whether an item carrying an optional feature_flag
// may be evaluated. A NULL flag means the item is not tied to a gated feature
// and is always available. A set flag resolves through the injected checker
// and fails closed — unknown keys, disabled flags, and an unwired checker all
// hide the item (same contract as menu_items.feature_flag). Evaluation runs
// in background (wallet:process_activity_event) with no caller scope, so the
// global toggle is the right semantics: a flag scoped to a subset of users
// still legitimately emits events for the users who reach it.
func (s *Service) featureAvailable(ctx context.Context, flag *string) bool {
	if flag == nil || *flag == "" {
		return true
	}
	if s.checker == nil {
		return false
	}
	return s.checker.IsEnabled(ctx, *flag)
}

// featureVisible reports whether an item carrying an optional feature_flag
// may be shown to THIS caller on public catalogue/collection surfaces —
// surface/geo/audience rules apply on top of the global toggle, matching
// menu_items.feature_flag pruning (menus.filterMenuItems). A flag enabled
// globally but scoped away from this caller hides the item: an unactivated
// or unreachable feature's badges must not surface. Fails closed on an
// unwired checker.
func (s *Service) featureVisible(ctx context.Context, flag *string, sc features.CallerScope) bool {
	if flag == nil || *flag == "" {
		return true
	}
	if s.checker == nil {
		return false
	}
	res := s.checker.EvalFlag(ctx, *flag, sc)
	return res.Enabled && res.Visible
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

// rfc3339 formats a pgtype.Timestamptz as RFC 3339 UTC, or "" if invalid.
func rfc3339(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}

// ptrStr dereferences a *string safely, returning "" for nil.
func ptrStr(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}

// uuidStr returns the string representation of a pgtype.UUID, or "" if invalid.
func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}

	return u.String()
}

// scanUUID parses a UUID string into a pgtype.UUID.
func scanUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}

	return u, nil
}

// scanTimestamp parses an RFC 3339 string into a pgtype.Timestamptz.
func scanTimestamp(s string) (pgtype.Timestamptz, error) {
	if s == "" {
		return pgtype.Timestamptz{}, nil
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return pgtype.Timestamptz{}, fmt.Errorf("gamification: parse timestamp %q: %w", s, err)
	}

	return pgtype.Timestamptz{Time: t, Valid: true}, nil
}

// mediaURL resolves an artwork media id (badge icon, challenge image) to a
// displayable URL via the injected MediaResolver. Returns "" when unwired
// or resolution fails — artwork is decorative and never fails a read.
func (s *Service) mediaURL(ctx context.Context, mediaID pgtype.UUID) string {
	if s.mediaResolver == nil || !mediaID.Valid {
		return ""
	}
	u, err := s.mediaResolver.ResolveMediaURL(ctx, mediaID)
	if err != nil {
		return ""
	}
	return u
}

// artworkChanged reports whether an update is actually replacing the
// artwork — re-saving the same media id skips the knockout round-trip.
func artworkChanged(current, next pgtype.UUID) bool {
	return !current.Valid || current != next
}

// resolveIconURLs batch-resolves artwork media ids to display URLs — one
// resolver round-trip for a whole list instead of one per badge. Keyed by
// media id string so callers can look up per-row IconMediaID values.
func (s *Service) resolveIconURLs(ctx context.Context, mediaIDs []pgtype.UUID) map[string]string {
	if s.mediaResolver == nil || len(mediaIDs) == 0 {
		return nil
	}
	ids := make([]pgtype.UUID, 0, len(mediaIDs))
	seen := make(map[pgtype.UUID]bool, len(mediaIDs))
	for _, id := range mediaIDs {
		if !id.Valid || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	urls, err := s.mediaResolver.ResolveMediaURLs(ctx, ids)
	if err != nil {
		return nil
	}
	return urls
}

// Artwork frame sizes — the longest edge an attached asset is downscaled
// to. Badge shields render at most ~80px (celebration modal) so 512px
// covers high-DPI; challenge heroes span a card column so they get more.
const (
	badgeIconMaxDim      = 512
	challengeImageMaxDim = 1536
)

// knockoutArtwork removes the solid edge background on a newly attached
// artwork media id (badge icon, challenge image) and downscales it to fit
// its frame so the asset rhymes with the circular badge/hero UI. Runs
// fire-and-forget (Rule 8) so a heavyweight raster transform never blocks
// the admin write — a brief window may serve the original until the
// rewrite lands. Nil-safe and failure-tolerant.
func (s *Service) knockoutArtwork(ctx context.Context, mediaID string, maxDim int) {
	if s.mediaProcessor == nil || mediaID == "" {
		return
	}
	id, err := scanUUID(mediaID)
	if err != nil || !id.Valid {
		return
	}
	go func() {
		if err := s.mediaProcessor.RemoveEdgeBackground(context.WithoutCancel(ctx), id, maxDim); err != nil {
			slog.Warn("gamification: artwork background knockout failed",
				"media_id", mediaID, "error", err)
		}
	}()
}

// badgeToInfo converts a gen.BadgeDefinition to a BadgeInfo DTO.
func (s *Service) badgeToInfo(ctx context.Context, b gen.BadgeDefinition) BadgeInfo {
	info := BadgeInfo{
		ID:           uuidStr(b.ID),
		Slug:         b.Slug,
		Name:         b.Name,
		Description:  ptrStr(b.Description),
		IconMediaID:  uuidStr(b.IconMediaID),
		AccentColor:  ptrStr(b.AccentColor),
		TriggerType:  b.TriggerType,
		IsActive:     b.IsActive,
		IsSystem:     b.IsSystem,
		DisplayOrder: int(b.DisplayOrder),
		Rarity:       b.Rarity,
		IsSecret:     b.IsSecret,
		IsLimited:    b.IsLimited,
		IsFeatured:   b.IsFeatured,
		PointsReward: int(b.PointsReward),
		CreatedAt:    rfc3339(b.CreatedAt),
	}
	if b.TriggerValue != nil {
		info.TriggerValue = int(*b.TriggerValue)
	}

	if b.TriggerActivityKey != nil {
		info.TriggerActivityKey = *b.TriggerActivityKey
	}

	if b.Category != nil {
		info.Category = *b.Category
	}

	if b.Tier != nil {
		info.Tier = *b.Tier
	}

	if b.BadgeGroup != nil {
		info.BadgeGroup = *b.BadgeGroup
	}

	if b.AvailableFrom.Valid {
		s := rfc3339(b.AvailableFrom)
		info.AvailableFrom = &s
	}

	if b.AvailableUntil.Valid {
		s := rfc3339(b.AvailableUntil)
		info.AvailableUntil = &s
	}

	if b.FeatureFlag != nil && *b.FeatureFlag != "" {
		info.FeatureFlag = b.FeatureFlag
	}

	return info
}

// badgeWithDetails builds the earned-badge DTO shared by the evaluator,
// the user-badge list, and the admin award path.
func (s *Service) badgeWithDetails(ctx context.Context, b gen.BadgeDefinition, awardedAt pgtype.Timestamptz) BadgeWithDetails {
	var iconID *string

	if b.IconMediaID.Valid {
		s := b.IconMediaID.String()
		iconID = &s
	}

	return BadgeWithDetails{
		ID:          uuidStr(b.ID),
		Slug:        b.Slug,
		Name:        b.Name,
		Description: ptrStr(b.Description),
		IconMediaID: iconID,
		Category:    ptrStr(b.Category),
		Rarity:      b.Rarity,
		Tier:        ptrStr(b.Tier),
		BadgeGroup:  ptrStr(b.BadgeGroup),
		AwardedAt:   rfc3339(awardedAt),
	}
}

// challengeToInfo converts a gen.ChallengeDefinition to a ChallengeInfo DTO.
func (s *Service) challengeToInfo(ctx context.Context, c gen.ChallengeDefinition) ChallengeInfo {
	info := ChallengeInfo{
		ID:           uuidStr(c.ID),
		Slug:         c.Slug,
		Name:         c.Name,
		Description:  ptrStr(c.Description),
		ImageMediaID: uuidStr(c.ImageMediaID),
		ActivityKey:  c.ActivityKey,
		TargetCount:  int(c.TargetCount),
		RewardPoints: int(c.RewardPoints),
		StartsAt:     rfc3339(c.StartsAt),
		IsActive:     c.IsActive,
		CreatedAt:    rfc3339(c.CreatedAt),
	}

	if c.RewardBadgeID.Valid {
		id := c.RewardBadgeID.String()
		info.RewardBadgeID = id
	}

	if c.EndsAt.Valid {
		ends := rfc3339(c.EndsAt)
		info.EndsAt = &ends
	}

	if c.FeatureFlag != nil && *c.FeatureFlag != "" {
		info.FeatureFlag = c.FeatureFlag
	}

	return info
}

// parseTimestamp parses an RFC3339 timestamp into a pgtype.Timestamptz.
func parseTimestamp(s string) (pgtype.Timestamptz, error) {
	var ts pgtype.Timestamptz
	if s == "" {
		return ts, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ts, err
	}
	ts.Time = t.UTC()
	ts.Valid = true
	return ts, nil
}
