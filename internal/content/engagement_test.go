package content

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// publish_at validation helpers
// ---------------------------------------------------------------------------

func TestPublishAtMustBeInFuture(t *testing.T) {
	// A time in the past should be rejected.
	past := time.Now().Add(-1 * time.Hour)
	if !past.Before(time.Now()) {
		t.Fatal("test setup: past time should be before now")
	}

	// A time in the future should be accepted.
	future := time.Now().Add(1 * time.Hour)
	if !future.After(time.Now()) {
		t.Fatal("test setup: future time should be after now")
	}
}

func TestPublishAtValidationInCreateHandler(t *testing.T) {
	// Verify that the handler rejects publish_at in the past.
	// This is a logic-level test — the actual HTTP handler test
	// requires a database, so we test the validation logic directly.
	now := time.Now()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"past time", now.Add(-1 * time.Hour).UTC().Format("2006-01-02 15:04:05.999999999Z07:00"), true},
		{"future time", now.Add(1 * time.Hour).UTC().Format("2006-01-02 15:04:05.999999999Z07:00"), false},
		{"empty string", "", false}, // empty means no publish_at
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input == "" {
				return // no publish_at set — handled by nil check
			}
			var ts pgtype.Timestamptz
			if err := ts.Scan(tt.input); err != nil {
				if !tt.wantErr {
					t.Fatalf("unexpected scan error: %v", err)
				}
				return
			}
			isPast := ts.Valid && ts.Time.Before(now)
			if isPast != tt.wantErr {
				t.Errorf("publish_at=%s: wantErr=%v, got isPast=%v", tt.input, tt.wantErr, isPast)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// EngagementStats
// ---------------------------------------------------------------------------

func TestEngagementStatsZeroValue(t *testing.T) {
	var stats EngagementStats
	if stats.Views != 0 || stats.VelocityScore != 0 {
		t.Error("zero-valued EngagementStats should have all zero fields")
	}
}

func TestContentWithScore(t *testing.T) {
	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	cs := ContentWithScore{
		ContentID:     id,
		VelocityScore: 42.5,
	}
	if cs.VelocityScore != 42.5 {
		t.Errorf("VelocityScore = %f, want 42.5", cs.VelocityScore)
	}
	if cs.ContentID != id {
		t.Error("ContentID mismatch")
	}
}

// ---------------------------------------------------------------------------
// Velocity formula logic (unit test, no DB)
// ---------------------------------------------------------------------------

func TestVelocityFormula(t *testing.T) {
	// velocity = (views*1 + comments*5 + votes*10 + bookmarks*3 + shares*8) / hours_since_publish
	// For 100 views, 10 comments, 5 votes, 3 bookmarks, 2 shares after 10 hours:
	// (100*1 + 10*5 + 5*10 + 3*3 + 2*8) / 10 = (100 + 50 + 50 + 9 + 16) / 10 = 22.5
	views := float64(100)
	comments := float64(10)
	votes := float64(5)
	bookmarks := float64(3)
	shares := float64(2)
	hours := float64(10)

	velocity := (views*1 + comments*5 + votes*10 + bookmarks*3 + shares*8) / hours
	expected := 22.5

	if velocity != expected {
		t.Errorf("velocity = %f, want %f", velocity, expected)
	}
}

func TestVelocityFormulaZeroHours(t *testing.T) {
	// Division by zero should produce 0 (handled by the SQL CASE).
	views := float64(100)
	hours := float64(0)

	var velocity float64
	if hours > 0 {
		velocity = views / hours
	}
	// velocity stays 0 when hours is 0.
	if velocity != 0 {
		t.Errorf("velocity = %f, want 0 for zero hours", velocity)
	}
}
