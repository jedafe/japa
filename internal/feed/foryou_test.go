package feed

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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

func TestForYouCacheKey_IncludesSort(t *testing.T) {
	t.Parallel()

	var tid, uid pgtype.UUID
	_ = tid.Scan("11111111-1111-1111-1111-111111111111")
	_ = uid.Scan("22222222-2222-2222-2222-222222222222")

	relevant := forYouCacheKey(tid, uid, SortRelevant)
	latest := forYouCacheKey(tid, uid, SortLatest)

	if relevant == latest {
		t.Fatal("cache key must differ per sort mode")
	}
	if want := "feed:foryou:11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222:relevant"; relevant != want {
		t.Errorf("key = %q, want %q", relevant, want)
	}
}

func TestPageSlice_Bounds(t *testing.T) {
	t.Parallel()

	items := []FeedItem{{ID: "a"}, {ID: "b"}, {ID: "c"}}

	if got := pageSlice(items, 1, 1); len(got) != 1 || got[0].ID != "b" {
		t.Errorf("offset 1 limit 1: got %+v", got)
	}
	if got := pageSlice(items, 5, 10); len(got) != 0 {
		t.Errorf("offset past end must be empty, got %+v", got)
	}
	if got := pageSlice(items, 2, 10); len(got) != 1 || got[0].ID != "c" {
		t.Errorf("limit past end must clamp, got %+v", got)
	}
}
