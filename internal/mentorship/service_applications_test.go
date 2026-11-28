// service_applications_test.go — coverage for the §102 2026-11-28 member
// mentor-application flow: input validation is unit-tested; the apply →
// approve → group grant → revoke state machine is live-DB covered
// (skipped unless JC_DATABASE_URL is set).
package mentorship

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestValidateApplicationInput(t *testing.T) {
	t.Run("trims and keeps motivation", func(t *testing.T) {
		m, areas, err := validateApplicationInput("  I can help with visas  ", []string{" visas ", "housing"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m != "I can help with visas" {
			t.Fatalf("motivation not trimmed: %q", m)
		}
		if len(areas) != 2 || areas[0] != "visas" {
			t.Fatalf("areas not normalized: %v", areas)
		}
	})

	t.Run("empty motivation rejected", func(t *testing.T) {
		if _, _, err := validateApplicationInput("   ", nil); !errors.Is(err, ErrInvalidMotivation) {
			t.Fatalf("expected ErrInvalidMotivation, got %v", err)
		}
	})

	t.Run("over-long motivation rejected", func(t *testing.T) {
		if _, _, err := validateApplicationInput(strings.Repeat("x", applicationMotivationMax+1), nil); !errors.Is(err, ErrInvalidMotivation) {
			t.Fatalf("expected ErrInvalidMotivation, got %v", err)
		}
	})

	t.Run("too many areas rejected", func(t *testing.T) {
		areas := make([]string, applicationAreasMax+1)
		for i := range areas {
			areas[i] = "a" + strings.Repeat("b", i+1)
		}
		if _, _, err := validateApplicationInput("ok", areas); !errors.Is(err, ErrInvalidAreas) {
			t.Fatalf("expected ErrInvalidAreas, got %v", err)
		}
	})

	t.Run("over-long area rejected", func(t *testing.T) {
		if _, _, err := validateApplicationInput("ok", []string{strings.Repeat("x", applicationAreaMaxLen+1)}); !errors.Is(err, ErrInvalidAreas) {
			t.Fatalf("expected ErrInvalidAreas, got %v", err)
		}
	})

	t.Run("duplicate areas dedupe case-insensitively", func(t *testing.T) {
		_, areas, err := validateApplicationInput("ok", []string{"Visa", "visa", "VISA"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(areas) != 1 {
			t.Fatalf("expected 1 deduped area, got %v", areas)
		}
	})
}

// fakePermChecker grants become_mentor only to users in the set — mirrors
// *rbac.RBAC for the eligibility probe.
type fakePermChecker map[pgtype.UUID]bool

func (f fakePermChecker) HasPermission(_ context.Context, userID pgtype.UUID, permKey string) bool {
	return permKey == "mentorship.become_mentor" && f[userID]
}

// inMemoryGroups records group mutations — enough to assert approve adds
// and decline(revoke) removes without a live rbac.Store.
type inMemoryGroups struct {
	members map[pgtype.UUID]bool
}

func (g *inMemoryGroups) AddUserToGroup(_ context.Context, userID, _ pgtype.UUID) error {
	g.members[userID] = true
	return nil
}

func (g *inMemoryGroups) RemoveUserFromGroup(_ context.Context, userID, _ pgtype.UUID) error {
	delete(g.members, userID)
	return nil
}

func TestApplicationFlow_LiveDB(t *testing.T) {
	database := liveDB(t)
	ctx := t.Context()

	svc := NewService(database)
	// mentorsGroupID resolves the seeded 'mentors' group — skip cleanly on
	// a DB whose migrations predate 001239.
	if _, err := svc.mentorsGroupID(ctx); err != nil {
		t.Skipf("mentors group not seeded (migration 001239?): %v", err)
	}

	eligible := fakePermChecker{}
	groups := &inMemoryGroups{members: map[pgtype.UUID]bool{}}
	svc.SetPermChecker(eligible)
	svc.SetGroupMembership(groups)

	var userID pgtype.UUID
	if err := database.Writer.QueryRow(ctx,
		`INSERT INTO users (email, username, profile_type) VALUES ($1, $2, 'regular') RETURNING id`,
		"applicant@svc.test", "applicant_"+strings.ReplaceAll(t.Name(), "/", "_")).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Writer.Exec(context.Background(),
			`DELETE FROM mentor_applications WHERE user_id = $1`, userID)
		_, _ = database.Writer.Exec(context.Background(),
			`DELETE FROM user_group_memberships WHERE user_id = $1`, userID)
		_, _ = database.Writer.Exec(context.Background(),
			`DELETE FROM users WHERE id = $1`, userID)
	})

	// Submit → pending row visible on the self probe.
	app, err := svc.ApplyForMentor(ctx, userID, "I mentor diaspora newcomers on UK visas", []string{"uk visa"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if app.Status != "pending" {
		t.Fatalf("expected pending, got %q", app.Status)
	}

	// Duplicate while pending → 409 arm.
	if _, err := svc.ApplyForMentor(ctx, userID, "again", nil); !errors.Is(err, ErrApplicationPending) {
		t.Fatalf("expected ErrApplicationPending, got %v", err)
	}

	// List reflects the pending row for the admin queue.
	apps, total, err := svc.ListApplications(ctx, "pending", "", 20, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 1 || len(apps) < 1 {
		t.Fatalf("expected at least the seeded pending application, got total=%d", total)
	}

	// Approve → approved row + mentors membership recorded by the seam.
	var appID pgtype.UUID
	if err := appID.Scan(app.ID); err != nil {
		t.Fatalf("scan app id: %v", err)
	}
	approved, err := svc.ApproveApplication(ctx, appID, userID)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != "approved" {
		t.Fatalf("expected approved, got %q", approved.Status)
	}
	if !groups.members[userID] {
		t.Fatal("approve did not grant the mentors group")
	}

	// Approved + group present ⇒ eligible ⇒ self-apply is the 409 arm.
	eligible[userID] = true
	if _, err := svc.ApplyForMentor(ctx, userID, "again", nil); !errors.Is(err, ErrAlreadyEligible) {
		t.Fatalf("expected ErrAlreadyEligible, got %v", err)
	}

	// Decline on the approved row = revoke: status declined + membership
	// removed (which fires the mentor-retire sweep in production).
	declined, err := svc.DeclineApplication(ctx, appID, userID, "insufficient experience")
	if err != nil {
		t.Fatalf("decline: %v", err)
	}
	if declined.Status != "declined" || declined.DeclineReason != "insufficient experience" {
		t.Fatalf("expected declined+reason, got %+v", declined)
	}
	if groups.members[userID] {
		t.Fatal("revoke did not remove the mentors membership")
	}

	// Resubmit after decline → back to pending with reviewer fields reset.
	eligible[userID] = false
	resub, err := svc.ApplyForMentor(ctx, userID, "Reapplying with more context", nil)
	if err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if resub.Status != "pending" || resub.DeclineReason != "" || resub.ReviewerUserID != "" {
		t.Fatalf("expected clean pending resubmission, got %+v", resub)
	}
}
