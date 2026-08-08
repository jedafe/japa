package gamification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Badge evaluation — called by wallet:process_activity_event
// ---------------------------------------------------------------------------

// EvaluateBadges checks activity_count badges for the given activity_key,
// determines if thresholds are newly crossed, and awards badges. Also
// increments challenge progress for matching challenges. Returns the newly
// awarded badges.
//
// This is called by the wallet:process_activity_event task after writing the
// points ledger entry.
func (s *Service) EvaluateBadges(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx, tenantID) {
		return nil, nil
	}

	// Get the current activity count for this activity_key.
	currentCount, err := s.store.CountUserActivityByKey(ctx, tenantID, userID, activityKey)
	if err != nil {
		return nil, fmt.Errorf("gamification: count user activity: %w", err)
	}

	// Get all activity_count badges for this activity_key.
	badges, err := s.store.GetBadgesByActivityTrigger(ctx, tenantID, activityKey)
	if err != nil {
		return nil, fmt.Errorf("gamification: get badges by activity trigger: %w", err)
	}

	awarded := make([]BadgeWithDetails, 0)
	for _, b := range badges {
		if b.TriggerValue == nil {
			continue
		}
		threshold := int64(*b.TriggerValue)
		if currentCount < threshold {
			continue
		}

		// Check if the user already has this badge.
		_, err := s.store.GetUserBadge(ctx, tenantID, userID, b.ID)
		if err == nil {
			continue // already awarded
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("gamification: check user badge: %w", err)
		}

		// Award the badge (awarded_by = NULL = system).
		ub, err := s.store.AwardBadge(ctx, gen.AwardBadgeParams{
			TenantID: tenantID,
			UserID:   userID,
			BadgeID:  b.ID,
		})
		if err != nil {
			// ON CONFLICT DO NOTHING may return no rows — skip duplicates.
			slog.Warn("gamification: evaluate: award badge failed",
				"badge_id", uuidStr(b.ID), "error", err)
			continue
		}

		var iconID *string
		if b.IconMediaID.Valid {
			s := b.IconMediaID.String()
			iconID = &s
		}
		awarded = append(awarded, BadgeWithDetails{
			ID:          uuidStr(b.ID),
			Slug:        b.Slug,
			Name:        b.Name,
			Description: ptrStr(b.Description),
			IconMediaID: iconID,
			AwardedAt:   rfc3339(ub.AwardedAt),
		})
	}

	// Increment challenge progress for matching challenges.
	if s.challengesEnabled(ctx, tenantID) {
		if err := s.incrementChallengeProgress(ctx, tenantID, userID, activityKey); err != nil {
			slog.Error("gamification: evaluate: increment challenge progress",
				"activity_key", activityKey, "error", err)
		}
	}

	return awarded, nil
}

// incrementChallengeProgress increments progress for all active challenges
// matching the activity_key by 1.
func (s *Service) incrementChallengeProgress(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) error {
	challenges, err := s.store.GetChallengesByActivityKey(ctx, tenantID, activityKey)
	if err != nil {
		return fmt.Errorf("get challenges by activity key: %w", err)
	}

	for _, c := range challenges {
		progress, err := s.store.UpsertChallengeProgress(ctx, gen.UpsertChallengeProgressParams{
			TenantID:     tenantID,
			UserID:       userID,
			ChallengeID:  c.ID,
			CurrentCount: 1,
		})
		if err != nil {
			slog.Warn("gamification: upsert challenge progress failed",
				"challenge_id", uuidStr(c.ID), "error", err)
			continue
		}

		// If the challenge just completed, award the reward badge if set.
		if progress.Completed && progress.CompletedAt.Valid && c.RewardBadgeID.Valid {
			// Only award the badge if it wasn't already awarded (the
			// completed_at timestamp changing indicates a fresh completion).
			_, err := s.store.GetUserBadge(ctx, tenantID, userID, c.RewardBadgeID)
			if err == nil {
				continue
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				slog.Warn("gamification: check reward badge", "error", err)
				continue
			}
			if _, err := s.store.AwardBadge(ctx, gen.AwardBadgeParams{
				TenantID: tenantID,
				UserID:   userID,
				BadgeID:  c.RewardBadgeID,
			}); err != nil {
				slog.Warn("gamification: award challenge reward badge failed",
					"challenge_id", uuidStr(c.ID), "error", err)
			}
		}
	}
	return nil
}

// EvaluatePointsBadges checks lifetime_points badges against the user's
// current lifetime points and awards any newly-earned badges.
func (s *Service) EvaluatePointsBadges(ctx context.Context, tenantID, userID pgtype.UUID, currentLifetimePoints int64) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx, tenantID) {
		return nil, nil
	}

	badges, err := s.store.GetBadgesByPointsTrigger(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gamification: get points badges: %w", err)
	}

	awarded := make([]BadgeWithDetails, 0)
	for _, b := range badges {
		if b.TriggerValue == nil {
			continue
		}
		threshold := int64(*b.TriggerValue)
		if currentLifetimePoints < threshold {
			continue
		}

		_, err := s.store.GetUserBadge(ctx, tenantID, userID, b.ID)
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("gamification: check user badge: %w", err)
		}

		ub, err := s.store.AwardBadge(ctx, gen.AwardBadgeParams{
			TenantID: tenantID,
			UserID:   userID,
			BadgeID:  b.ID,
		})
		if err != nil {
			slog.Warn("gamification: evaluate points: award badge failed",
				"badge_id", uuidStr(b.ID), "error", err)
			continue
		}

		var iconID *string
		if b.IconMediaID.Valid {
			s := b.IconMediaID.String()
			iconID = &s
		}
		awarded = append(awarded, BadgeWithDetails{
			ID:          uuidStr(b.ID),
			Slug:        b.Slug,
			Name:        b.Name,
			Description: ptrStr(b.Description),
			IconMediaID: iconID,
			AwardedAt:   rfc3339(ub.AwardedAt),
		})
	}

	return awarded, nil
}
