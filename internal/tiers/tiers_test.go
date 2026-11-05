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

// TestSetUserTier_RejectsStaffTiers verifies staff tiers cannot be assigned
// through the subscription path — they are group-membership-only (001049).
func TestSetUserTier_RejectsStaffTiers(t *testing.T) {
	t.Parallel()

	s := &Service{store: &Store{}}

	for _, tier := range []string{"staff", "admin", "super_admin"} {
		err := s.SetUserTier(t.Context(), pgtype.UUID{}, tier)
		if err == nil {
			t.Fatalf("expected error for staff tier %q", tier)
		}

		if err.Error() != "invalid tier: "+tier {
			t.Fatalf("unexpected error for %q: %v", tier, err)
		}
	}
}

// TestTierNameForRank verifies rank → name mapping for both rank tables,
// including the regular fallback for unknown ranks.
func TestTierNameForRank(t *testing.T) {
	t.Parallel()

	cases := []struct {
		rank int32
		want string
	}{
		{1, "regular"},
		{2, "pro"},
		{3, "business"},
		{4, "staff"},
		{5, "admin"},
		{6, "super_admin"},
		{0, "regular"},
		{99, "regular"},
	}

	for _, tc := range cases {
		if got := tierNameForRank(effectiveTierRanks, tc.rank); got != tc.want {
			t.Errorf("effectiveTierRanks rank %d: got %q, want %q", tc.rank, got, tc.want)
		}
	}

	// Subscription map must never resolve a staff rank to a staff tier —
	// ranks 4–6 fall back to regular.
	for _, rank := range []int32{4, 5, 6} {
		if got := tierNameForRank(subscriptionTierRanks, rank); got != "regular" {
			t.Errorf("subscriptionTierRanks rank %d: got %q, want regular", rank, got)
		}
	}
}

// TestIsStaffTier exercises the staff-tier predicate used by quota fallbacks.
func TestIsStaffTier(t *testing.T) {
	t.Parallel()

	for _, tier := range []string{"staff", "admin", "super_admin"} {
		if !IsStaffTier(tier) {
			t.Errorf("IsStaffTier(%q) = false, want true", tier)
		}
	}

	for _, tier := range []string{"regular", "pro", "business", "guest", ""} {
		if IsStaffTier(tier) {
			t.Errorf("IsStaffTier(%q) = true, want false", tier)
		}
	}
}
