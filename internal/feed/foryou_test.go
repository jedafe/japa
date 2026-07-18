package feed

import (
	"testing"
	"time"
)

func TestComputeForYouScore_AffinityBoostsRank(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	base := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 10}

	followed := base
	followed.IsAuthorFollowed = true

	// Same velocity + recency; the followed-author candidate must score higher.
	if computeForYouScore(followed, now, 10) <= computeForYouScore(base, now, 10) {
		t.Fatal("followed-author candidate should outscore an identical non-followed one")
	}
}

func TestRecencyDecay_FresherWins(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	fresh := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 5}
	old := Candidate{CreatedAt: now.Add(-240 * time.Hour), VelocityScore: 5}

	if computeForYouScore(fresh, now, 5) <= computeForYouScore(old, now, 5) {
		t.Fatal("fresher candidate should outscore an older one at equal velocity")
	}
}

func TestRankCandidates_SortModes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	a := Candidate{Item: FeedItem{ID: "a"}, CreatedAt: now.Add(-1 * time.Hour), VoteScore: 1, VelocityScore: 9}
	b := Candidate{Item: FeedItem{ID: "b"}, CreatedAt: now.Add(-5 * time.Hour), VoteScore: 9, VelocityScore: 1}

	if got := rankCandidates([]Candidate{a, b}, SortLatest, now); got[0].Item.ID != "a" {
		t.Errorf("latest: want a first, got %s", got[0].Item.ID)
	}
	if got := rankCandidates([]Candidate{a, b}, SortTop, now); got[0].Item.ID != "b" {
		t.Errorf("top: want b first, got %s", got[0].Item.ID)
	}
	if got := rankCandidates([]Candidate{a, b}, SortRising, now); got[0].Item.ID != "a" {
		t.Errorf("rising: want a first, got %s", got[0].Item.ID)
	}
}
