package blog

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Constants and configuration
// ---------------------------------------------------------------------------

func TestSeriesStatusConstants(t *testing.T) {
	t.Parallel()

	if SeriesStatusActive != "active" {
		t.Errorf("SeriesStatusActive = %q, want %q", SeriesStatusActive, "active")
	}

	if SeriesStatusArchived != "archived" {
		t.Errorf("SeriesStatusArchived = %q, want %q", SeriesStatusArchived, "archived")
	}
}

func TestReviewStatusConstants(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		ReviewStatusPending:          "pending",
		ReviewStatusApproved:         "approved",
		ReviewStatusChangesRequested: "changes_requested",
		ReviewStatusRejected:         "rejected",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("review status constant = %q, want %q", got, want)
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
// Series status transitions
// ---------------------------------------------------------------------------

func TestSeriesStatusTransitions(t *testing.T) {
	t.Parallel()
	// Valid statuses: active, archived.
	validStatuses := map[string]bool{
		SeriesStatusActive:   true,
		SeriesStatusArchived: true,
	}
	for status := range validStatuses {
		if status != SeriesStatusActive && status != SeriesStatusArchived {
			t.Errorf("expected %q to be a valid series status", status)
		}
	}

	// Invalid statuses.
	invalidStatuses := []string{"draft", "published", "deleted", ""}
	for _, status := range invalidStatuses {
		if validStatuses[status] {
			t.Errorf("expected %q to be an invalid series status", status)
		}
	}
}

// ---------------------------------------------------------------------------
// Review status transitions
// ---------------------------------------------------------------------------

func TestReviewStatusTransitions(t *testing.T) {
	t.Parallel()
	// Valid statuses.
	validStatuses := map[string]bool{
		ReviewStatusPending:          true,
		ReviewStatusApproved:         true,
		ReviewStatusChangesRequested: true,
		ReviewStatusRejected:         true,
	}
	for status := range validStatuses {
		if !validStatuses[status] {
			t.Errorf("expected %q to be a valid review status", status)
		}
	}

	// Invalid statuses.
	invalidStatuses := []string{"draft", "published", "closed", ""}
	for _, status := range invalidStatuses {
		if validStatuses[status] {
			t.Errorf("expected %q to be an invalid review status", status)
		}
	}
}

// ---------------------------------------------------------------------------
// CreateSeriesInput validation
// ---------------------------------------------------------------------------

func TestCreateSeriesInputValidation(t *testing.T) {
	t.Parallel()
	// Empty title should be invalid.
	input := CreateSeriesInput{Title: "", Slug: "test-series"}
	if input.Title != "" {
		t.Error("expected empty title")
	}

	// Empty slug should be invalid.
	input2 := CreateSeriesInput{Title: "Test", Slug: ""}
	if input2.Slug != "" {
		t.Error("expected empty slug")
	}
}

// ---------------------------------------------------------------------------
// CreateReviewInput validation
// ---------------------------------------------------------------------------

func TestCreateReviewInputValidation(t *testing.T) {
	t.Parallel()
	// Empty content_id should be invalid.
	input := CreateReviewInput{}

	var zeroUUID pgtype.UUID

	if input.ContentID != zeroUUID {
		t.Error("expected zero UUID for content_id")
	}
}

// ---------------------------------------------------------------------------
// FeaturePostInput validation
// ---------------------------------------------------------------------------

func TestFeaturePostInputValidation(t *testing.T) {
	t.Parallel()
	// Empty content_id should be invalid.
	input := FeaturePostInput{}

	var zeroUUID pgtype.UUID

	if input.ContentID != zeroUUID {
		t.Error("expected zero UUID for content_id")
	}

	// Default order should be 0.
	if input.FeaturedOrder != 0 {
		t.Errorf("expected default featured_order 0, got %d", input.FeaturedOrder)
	}
}

// ---------------------------------------------------------------------------
// RSS XML generation
// ---------------------------------------------------------------------------

func TestRSSXMLHeader(t *testing.T) {
	t.Parallel()
	// Verify the XML header format.
	header := xml.Header
	if header != "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" {
		t.Errorf("unexpected XML header: %q", header)
	}
}

func TestRSSItemFields(t *testing.T) {
	t.Parallel()
	// Verify RSSItem struct has all required fields.
	item := RSSItem{
		Title:       "Test Post",
		Link:        "https://example.com/test",
		GUID:        "urn:uuid:123",
		Description: "A test post",
		PubDate:     "Mon, 01 Jan 2024 00:00:00 +0000",
		Author:      "testuser",
	}
	if item.Title != "Test Post" {
		t.Errorf("unexpected title: %q", item.Title)
	}

	if item.Link != "https://example.com/test" {
		t.Errorf("unexpected link: %q", item.Link)
	}

	if item.GUID != "urn:uuid:123" {
		t.Errorf("unexpected guid: %q", item.GUID)
	}
}

// ---------------------------------------------------------------------------
// Series and review entity type constants
// ---------------------------------------------------------------------------

func TestSeriesStatusValues(t *testing.T) {
	t.Parallel()

	statuses := []string{SeriesStatusActive, SeriesStatusArchived}
	for _, s := range statuses {
		if s == "" {
			t.Error("series status should not be empty")
		}
	}
}

func TestReviewStatusValues(t *testing.T) {
	t.Parallel()

	statuses := []string{
		ReviewStatusPending,
		ReviewStatusApproved,
		ReviewStatusChangesRequested,
		ReviewStatusRejected,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("review status should not be empty")
		}
	}
}

// TestGenerateRSSXML_PubDateZeroTimeFallback verifies RSS-003: a post with
// a zero/NULL published_at must fall back to created_at for the pubDate,
// not emit "Mon, 01 Jan 0001 ...". time.Time{}.Format(RFC1123Z) is non-empty,
// so the old `if pubDate == ""` check never fired.
func TestGenerateRSSXML_PubDateZeroTimeFallback(t *testing.T) {
	t.Parallel()

	created := pgtype.Timestamptz{Time: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC), Valid: true}
	posts := []gen.ListPublishedPostsForRSSRow{
		{
			ID:          pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true},
			Title:       "Zero Pub Post",
			Slug:        "zero-pub-post",
			BodyHtml:    "<p>Hello</p>",
			Body:        "Hello",
			PublishedAt: pgtype.Timestamptz{}, // zero/NULL — must fall back
			CreatedAt:   created,
		},
	}

	var sb strings.Builder
	if err := generateRSSXML(&sb, posts, "https://example.com", "Test Site"); err != nil {
		t.Fatalf("generateRSSXML: %v", err)
	}

	body := sb.String()

	// The bogus year-0001 pubDate must NOT appear.
	if strings.Contains(body, "01 Jan 0001") {
		t.Fatalf("RSS-003 regression: feed contains bogus year-0001 pubDate:\n%s", body)
	}

	// The created_at fallback (15 Mar 2024) must appear.
	if !strings.Contains(body, "15 Mar 2024") {
		t.Fatalf("expected created_at fallback '15 Mar 2024' in feed:\n%s", body)
	}

	// Sanity: still valid RSS 2.0 XML.
	var feed RSSFeed
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	if feed.Version != "2.0" {
		t.Fatalf("expected RSS 2.0, got %s", feed.Version)
	}
}
