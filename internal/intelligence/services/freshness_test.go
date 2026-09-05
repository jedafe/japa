package services

import (
	"testing"
	"time"
)

func TestFreshnessScore(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		verifiedAt time.Time
		want       int
	}{
		{"now -> 100", now, 100},
		{"1 day ago -> 100", now.Add(-24 * time.Hour), 100},
		{"3 days ago -> 90", now.Add(-3 * 24 * time.Hour), 90},
		{"10 days ago -> 70", now.Add(-10 * 24 * time.Hour), 70},
		{"45 days ago -> 40", now.Add(-45 * 24 * time.Hour), 40},
		{"100 days ago -> 0", now.Add(-100 * 24 * time.Hour), 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := FreshnessScore(tc.verifiedAt, now)
			if got != tc.want {
				t.Fatalf("FreshnessScore(%v, %v) = %d, want %d", tc.verifiedAt, now, got, tc.want)
			}
		})
	}
}
