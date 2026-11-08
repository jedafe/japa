package feed

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/visitor"
)

func TestComputeForYouScore_AffinityBoostsRank(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	base := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 10}

	followed := base
	followed.IsAuthorFollowed = true

	// Same velocity + recency; the followed-author candidate must score higher.
	if computeForYouScore(followed, now, 10, 0, nil, nil, defaultRankingWeights()) <= computeForYouScore(base, now, 10, 0, nil, nil, defaultRankingWeights()) {
		t.Fatal("followed-author candidate should outscore an identical non-followed one")
	}
}

func TestRecencyDecay_FresherWins(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	fresh := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 5}
	old := Candidate{CreatedAt: now.Add(-240 * time.Hour), VelocityScore: 5}

	if computeForYouScore(fresh, now, 5, 0, nil, nil, defaultRankingWeights()) <= computeForYouScore(old, now, 5, 0, nil, nil, defaultRankingWeights()) {
		t.Fatal("fresher candidate should outscore an older one at equal velocity")
	}
}

func TestComputeForYouScore_AuthorPointsBoost(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	base := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 10}

	reputed := base
	reputed.AuthorPoints = 5000

	// Same velocity + recency; the high-points author's candidate must score
	// higher (pool max = the 5000-point author).
	if computeForYouScore(reputed, now, 10, 5000, nil, nil, defaultRankingWeights()) <= computeForYouScore(base, now, 10, 5000, nil, nil, defaultRankingWeights()) {
		t.Fatal("high-points author should outscore an identical zero-points one")
	}
}

func TestComputeForYouScore_AuthorPointsLogScale(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	w := defaultRankingWeights()

	// A whale dominating the pool (1e6) should not flatten a mid-tier author
	// to ~0 — log normalization keeps mid balances meaningful.
	mid := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 10, AuthorPoints: 100}
	zero := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 10}

	midScore := computeForYouScore(mid, now, 10, 1_000_000, nil, nil, w)
	zeroScore := computeForYouScore(zero, now, 10, 1_000_000, nil, nil, w)

	if midScore <= zeroScore {
		t.Fatal("mid-tier author should still outscore a zero-points author under a whale-dominated pool")
	}

	// Linear normalization would give rep = 100/1e6 ≈ 0.0001; log gives
	// ln(101)/ln(1e6+1) ≈ 0.33 — assert the signal is well above linear noise.
	if midScore-zeroScore < w.AuthorPoints*0.2 {
		t.Fatalf("points contribution too small for log scale: delta=%f, want >= %f", midScore-zeroScore, w.AuthorPoints*0.2)
	}
}

func TestComputeForYouScore_AuthorPointsWeightZeroDisables(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	w := defaultRankingWeights()
	w.AuthorPoints = 0

	reputed := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 10, AuthorPoints: 5000}
	zero := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 10}

	if got := computeForYouScore(reputed, now, 10, 5000, nil, nil, w); got != computeForYouScore(zero, now, 10, 5000, nil, nil, w) {
		t.Fatalf("weight 0 must neutralize the points signal: got %f for a 5000-point author vs identical zero-points author", got)
	}
}

func TestRankCandidates_RelevantPrefersReputedAuthor(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	low := Candidate{Item: FeedItem{ID: "low"}, CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 5}
	high := Candidate{Item: FeedItem{ID: "high"}, CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 5, AuthorPoints: 2000}

	got := rankCandidates([]Candidate{low, high}, SortRelevant, now, nil, nil, defaultRankingWeights())
	if got[0].Item.ID != "high" {
		t.Errorf("relevant: want high-points author first, got %s", got[0].Item.ID)
	}
}

func TestRankCandidates_SortModes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	a := Candidate{Item: FeedItem{ID: "a"}, CreatedAt: now.Add(-1 * time.Hour), ReactionCount: 1, VelocityScore: 9}
	b := Candidate{Item: FeedItem{ID: "b"}, CreatedAt: now.Add(-5 * time.Hour), ReactionCount: 9, VelocityScore: 1}

	if got := rankCandidates([]Candidate{a, b}, SortLatest, now, nil, nil, defaultRankingWeights()); got[0].Item.ID != "a" {
		t.Errorf("latest: want a first, got %s", got[0].Item.ID)
	}

	if got := rankCandidates([]Candidate{a, b}, SortTop, now, nil, nil, defaultRankingWeights()); got[0].Item.ID != "b" {
		t.Errorf("top: want b first, got %s", got[0].Item.ID)
	}

	if got := rankCandidates([]Candidate{a, b}, SortRising, now, nil, nil, defaultRankingWeights()); got[0].Item.ID != "a" {
		t.Errorf("rising: want a first, got %s", got[0].Item.ID)
	}
}

func TestForYouCacheKey_IncludesSort(t *testing.T) {
	t.Parallel()

	var tid, uid pgtype.UUID
	_ = tid.Scan("11111111-1111-1111-1111-111111111111")
	_ = uid.Scan("22222222-2222-2222-2222-222222222222")

	vc := &visitor.VisitorContext{ContentCountry: "NG", ContentRegion: "Africa"}
	relevant := forYouCacheKey(uid, SortRelevant, vc)
	latest := forYouCacheKey(uid, SortLatest, vc)

	if relevant == latest {
		t.Fatal("cache key must differ per sort mode")
	}
}

func TestForYouCacheKey_IncludesContentCountry(t *testing.T) {
	t.Parallel()

	var tid, uid pgtype.UUID
	_ = tid.Scan("11111111-1111-1111-1111-111111111111")
	_ = uid.Scan("22222222-2222-2222-2222-222222222222")

	ng := forYouCacheKey(uid, SortRelevant, &visitor.VisitorContext{ContentCountry: "NG", ContentRegion: "Africa"})
	gb := forYouCacheKey(uid, SortRelevant, &visitor.VisitorContext{ContentCountry: "GB", ContentRegion: "Europe"})

	if ng == gb {
		t.Fatal("cache key must differ per content country — otherwise a ?geo= override or preference change serves a stale, wrong-geo ranking (ADR-064 §32)")
	}
}

func TestForYouCacheKey_IncludesContentRegion(t *testing.T) {
	t.Parallel()

	var tid, uid pgtype.UUID
	_ = tid.Scan("11111111-1111-1111-1111-111111111111")
	_ = uid.Scan("22222222-2222-2222-2222-222222222222")

	ngAfrica := forYouCacheKey(uid, SortRelevant, &visitor.VisitorContext{ContentCountry: "NG", ContentRegion: "Africa"})
	ngEurope := forYouCacheKey(uid, SortRelevant, &visitor.VisitorContext{ContentCountry: "NG", ContentRegion: "Europe"})

	if ngAfrica == ngEurope {
		t.Fatal("cache key must differ per content region")
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
