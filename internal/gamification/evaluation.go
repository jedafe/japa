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

// EvaluateBadges checks activity_count and streak_days badges for the given
// activity_key, determines if thresholds are newly crossed, and awards
// badges. Also increments challenge progress for matching challenges.
// Returns the newly awarded badges.
//
// This is called by the wallet:process_activity_event task after writing the
// points ledger entry.
func (s *Service) EvaluateBadges(ctx context.Context, userID pgtype.UUID, activityKey string) ([]BadgeWithDetails, error) {
	awarded := make([]BadgeWithDetails, 0)

	// badges — gated by gamification.badges_enabled only. The challenge
	// increment below is gated separately by challenges_enabled so disabling
	// badges does not silently stop challenge progress.
	if s.badgesEnabled(ctx) {
		newlyAwarded, err := s.evaluateCountBadges(ctx, userID, activityKey)
		if err != nil {
			return nil, err
		}
		awarded = append(awarded, newlyAwarded...)

		streakAwarded, err := s.evaluateStreakBadges(ctx, userID, activityKey)
		if err != nil {
			return nil, err
		}
		awarded = append(awarded, streakAwarded...)
	}

	// Increment challenge progress for matching challenges — independent of
	// the badges toggle.
	if s.challengesEnabled(ctx) {
		if err := s.incrementChallengeProgress(ctx, userID, activityKey); err != nil {
			slog.Error("gamification: evaluate: increment challenge progress",
				"activity_key", activityKey, "error", err)
		}
	}

	return awarded, nil
}

// evaluateCountBadges handles trigger_type='activity_count' rows matching
// the event's key: refreshes badge_progress for every matching badge, then
// awards any whose threshold is newly met.
func (s *Service) evaluateCountBadges(ctx context.Context, userID pgtype.UUID, activityKey string) ([]BadgeWithDetails, error) {
	awarded := make([]BadgeWithDetails, 0)

	// Get all activity_count badges for this activity_key.
	badges, err := s.store.GetBadgesByActivityTrigger(ctx, activityKey)
	if err != nil {
		return nil, fmt.Errorf("gamification: get badges by activity trigger: %w", err)
	}

	if len(badges) == 0 {
		return awarded, nil
	}

	// Get the current activity count for this activity_key.
	currentCount, err := s.store.CountUserActivityByKey(ctx, userID, activityKey)
	if err != nil {
		return nil, fmt.Errorf("gamification: count user activity: %w", err)
	}

	for _, b := range badges {
		if b.TriggerValue == nil {
			continue
		}

		// Track progress toward locked badges for the achievement centre.
		// Best-effort — a failure must not block evaluation.
		if err := s.store.UpsertBadgeProgress(ctx, userID, b.ID, int32(min64(currentCount, int64(*b.TriggerValue)))); err != nil {
			slog.Warn("gamification: upsert badge progress failed",
				"badge_id", uuidStr(b.ID), "error", err)
		}

		threshold := int64(*b.TriggerValue)
		if currentCount < threshold {
			continue
		}

		if d, ok := s.awardIfNew(ctx, userID, b, TriggerTypeActivityCount); ok {
			awarded = append(awarded, d)
		}
	}

	return awarded, nil
}

// evaluateStreakBadges handles trigger_type='streak_days' rows matching the
// event's key: computes the user's current consecutive-day streak for the
// key and awards badges whose day threshold is newly met.
func (s *Service) evaluateStreakBadges(ctx context.Context, userID pgtype.UUID, activityKey string) ([]BadgeWithDetails, error) {
	awarded := make([]BadgeWithDetails, 0)

	badges, err := s.store.GetBadgesByStreakTrigger(ctx, activityKey)
	if err != nil {
		return nil, fmt.Errorf("gamification: get badges by streak trigger: %w", err)
	}

	if len(badges) == 0 {
		return awarded, nil
	}

	streak, err := s.store.GetActivityDayStreak(ctx, userID, activityKey)
	if err != nil {
		return nil, fmt.Errorf("gamification: activity day streak: %w", err)
	}

	for _, b := range badges {
		if b.TriggerValue == nil {
			continue
		}

		if err := s.store.UpsertBadgeProgress(ctx, userID, b.ID, int32(min64(int64(streak), int64(*b.TriggerValue)))); err != nil {
			slog.Warn("gamification: upsert badge progress failed",
				"badge_id", uuidStr(b.ID), "error", err)
		}

		if int64(streak) < int64(*b.TriggerValue) {
			continue
		}

		if d, ok := s.awardIfNew(ctx, userID, b, TriggerTypeStreakDays); ok {
			awarded = append(awarded, d)
		}
	}

	return awarded, nil
}

// awardIfNew awards the badge when the user does not already hold it. On a
// fresh award it writes the badge_award_events audit row, grants the
// badge's points_reward (when configured), and notifies the user. Returns
// the awarded-badge DTO and true on a fresh award.
func (s *Service) awardIfNew(ctx context.Context, userID pgtype.UUID, b gen.BadgeDefinition, source string) (BadgeWithDetails, bool) {
	// Check if the user already has this badge.
	_, err := s.store.GetUserBadge(ctx, userID, b.ID)
	if err == nil {
		return BadgeWithDetails{}, false // already awarded
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("gamification: check user badge failed",
			"badge_id", uuidStr(b.ID), "error", err)
		return BadgeWithDetails{}, false
	}

	// Award the badge (awarded_by = NULL = system).
	ub, err := s.store.AwardBadge(ctx, gen.AwardBadgeParams{
		UserID:  userID,
		BadgeID: b.ID,
	})
	if err != nil {
		// ON CONFLICT DO NOTHING may return no rows — skip duplicates.
		slog.Warn("gamification: award badge failed",
			"badge_id", uuidStr(b.ID), "error", err)
		return BadgeWithDetails{}, false
	}

	// Audit trail — best-effort.
	if err := s.store.InsertBadgeAwardEvent(ctx, gen.InsertBadgeAwardEventParams{
		UserID:  userID,
		BadgeID: b.ID,
		Source:  source,
	}); err != nil {
		slog.Warn("gamification: insert badge award event failed",
			"badge_id", uuidStr(b.ID), "error", err)
	}

	s.notifyBadgeAwarded(ctx, userID, b.Name)
	s.grantBadgePoints(ctx, userID, b)

	d := s.badgeWithDetails(ctx, b, ub.AwardedAt)
	d.IconURL = s.mediaURL(ctx, b.IconMediaID)
	return d, true
}

// grantBadgePoints credits a badge's points_reward through the retryable
// wallet:award_points task (idempotent on the badge id). Nil-safe.
func (s *Service) grantBadgePoints(ctx context.Context, userID pgtype.UUID, b gen.BadgeDefinition) {
	if b.PointsReward <= 0 || s.pointsAwarder == nil {
		return
	}

	reason := "badge_awarded:" + uuidStr(b.ID)
	if err := s.pointsAwarder.EnqueuePointsAward(ctx, userID, int(b.PointsReward), reason, uuidStr(b.ID)); err != nil {
		slog.Warn("gamification: award badge points failed",
			"badge_id", uuidStr(b.ID), "error", err)
	}
}

// min64 returns the smaller of two int64s.
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// incrementChallengeProgress increments progress for all active challenges
// matching the activity_key by 1. On the first transition to completed it
// grants the configured rewards — reward badge, reward points, and a
// completion notification — and marks the reward claimed.
func (s *Service) incrementChallengeProgress(ctx context.Context, userID pgtype.UUID, activityKey string) error {
	challenges, err := s.store.GetChallengesByActivityKey(ctx, activityKey)
	if err != nil {
		return fmt.Errorf("get challenges by activity key: %w", err)
	}

	for _, c := range challenges {
		// Read the prior progress row so a fresh completion can be told apart
		// from increments that land after the challenge already completed —
		// without this the reward would fire on every subsequent event.
		prior, priorErr := s.store.GetChallengeProgress(ctx, userID, c.ID)
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

		s.grantChallengeReward(ctx, userID, c, progress)

		// Feed challenge-count badges (Challenge Accepted/Hunter/...) — the
		// event dedupes on (activity_key, reference) so retries are safe.
		s.emitActivity(ctx, userID, "challenge.completed",
			"challenge:"+uuidStr(c.ID), nil)
	}

	return nil
}

// notifyBadgeAwarded fires the badge-award notification (fire-and-forget,
// Rule 8) — nil-safe when no Notifier is wired.
func (s *Service) notifyBadgeAwarded(ctx context.Context, userID pgtype.UUID, badgeName string) {
	if s.notifier == nil {
		return
	}

	if err := s.notifier.Notify(ctx, userID, NotifTypeBadgeAwarded,
		"Badge earned: "+badgeName,
		"You earned the "+badgeName+" badge.",
		"/badges"); err != nil {
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
func (s *Service) grantChallengeReward(ctx context.Context, userID pgtype.UUID, c gen.ChallengeDefinition, progress gen.UserChallengeProgress) {
	rewardGranted := false

	if c.RewardBadgeID.Valid {
		// Skip when the user already holds the reward badge.
		_, err := s.store.GetUserBadge(ctx, userID, c.RewardBadgeID)

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
				// Audit trail — best-effort.
				if err := s.store.InsertBadgeAwardEvent(ctx, gen.InsertBadgeAwardEventParams{
					UserID:  userID,
					BadgeID: c.RewardBadgeID,
					Source:  "challenge_reward",
				}); err != nil {
					slog.Warn("gamification: insert award event failed",
						"challenge_id", uuidStr(c.ID), "error", err)
				}
				rewardGranted = true
			}
		}
	}

	if c.RewardPoints > 0 && s.pointsAwarder != nil {
		// Enqueue the retryable wallet:award_points task — the deterministic
		// idempotency key makes retried or concurrent completions safe and a
		// transient failure retries instead of losing the reward (PTS-006).
		reason := "challenge_completed:" + uuidStr(c.ID)
		if err := s.pointsAwarder.EnqueuePointsAward(ctx, userID, int(c.RewardPoints), reason, uuidStr(c.ID)); err != nil {
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
		if err := s.notifier.Notify(ctx, userID, NotifTypeChallengeCompleted,
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
func (s *Service) EvaluatePointsBadges(ctx context.Context, userID pgtype.UUID, currentLifetimePoints int64) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx) {
		return nil, nil
	}

	badges, err := s.store.GetBadgesByPointsTrigger(ctx)
	if err != nil {
		return nil, fmt.Errorf("gamification: get points badges: %w", err)
	}

	awarded := make([]BadgeWithDetails, 0)

	for _, b := range badges {
		if b.TriggerValue == nil {
			continue
		}

		if err := s.store.UpsertBadgeProgress(ctx, userID, b.ID, int32(min64(currentLifetimePoints, int64(*b.TriggerValue)))); err != nil {
			slog.Warn("gamification: upsert badge progress failed",
				"badge_id", uuidStr(b.ID), "error", err)
		}

		threshold := int64(*b.TriggerValue)
		if currentLifetimePoints < threshold {
			continue
		}

		if d, ok := s.awardIfNew(ctx, userID, b, TriggerTypeLifetimePoints); ok {
			awarded = append(awarded, d)
		}
	}

	return awarded, nil
}
