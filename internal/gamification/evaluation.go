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
	awarded := make([]BadgeWithDetails, 0)

	// activity_count badges — gated by gamification.badges_enabled only. The
	// challenge increment below is gated separately by challenges_enabled so
	// disabling badges does not silently stop challenge progress.
	if s.badgesEnabled(ctx, tenantID) {
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

				UserID:  userID,
				BadgeID: b.ID,
			})
			if err != nil {
				// ON CONFLICT DO NOTHING may return no rows — skip duplicates.
				slog.Warn("gamification: evaluate: award badge failed",
					"badge_id", uuidStr(b.ID), "error", err)
				continue
			}

			s.notifyBadgeAwarded(ctx, tenantID, userID, b.Name)

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
	}

	// Increment challenge progress for matching challenges — independent of
	// the badges toggle.
	if s.challengesEnabled(ctx, tenantID) {
		if err := s.incrementChallengeProgress(ctx, tenantID, userID, activityKey); err != nil {
			slog.Error("gamification: evaluate: increment challenge progress",
				"activity_key", activityKey, "error", err)
		}
	}

	return awarded, nil
}

// incrementChallengeProgress increments progress for all active challenges
// matching the activity_key by 1. On the first transition to completed it
// grants the configured rewards — reward badge, reward points, and a
// completion notification — and marks the reward claimed.
func (s *Service) incrementChallengeProgress(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) error {
	challenges, err := s.store.GetChallengesByActivityKey(ctx, tenantID, activityKey)
	if err != nil {
		return fmt.Errorf("get challenges by activity key: %w", err)
	}

	for _, c := range challenges {
		// Read the prior progress row so a fresh completion can be told apart
		// from increments that land after the challenge already completed —
		// without this the reward would fire on every subsequent event.
		prior, priorErr := s.store.GetChallengeProgress(ctx, tenantID, userID, c.ID)
		wasCompleted := priorErr == nil && prior.Completed

		progress, err := s.store.UpsertChallengeProgress(ctx, gen.UpsertChallengeProgressParams{

			UserID:       userID,
			ChallengeID:  c.ID,
			CurrentCount: 1,
		})
		if err != nil {
			slog.Warn("gamification: upsert challenge progress failed",
				"challenge_id", uuidStr(c.ID), "error", err)
			continue
		}

		if wasCompleted || !progress.Completed {
			continue
		}

		s.grantChallengeReward(ctx, tenantID, userID, c, progress)
	}

	return nil
}

// notifyBadgeAwarded fires the badge-award notification (fire-and-forget,
// Rule 8) — nil-safe when no Notifier is wired.
func (s *Service) notifyBadgeAwarded(ctx context.Context, tenantID, userID pgtype.UUID, badgeName string) {
	if s.notifier == nil {
		return
	}

	if err := s.notifier.Notify(ctx, tenantID, userID, NotifTypeBadgeAwarded,
		"Badge earned: "+badgeName,
		"You earned the "+badgeName+" badge.",
		""); err != nil {
		slog.Warn("gamification: badge award notification failed",
			"badge", badgeName, "error", err)
	}
}

// grantChallengeReward delivers everything a completed challenge pays out:
// the reward badge (when configured), the reward points via the wallet
// ledger (idempotent on the challenge id), and the completion notification.
// The progress row's reward_claimed flag is set once a reward lands.
// Failures are logged and skipped — a reward miss must not fail the
// activity event that triggered it.
func (s *Service) grantChallengeReward(ctx context.Context, tenantID, userID pgtype.UUID, c gen.ChallengeDefinition, progress gen.UserChallengeProgress) {
	rewardGranted := false

	if c.RewardBadgeID.Valid {
		// Skip when the user already holds the reward badge.
		_, err := s.store.GetUserBadge(ctx, tenantID, userID, c.RewardBadgeID)
		switch {
		case err == nil:
			// already held
		case !errors.Is(err, pgx.ErrNoRows):
			slog.Warn("gamification: check reward badge", "error", err)
		default:
			if _, err := s.store.AwardBadge(ctx, gen.AwardBadgeParams{

				UserID:  userID,
				BadgeID: c.RewardBadgeID,
			}); err != nil {
				slog.Warn("gamification: award challenge reward badge failed",
					"challenge_id", uuidStr(c.ID), "error", err)
			} else {
				rewardGranted = true
			}
		}
	}

	if c.RewardPoints > 0 && s.pointsAwarder != nil {
		// The ledger idempotency key (gamification_award:{user}:{reason})
		// makes retried or concurrent completions safe.
		reason := "challenge_completed:" + uuidStr(c.ID)
		if err := s.pointsAwarder.AwardPoints(ctx, tenantID, userID, int(c.RewardPoints), reason); err != nil {
			slog.Warn("gamification: award challenge reward points failed",
				"challenge_id", uuidStr(c.ID), "error", err)
		} else {
			rewardGranted = true
		}
	}

	if rewardGranted {
		if err := s.store.MarkChallengeRewardClaimed(ctx, progress.ID); err != nil {
			slog.Warn("gamification: mark challenge reward claimed failed",
				"progress_id", uuidStr(progress.ID), "error", err)
		}
	}

	if s.notifier != nil {
		if err := s.notifier.Notify(ctx, tenantID, userID, NotifTypeChallengeCompleted,
			"Challenge completed: "+c.Name,
			"You completed the "+c.Name+" challenge and earned your reward.",
			"/challenges"); err != nil {
			slog.Warn("gamification: challenge completion notification failed",
				"challenge_id", uuidStr(c.ID), "error", err)
		}
	}
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

			UserID:  userID,
			BadgeID: b.ID,
		})
		if err != nil {
			slog.Warn("gamification: evaluate points: award badge failed",
				"badge_id", uuidStr(b.ID), "error", err)
			continue
		}

		s.notifyBadgeAwarded(ctx, tenantID, userID, b.Name)

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
