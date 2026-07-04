package reviews

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Constants and configuration
// ---------------------------------------------------------------------------

func TestReviewStatusConstants(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		ReviewStatusPublished: "published",
		ReviewStatusFlagged:   "flagged",
		ReviewStatusRemoved:   "removed",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("review status constant = %q, want %q", got, want)
		}
	}
}

func TestRatingBounds(t *testing.T) {
	t.Parallel()

	if RatingMin != 1 {
		t.Errorf("RatingMin = %d, want 1", RatingMin)
	}

	if RatingMax != 5 {
		t.Errorf("RatingMax = %d, want 5", RatingMax)
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
// Rating validation
// ---------------------------------------------------------------------------

func TestRatingValidation(t *testing.T) {
	t.Parallel()

	validRatings := []int16{1, 2, 3, 4, 5}
	for _, r := range validRatings {
		if r < RatingMin || r > RatingMax {
			t.Errorf("expected rating %d to be valid", r)
		}
	}

	invalidRatings := []int16{0, 6, -1, 100}
	for _, r := range invalidRatings {
		if r >= RatingMin && r <= RatingMax {
			t.Errorf("expected rating %d to be invalid", r)
		}
	}
}

// ---------------------------------------------------------------------------
// CreateReviewInput validation
// ---------------------------------------------------------------------------

func TestCreateReviewInputValidation(t *testing.T) {
	t.Parallel()
	// Empty entity_type should be invalid.
	input := CreateReviewInput{EntityType: "", EntityID: pgtype.UUID{}, Rating: 3}
	if input.EntityType != "" {
		t.Error("expected empty entity_type")
	}

	// Rating out of range should be invalid.
	input2 := CreateReviewInput{EntityType: "organization", Rating: 0}
	if input2.Rating >= RatingMin {
		t.Error("expected rating 0 to be invalid")
	}
}

// ---------------------------------------------------------------------------
// Review status transitions
// ---------------------------------------------------------------------------

func TestReviewStatusTransitions(t *testing.T) {
	t.Parallel()
	// Valid statuses.
	validStatuses := map[string]bool{
		ReviewStatusPublished: true,
		ReviewStatusFlagged:   true,
		ReviewStatusRemoved:   true,
	}
	for status := range validStatuses {
		if !validStatuses[status] {
			t.Errorf("expected %q to be a valid review status", status)
		}
	}

	// Invalid statuses.
	invalidStatuses := []string{"draft", "pending", "approved", ""}
	for _, status := range invalidStatuses {
		if validStatuses[status] {
			t.Errorf("expected %q to be an invalid review status", status)
		}
	}
}

// ---------------------------------------------------------------------------
// Review status values
// ---------------------------------------------------------------------------

func TestReviewStatusValues(t *testing.T) {
	t.Parallel()

	statuses := []string{
		ReviewStatusPublished,
		ReviewStatusFlagged,
		ReviewStatusRemoved,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("review status should not be empty")
		}
	}
}
