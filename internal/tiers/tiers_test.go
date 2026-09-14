package tiers

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestSetUserTier_InvalidTier verifies that an unrecognised tier is rejected
// before any store call is made.
func TestSetUserTier_InvalidTier(t *testing.T) {
	t.Parallel()

	s := &Service{store: &Store{}}

	err := s.SetUserTier(t.Context(), pgtype.UUID{}, "premium")
	if err == nil {
		t.Fatalf("expected error for invalid tier")
	}

	if err.Error() != "invalid tier: premium" {
		t.Fatalf("unexpected error: %v", err)
	}
}
