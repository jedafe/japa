//go:build integration

package gamification

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/testdb"
)

func mustScanUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}
	return id
}

type stubPointsAwarder struct {
	calls  int
	points int
	reason string
}

func (s *stubPointsAwarder) AwardPoints(_ context.Context, _, _ pgtype.UUID, points int, reason string) error {
	s.calls++
	s.points = points
	s.reason = reason
	return nil
}

type stubNotifier struct {
	calls  int
	nTypes []string
}

func (s *stubNotifier) Notify(_ context.Context, _, _ pgtype.UUID, nType, _, _, _ string) error {
	s.calls++
	s.nTypes = append(s.nTypes, nType)
	return nil
}

// TestChallengeCompletionGrantsReward covers the §64 challenge reward path:
// on the first transition to completed the service must award the reward
// badge, credit reward_points through the wallet awarder, mark the reward
// claimed, and emit the completion notification. A second event for an
// already-completed challenge must not re-grant anything.
func TestChallengeCompletionGrantsReward(t *testing.T) {
	t.Parallel()

	d := testdb.Connect(t)
	store := NewStore(d)
	svc := NewService(store, nil) // nil settings → all gamification toggles on

	awarder := &stubPointsAwarder{}
	notifier := &stubNotifier{}
	svc.SetPointsAwarder(awarder)
	svc.SetNotifier(notifier)

	ctx := context.Background()
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})

	badge, err := svc.CreateBadge(ctx, tenantID, CreateBadgeInput{
		Slug:        "reward_badge",
		Name:        "Reward Badge",
		TriggerType: TriggerTypeManual,
	})
	if err != nil {
		t.Fatalf("create badge: %v", err)
	}

	ch, err := svc.CreateChallenge(ctx, tenantID, CreateChallengeInput{
		Slug:          "first_poster",
		Name:          "First Poster",
		ActivityKey:   "content.created",
		TargetCount:   1,
		RewardPoints:  25,
		RewardBadgeID: &badge.ID,
		StartsAt:      time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}

	// First matching activity completes the challenge.
	if _, err := svc.EvaluateBadges(ctx, tenantID, userID, "content.created"); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	progress, err := store.GetChallengeProgress(ctx, tenantID, userID, mustScanUUID(t, ch.ID))
	if err != nil {
		t.Fatalf("get progress: %v", err)
	}
	if !progress.Completed {
		t.Fatal("expected challenge to be completed")
	}
	if !progress.CompletedAt.Valid {
		t.Fatal("expected completed_at to be set")
	}
	if !progress.RewardClaimed {
		t.Fatal("expected reward_claimed to be set")
	}

	if awarder.calls != 1 || awarder.points != 25 {
		t.Fatalf("expected 1 AwardPoints call for 25 points, got calls=%d points=%d", awarder.calls, awarder.points)
	}

	if _, err := store.GetUserBadge(ctx, tenantID, userID, mustScanUUID(t, badge.ID)); err != nil {
		t.Fatalf("expected reward badge to be awarded: %v", err)
	}

	if notifier.calls != 1 || len(notifier.nTypes) != 1 || notifier.nTypes[0] != NotifTypeChallengeCompleted {
		t.Fatalf("expected 1 challenge_completed notification, got %v", notifier.nTypes)
	}

	firstCompletedAt := progress.CompletedAt

	// A second matching event must not re-grant rewards or drift
	// completed_at.
	if _, err := svc.EvaluateBadges(ctx, tenantID, userID, "content.created"); err != nil {
		t.Fatalf("second evaluate: %v", err)
	}

	progress2, err := store.GetChallengeProgress(ctx, tenantID, userID, mustScanUUID(t, ch.ID))
	if err != nil {
		t.Fatalf("get progress after second event: %v", err)
	}
	if !progress2.CompletedAt.Time.Equal(firstCompletedAt.Time) {
		t.Fatalf("completed_at drifted: %v → %v", firstCompletedAt.Time, progress2.CompletedAt.Time)
	}
	if awarder.calls != 1 {
		t.Fatalf("expected no additional AwardPoints call, got %d", awarder.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected no additional notification, got %d", notifier.calls)
	}
}
