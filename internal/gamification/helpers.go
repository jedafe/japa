package gamification

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Settings helpers
// ---------------------------------------------------------------------------

// badgesEnabled returns whether the badge system is enabled.
func (s *Service) badgesEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, pgtype.UUID{}, "gamification.badges_enabled", true)
}

// challengesEnabled returns whether the challenge system is enabled.
func (s *Service) challengesEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, pgtype.UUID{}, "gamification.challenges_enabled", true)
}

// leaderboardEnabled returns whether the leaderboard is visible.
func (s *Service) leaderboardEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, pgtype.UUID{}, "gamification.leaderboard_enabled", true)
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

// badgeToInfo converts a gen.BadgeDefinition to a BadgeInfo DTO.
func badgeToInfo(b gen.BadgeDefinition) BadgeInfo {
	info := BadgeInfo{
		ID:           uuidStr(b.ID),
		Slug:         b.Slug,
		Name:         b.Name,
		Description:  ptrStr(b.Description),
		IconMediaID:  uuidStr(b.IconMediaID),
		TriggerType:  b.TriggerType,
		IsActive:     b.IsActive,
		IsSystem:     b.IsSystem,
		DisplayOrder: int(b.DisplayOrder),
		CreatedAt:    rfc3339(b.CreatedAt),
	}
	if b.TriggerValue != nil {
		info.TriggerValue = int(*b.TriggerValue)
	}

	if b.TriggerActivityKey != nil {
		info.TriggerActivityKey = *b.TriggerActivityKey
	}

	return info
}

// challengeToInfo converts a gen.ChallengeDefinition to a ChallengeInfo DTO.
func challengeToInfo(c gen.ChallengeDefinition) ChallengeInfo {
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

	return info
}
