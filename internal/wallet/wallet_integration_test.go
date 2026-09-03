//go:build integration

package wallet

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/testdb"
)

func mustUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(uuid.New().String()); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}

func TestOneTimeActivityRule(t *testing.T) {
	t.Parallel()

	db := testdb.Connect(t)
	s := NewService(db)
	ctx := context.Background()
	tenantID := testdb.NewTenant(t, db)
	userID := testdb.SeedUser(t, db, tenantID, testdb.UserOpts{})

	// Create a one-time rule.
	rule, err := s.CreateOrUpdateActivityRule(ctx, tenantID, CreateOrUpdateActivityRuleInput{
		ActivityKey: "content.published",
		PointValue:  10,
		IsEnabled:   true,
		OneTime:     true,
	})
	if err != nil {
		t.Fatalf("creating one-time rule: %v", err)
	}
	if !rule.OneTime {
		t.Fatalf("expected rule to be one_time, got %v", rule.OneTime)
	}

	// First activity event should award points.
	event1, err := s.EmitActivityEvent(ctx, tenantID, userID, EmitActivityEventInput{
		ActivityKey: "content.published",
		Reference:   "post-1",
		Payload:     map[string]any{"post_id": "post-1"},
	})
	if err != nil {
		t.Fatalf("emitting first event: %v", err)
	}
	if err := s.ProcessActivityEvent(ctx, event1.ID, tenantID); err != nil {
		t.Fatalf("processing first event: %v", err)
	}

	balance1, err := s.GetPointsBalance(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("getting balance after first event: %v", err)
	}
	if balance1 != 10 {
		t.Fatalf("expected balance 10 after first event, got %d", balance1)
	}

	// Second activity event for the same user with a different reference
	// should be skipped because the rule is one-time.
	event2, err := s.EmitActivityEvent(ctx, tenantID, userID, EmitActivityEventInput{
		ActivityKey: "content.published",
		Reference:   "post-2",
		Payload:     map[string]any{"post_id": "post-2"},
	})
	if err != nil {
		t.Fatalf("emitting second event: %v", err)
	}
	if err := s.ProcessActivityEvent(ctx, event2.ID, tenantID); err != nil {
		t.Fatalf("processing second event: %v", err)
	}

	balance2, err := s.GetPointsBalance(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("getting balance after second event: %v", err)
	}
	if balance2 != 10 {
		t.Fatalf("expected balance to remain 10 after one-time rule, got %d", balance2)
	}

	// The second event should be marked processed; the balance staying at 10
	// is the observable proof the one-time rule was enforced.
}

func TestAwardPointsOneTimeRule(t *testing.T) {
	t.Parallel()

	db := testdb.Connect(t)
	s := NewService(db)
	ctx := context.Background()
	tenantID := testdb.NewTenant(t, db)
	userID := testdb.SeedUser(t, db, tenantID, testdb.UserOpts{})

	// Configure a one-time rule for the direct award reason.
	if _, err := s.CreateOrUpdateActivityRule(ctx, tenantID, CreateOrUpdateActivityRuleInput{
		ActivityKey: "scholarship_saved",
		PointValue:  25,
		IsEnabled:   true,
		OneTime:     true,
	}); err != nil {
		t.Fatalf("creating one-time rule: %v", err)
	}

	payload := func(ref string) []byte {
		p := AwardPointsPayload{
			TenantID:    tenantID.String(),
			UserID:      userID.String(),
			Points:      25,
			Reason:      "scholarship_saved",
			ReferenceID: ref,
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshaling payload: %v", err)
		}
		return b
	}

	if err := s.AwardPointsTask(ctx, payload("scholarship-1")); err != nil {
		t.Fatalf("first award: %v", err)
	}

	balance1, err := s.GetPointsBalance(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("getting balance after first award: %v", err)
	}
	if balance1 != 25 {
		t.Fatalf("expected balance 25 after first award, got %d", balance1)
	}

	// Second award with a different reference should be skipped.
	if err := s.AwardPointsTask(ctx, payload("scholarship-2")); err != nil {
		t.Fatalf("second award: %v", err)
	}

	balance2, err := s.GetPointsBalance(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("getting balance after second award: %v", err)
	}
	if balance2 != 25 {
		t.Fatalf("expected balance to remain 25 after one-time skip, got %d", balance2)
	}
}

func TestRepeatableActivityRuleCaps(t *testing.T) {
	t.Parallel()

	db := testdb.Connect(t)
	s := NewService(db)
	ctx := context.Background()
	tenantID := testdb.NewTenant(t, db)
	userID := testdb.SeedUser(t, db, tenantID, testdb.UserOpts{})

	capPerDay := int32(2)
	rule, err := s.CreateOrUpdateActivityRule(ctx, tenantID, CreateOrUpdateActivityRuleInput{
		ActivityKey: "comment.created",
		PointValue:  5,
		IsEnabled:   true,
		OneTime:     false,
		CapPerDay:   &capPerDay,
	})
	if err != nil {
		t.Fatalf("creating repeatable rule: %v", err)
	}
	if rule.OneTime {
		t.Fatalf("expected repeatable rule, got one_time")
	}

	for i := 1; i <= 3; i++ {
		event, err := s.EmitActivityEvent(ctx, tenantID, userID, EmitActivityEventInput{
			ActivityKey: "comment.created",
			Reference:   uuid.New().String(),
			Payload:     map[string]any{"comment_id": i},
		})
		if err != nil {
			t.Fatalf("emitting event %d: %v", i, err)
		}
		if err := s.ProcessActivityEvent(ctx, event.ID, tenantID); err != nil {
			t.Fatalf("processing event %d: %v", i, err)
		}
	}

	balance, err := s.GetPointsBalance(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("getting balance: %v", err)
	}
	if balance != 10 {
		t.Fatalf("expected balance 10 after cap of 2 x 5 points, got %d", balance)
	}
}
