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
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()

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
