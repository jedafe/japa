package feed

import (
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestSortModeConstants(t *testing.T) {
	t.Parallel()

	if SortLatest != "latest" {
		t.Errorf("SortLatest = %q, want latest", SortLatest)
	}

	if SortTop != "top" {
		t.Errorf("SortTop = %q, want top", SortTop)
	}

	if SortRising != "rising" {
		t.Errorf("SortRising = %q, want rising", SortRising)
	}

	if SortRelevant != "relevant" {
		t.Errorf("SortRelevant = %q, want relevant", SortRelevant)
	}
}

func TestFanOutThreshold(t *testing.T) {
	t.Parallel()

	if FanOutThreshold != 5000 {
		t.Errorf("FanOutThreshold = %d, want 5000", FanOutThreshold)
	}
}

func TestPageSizeConstants(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

// ---------------------------------------------------------------------------
// ComputeVelocity
// ---------------------------------------------------------------------------

func TestComputeVelocity(t *testing.T) {
	t.Parallel()

	// Formula mirrors the SQL in feeds.sql (UpdateVelocityScores):
	//   (vote_score*2 + comment_count + view_count/100) / hours^1.8
	tests := []struct {
		name          string
		voteScore     int32
		commentCount  int32
		viewCount     int32
		bookmarkCount int32
		hoursOld      float64
		want          float64
	}{
		{
			name:          "fresh post with engagement",
			voteScore:     10,
			commentCount:  5,
			viewCount:     100,
			bookmarkCount: 3,
			hoursOld:      1,
			want:          (10*2 + 5 + 100.0/100) / math.Pow(1, 1.8), // =26 / 1^1.8 = 26
		},
		{
			name:          "old post with engagement",
			voteScore:     50,
			commentCount:  20,
			viewCount:     500,
			bookmarkCount: 10,
			hoursOld:      24,
			want:          (50*2 + 20 + 500.0/100) / math.Pow(24, 1.8), // =125 / 24^1.8
		},
		{
			name:          "no engagement",
			voteScore:     0,
			commentCount:  0,
			viewCount:     0,
			bookmarkCount: 0,
			hoursOld:      5,
			want:          0,
		},
		{
			name:          "very new post",
			voteScore:     1,
			commentCount:  0,
			viewCount:     10,
			bookmarkCount: 0,
			hoursOld:      0.1,
			want:          (1*2 + 0 + 10.0/100) / math.Pow(0.1, 1.8), // =2.1 / 0.1^1.8
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ComputeVelocity(tt.voteScore, tt.commentCount, tt.viewCount, tt.bookmarkCount, tt.hoursOld)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("ComputeVelocity(%d, %d, %d, %d, %f) = %f, want %f",
					tt.voteScore, tt.commentCount, tt.viewCount, tt.bookmarkCount, tt.hoursOld, got, tt.want)
			}
		})
	}
}

func TestComputeVelocityDenominatorGrowsWithAge(t *testing.T) {
	t.Parallel()
	// As hoursOld increases, velocity should decrease for same engagement.
	v1 := ComputeVelocity(10, 5, 100, 3, 1)
	v2 := ComputeVelocity(10, 5, 100, 3, 24)

	if v1 <= v2 {
		t.Errorf("velocity at 1h (%f) should be > velocity at 24h (%f)", v1, v2)
	}
}

func TestComputeVelocityEngagementScalesLinearly(t *testing.T) {
	t.Parallel()
	// Double engagement should double velocity for same age.
	v1 := ComputeVelocity(10, 5, 100, 3, 5)
	v2 := ComputeVelocity(20, 10, 200, 6, 5)

	if math.Abs(v2-2*v1) > 1e-10 {
		t.Errorf("double engagement should double velocity: v1=%f, v2=%f", v1, v2)
	}
}

// ---------------------------------------------------------------------------
// FeedItem struct
// ---------------------------------------------------------------------------

func TestFeedItemStructFields(t *testing.T) {
	t.Parallel()

	spaceID := "space-789"
	item := FeedItem{
		ID:            "abc-123",
		Title:         "Test Post",
		Slug:          "test-post",
		ContentType:   "discussion",
		AuthorID:      "author-456",
		SpaceID:       &spaceID,
		CreatedAt:     "2026-01-01",
		ViewCount:     100,
		CommentCount:  5,
		VoteScore:     10,
		VelocityScore: 3.14,
	}

	if item.ID != "abc-123" {
		t.Errorf("ID = %q, want abc-123", item.ID)
	}

	if item.Title != "Test Post" {
		t.Errorf("Title = %q, want Test Post", item.Title)
	}

	if item.Slug != "test-post" {
		t.Errorf("Slug = %q, want test-post", item.Slug)
	}

	if item.ContentType != "discussion" {
		t.Errorf("ContentType = %q, want discussion", item.ContentType)
	}

	if item.AuthorID != "author-456" {
		t.Errorf("AuthorID = %q, want author-456", item.AuthorID)
	}

	if item.SpaceID == nil || *item.SpaceID != "space-789" {
		t.Errorf("SpaceID = %v, want space-789", item.SpaceID)
	}

	if item.ViewCount != 100 {
		t.Errorf("ViewCount = %d, want 100", item.ViewCount)
	}

	if item.CommentCount != 5 {
		t.Errorf("CommentCount = %d, want 5", item.CommentCount)
	}

	if item.VoteScore != 10 {
		t.Errorf("VoteScore = %d, want 10", item.VoteScore)
	}
}

func TestFeedItemNilSpaceID(t *testing.T) {
	t.Parallel()

	item := FeedItem{
		ID:    "abc-123",
		Title: "No Space",
	}
	if item.SpaceID != nil {
		t.Errorf("SpaceID should be nil, got %v", item.SpaceID)
	}
}

// ---------------------------------------------------------------------------
// PrimaryFeedItem struct
// ---------------------------------------------------------------------------

func TestPrimaryFeedItemStructFields(t *testing.T) {
	t.Parallel()

	spaceID := "space-3"

	item := PrimaryFeedItem{
		ID:               "feed-1",
		ContentID:        "content-2",
		SpaceID:          &spaceID,
		ContentType:      "article",
		Score:            5.5,
		CreatedAt:        "2026-01-01",
		Title:            "My Article",
		Slug:             "my-article",
		AuthorID:         "author-4",
		ContentCreatedAt: "2026-01-01",
	}
	if item.Score != 5.5 {
		t.Errorf("Score = %f, want 5.5", item.Score)
	}

	if item.ContentType != "article" {
		t.Errorf("ContentType = %q, want article", item.ContentType)
	}
}

// ---------------------------------------------------------------------------
// GetFeed with different sort modes (unit test, no DB)
// ---------------------------------------------------------------------------

func TestGetFeedUnknownTimeline(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	tenantID := pgtype.UUID{}
	_ = tenantID.Scan("00000000-0000-0000-0000-000000000001")

	_, err := svc.GetFeed(t.Context(), tenantID, "bogus", nil, nil, nil, SortLatest, 20, 0)
	if err == nil {
		t.Error("expected error for unknown timeline")
	}
}

func TestGetFeedPrimaryRequiresUserID(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	tenantID := pgtype.UUID{}
	_ = tenantID.Scan("00000000-0000-0000-0000-000000000001")

	_, err := svc.GetFeed(t.Context(), tenantID, "primary", nil, nil, nil, SortLatest, 20, 0)
	if err == nil {
		t.Error("expected error when primary feed has nil user_id")
	}
}

func TestGetFeedSpaceRequiresSpaceID(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	tenantID := pgtype.UUID{}
	_ = tenantID.Scan("00000000-0000-0000-0000-000000000001")

	_, err := svc.GetFeed(t.Context(), tenantID, "space", nil, nil, nil, SortLatest, 20, 0)
	if err == nil {
		t.Error("expected error when space feed has nil space_id")
	}
}

func TestGetFeedProfileRequiresAuthorID(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	tenantID := pgtype.UUID{}
	_ = tenantID.Scan("00000000-0000-0000-0000-000000000001")

	_, err := svc.GetFeed(t.Context(), tenantID, "profile", nil, nil, nil, SortLatest, 20, 0)
	if err == nil {
		t.Error("expected error when profile feed has nil author_id")
	}
}

// ---------------------------------------------------------------------------
// Empty feed handling
// ---------------------------------------------------------------------------

func TestExploreRowsToItemsEmpty(t *testing.T) {
	t.Parallel()

	items := exploreRowsToItems(nil)
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

func TestTopRowsToItemsEmpty(t *testing.T) {
	t.Parallel()

	items := topRowsToItems(nil)
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

func TestRisingRowsToItemsEmpty(t *testing.T) {
	t.Parallel()

	items := risingRowsToItems(nil)
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

func TestPrimaryRowsToItemsEmpty(t *testing.T) {
	t.Parallel()

	items := primaryRowsToItems(nil)
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

func TestSpaceRowsToItemsEmpty(t *testing.T) {
	t.Parallel()

	items := spaceRowsToItems(nil)
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

func TestProfileRowsToItemsEmpty(t *testing.T) {
	t.Parallel()

	items := profileRowsToItems(nil)
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

// ---------------------------------------------------------------------------
// Score update logic
// ---------------------------------------------------------------------------

func TestComputeVelocityScoreUpdateLogic(t *testing.T) {
	t.Parallel()
	// Verify that velocity increases when engagement increases.
	lowEngagement := ComputeVelocity(5, 2, 50, 1, 5)
	highEngagement := ComputeVelocity(20, 10, 200, 5, 5)

	if lowEngagement >= highEngagement {
		t.Errorf("higher engagement should yield higher velocity: low=%f, high=%f", lowEngagement, highEngagement)
	}
}

func TestComputeVelocityScoreDecay(t *testing.T) {
	t.Parallel()
	// Verify that velocity decays over time.
	recent := ComputeVelocity(10, 5, 100, 3, 2)
	old := ComputeVelocity(10, 5, 100, 3, 48)

	if recent <= old {
		t.Errorf("recent velocity (%f) should be > old velocity (%f)", recent, old)
	}
}
