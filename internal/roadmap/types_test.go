package roadmap

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Constants and configuration
// ---------------------------------------------------------------------------

func TestItemStatusConstants(t *testing.T) {
	t.Parallel()

	expected := map[ItemStatus]string{
		StatusPending:     "pending",
		StatusUnderReview: "under_review",
		StatusPlanned:     "planned",
		StatusInProgress:  "in_progress",
		StatusTesting:     "testing",
		StatusShipped:     "shipped",
		StatusDeclined:    "declined",
		StatusArchived:    "archived",
	}
	for got, want := range expected {
		if string(got) != want {
			t.Errorf("status constant = %q, want %q", got, want)
		}
	}
}

func TestPublicStatuses(t *testing.T) {
	t.Parallel()

	want := []string{"under_review", "planned", "in_progress", "testing", "shipped"}
	if len(PublicStatuses) != len(want) {
		t.Fatalf("PublicStatuses has %d entries, want %d", len(PublicStatuses), len(want))
	}

	for i, s := range want {
		if PublicStatuses[i] != s {
			t.Errorf("PublicStatuses[%d] = %q, want %q", i, PublicStatuses[i], s)
		}
	}
}

func TestVotableStatuses(t *testing.T) {
	t.Parallel()

	want := []string{"under_review", "planned", "in_progress", "testing"}
	if len(VotableStatuses) != len(want) {
		t.Fatalf("VotableStatuses has %d entries, want %d", len(VotableStatuses), len(want))
	}

	for i, s := range want {
		if VotableStatuses[i] != s {
			t.Errorf("VotableStatuses[%d] = %q, want %q", i, VotableStatuses[i], s)
		}
	}
}

func TestIsVotableStatus(t *testing.T) {
	t.Parallel()

	votable := []ItemStatus{StatusUnderReview, StatusPlanned, StatusInProgress, StatusTesting}
	for _, s := range votable {
		if !IsVotableStatus(s) {
			t.Errorf("expected status %q to be votable", s)
		}
	}

	notVotable := []ItemStatus{StatusPending, StatusShipped, StatusDeclined, StatusArchived}
	for _, s := range notVotable {
		if IsVotableStatus(s) {
			t.Errorf("expected status %q to NOT be votable", s)
		}
	}
}

func TestIsPublicStatus(t *testing.T) {
	t.Parallel()

	public := []ItemStatus{StatusUnderReview, StatusPlanned, StatusInProgress, StatusTesting, StatusShipped}
	for _, s := range public {
		if !IsPublicStatus(s) {
			t.Errorf("expected status %q to be public", s)
		}
	}

	hidden := []ItemStatus{StatusPending, StatusDeclined, StatusArchived}
	for _, s := range hidden {
		if IsPublicStatus(s) {
			t.Errorf("expected status %q to be hidden from public routes", s)
		}
	}
}

func TestPaginationDefaults(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

// ---------------------------------------------------------------------------
// Status state machine — Ref: §113 (Status Lifecycle)
// ---------------------------------------------------------------------------

func TestIsValidTransition(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		from ItemStatus
		to   ItemStatus
	}{
		{StatusPending, StatusUnderReview},
		{StatusPending, StatusDeclined},
		{StatusUnderReview, StatusPlanned},
		{StatusUnderReview, StatusDeclined},
		{StatusUnderReview, StatusArchived},
		{StatusUnderReview, StatusTesting},
		{StatusPlanned, StatusInProgress},
		{StatusPlanned, StatusUnderReview},
		{StatusPlanned, StatusDeclined},
		{StatusPlanned, StatusArchived},
		{StatusPlanned, StatusTesting},
		{StatusInProgress, StatusShipped},
		{StatusInProgress, StatusPlanned},
		{StatusInProgress, StatusArchived},
		{StatusInProgress, StatusTesting},
		{StatusTesting, StatusShipped},
		{StatusTesting, StatusInProgress},
		{StatusTesting, StatusArchived},
		{StatusShipped, StatusArchived},
		{StatusDeclined, StatusUnderReview},
		{StatusDeclined, StatusArchived},
		// No-op transitions are permitted for idempotent PATCHes.
		{StatusPlanned, StatusPlanned},
		{StatusTesting, StatusTesting},
		{StatusShipped, StatusShipped},
	}

	for _, tc := range validCases {
		if !IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected transition %s → %s to be valid", tc.from, tc.to)
		}
	}

	invalidCases := []struct {
		from ItemStatus
		to   ItemStatus
	}{
		{StatusPending, StatusPlanned},
		{StatusPending, StatusInProgress},
		{StatusPending, StatusTesting},
		{StatusPending, StatusShipped},
		{StatusPending, StatusArchived},
		{StatusUnderReview, StatusInProgress},
		{StatusUnderReview, StatusShipped},
		{StatusPlanned, StatusShipped},
		{StatusInProgress, StatusUnderReview},
		{StatusInProgress, StatusDeclined},
		{StatusTesting, StatusUnderReview},
		{StatusTesting, StatusPlanned},
		{StatusTesting, StatusDeclined},
		{StatusShipped, StatusPlanned},
		{StatusShipped, StatusInProgress},
		{StatusShipped, StatusTesting},
		{StatusDeclined, StatusTesting},
		{StatusArchived, StatusUnderReview},
		{StatusArchived, StatusPlanned},
		{StatusArchived, StatusTesting},
		{StatusArchived, StatusShipped},
	}

	for _, tc := range invalidCases {
		if IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected transition %s → %s to be invalid", tc.from, tc.to)
		}
	}
}

func TestValidStatus(t *testing.T) {
	t.Parallel()

	for _, s := range AllStatuses() {
		if !ValidStatus(string(s)) {
			t.Errorf("expected %q to be a valid status", s)
		}
	}

	if ValidStatus("bogus") {
		t.Error("expected \"bogus\" to be an invalid status")
	}

	if !ValidStatus("testing") {
		t.Error("expected \"testing\" to be a valid status")
	}

	if len(AllStatuses()) != 8 {
		t.Errorf("AllStatuses() returned %d items, want 8", len(AllStatuses()))
	}
}
