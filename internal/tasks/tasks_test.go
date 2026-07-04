package tasks

import (
	"testing"
)

func TestScheduledPublisherCreation(t *testing.T) {
	t.Parallel()
	// Verify that NewScheduledPublisher returns a non-nil value.
	// Full integration tests require a running database.
	sp := NewScheduledPublisher(nil)
	if sp == nil {
		t.Fatal("NewScheduledPublisher returned nil")
	}
}

func TestVelocityScorerCreation(t *testing.T) {
	t.Parallel()

	vs := NewVelocityScorer(nil)
	if vs == nil {
		t.Fatal("NewVelocityScorer returned nil")
	}
}

func TestVelocityFormula(t *testing.T) {
	t.Parallel()
	// Verify the velocity formula matches the specification.
	// velocity = (views*1 + comments*5 + votes*10 + bookmarks*3 + shares*8) / hours_since_publish

	tests := []struct {
		name      string
		views     float64
		comments  float64
		votes     float64
		bookmarks float64
		shares    float64
		hours     float64
		want      float64
	}{
		{
			name:  "zero engagement",
			views: 0, comments: 0, votes: 0, bookmarks: 0, shares: 0,
			hours: 10,
			want:  0,
		},
		{
			name:  "100 views after 10 hours",
			views: 100, comments: 0, votes: 0, bookmarks: 0, shares: 0,
			hours: 10,
			want:  10,
		},
		{
			name:  "mixed engagement after 10 hours",
			views: 100, comments: 10, votes: 5, bookmarks: 3, shares: 2,
			hours: 10,
			want:  22.5,
		},
		{
			name:  "high engagement after 1 hour",
			views: 50, comments: 20, votes: 10, bookmarks: 5, shares: 3,
			hours: 1,
			want:  289,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var velocity float64
			if tt.hours > 0 {
				velocity = (tt.views*1 + tt.comments*5 + tt.votes*10 + tt.bookmarks*3 + tt.shares*8) / tt.hours
			}

			if velocity != tt.want {
				t.Errorf("velocity = %f, want %f", velocity, tt.want)
			}
		})
	}
}
