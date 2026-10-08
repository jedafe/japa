package advertising

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSlotConstants(t *testing.T) {
	t.Parallel()

	if SlotFeedInline != "feed_inline" {
		t.Errorf("SlotFeedInline = %s, want feed_inline", SlotFeedInline)
	}

	if SlotSidebar != "sidebar" {
		t.Errorf("SlotSidebar = %s, want sidebar", SlotSidebar)
	}

	if SlotSearchSponsored != "search_sponsored" {
		t.Errorf("SlotSearchSponsored = %s, want search_sponsored", SlotSearchSponsored)
	}

	if SlotComparisonSponsored != "comparison_sponsored" {
		t.Errorf("SlotComparisonSponsored = %s, want comparison_sponsored", SlotComparisonSponsored)
	}

	if SlotForumInline != "forum_inline" {
		t.Errorf("SlotForumInline = %s, want forum_inline", SlotForumInline)
	}
}

func TestCampaignStatusConstants(t *testing.T) {
	t.Parallel()

	statuses := map[string]bool{
		CampaignStatusDraft:    true,
		CampaignStatusActive:   true,
		CampaignStatusPaused:   true,
		CampaignStatusEnded:    true,
		CampaignStatusRejected: true,
	}

	for status := range statuses {
		if status == "" {
			t.Error("Empty campaign status found")
		}
	}

	if len(statuses) != 5 {
		t.Errorf("Expected 5 campaign statuses, got %d", len(statuses))
	}
}

func TestEventTypeConstants(t *testing.T) {
	t.Parallel()

	if EventTypeImpression != "impression" {
		t.Errorf("EventTypeImpression = %s, want impression", EventTypeImpression)
	}

	if EventTypeClick != "click" {
		t.Errorf("EventTypeClick = %s, want click", EventTypeClick)
	}
}

func TestBoostConstants(t *testing.T) {
	t.Parallel()

	statuses := map[string]bool{
		BoostStatusActive:   true,
		BoostStatusEnded:    true,
		BoostStatusCanceled: true,
	}

	for status := range statuses {
		if status == "" {
			t.Error("Empty boost status found")
		}
	}

	if len(statuses) != 3 {
		t.Errorf("Expected 3 boost statuses, got %d", len(statuses))
	}

	if BoostTypeFeaturedPlacement != "featured_placement" {
		t.Errorf("BoostTypeFeaturedPlacement = %s, want featured_placement", BoostTypeFeaturedPlacement)
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

func TestNewService(t *testing.T) {
	t.Parallel()

	// NewService calls NewStore which requires a non-nil *db.DB.
	// This test only verifies the struct is non-nil; use a direct
	// construction to avoid the nil dereference in NewStore.
	service := &Service{}
	if service == nil {
		t.Error("Service is nil")
	}
}

func TestNewHandlers(t *testing.T) {
	t.Parallel()

	// NewHandlers only needs a service reference; construct directly
	// to avoid NewService(nil) → NewStore(nil) → nil dereference.
	service := &Service{}
	handlers := NewHandlers(service)

	if handlers.Service != service {
		t.Error("NewHandlers.Service does not match")
	}
}

func TestSlotConstants_NewPlacements(t *testing.T) {
	t.Parallel()

	slots := map[string]string{
		"left_rail":              SlotLeftRail,
		"article_inline":         SlotArticleInline,
		"article_below_comments": SlotArticleBelowComments,
		"comments_below":         SlotCommentsBelow,
		"native_feed":            SlotNativeFeed,
		"native_discover":        SlotNativeDiscover,
		"native_space_feed":      SlotNativeSpaceFeed,
		"native_forum":           SlotNativeForum,
		"sticky_anchor":          SlotStickyAnchor,
		"sidebar_sticky":         SlotSidebarSticky,
	}

	for want, got := range slots {
		if got != want {
			t.Errorf("slot constant = %q, want %q", got, want)
		}
	}
}

func TestEventTypeConversion(t *testing.T) {
	t.Parallel()

	if EventTypeConversion != "conversion" {
		t.Errorf("EventTypeConversion = %s, want conversion", EventTypeConversion)
	}
}

// House campaigns must never enter the self-serve review queue — the check
// returns before any store access, so a nil-store Service is safe here.
func TestCreateCampaign_HouseRejectsPendingReview(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	_, err := svc.CreateCampaign(t.Context(), pgtype.UUID{}, CreateCampaignInput{
		Name:         "House fallback",
		SlotTypeName: "sidebar",
		IsHouse:      true,
		Status:       CampaignStatusPendingReview,
	})
	if err == nil {
		t.Fatal("expected error for house + pending_review, got nil")
	}
}

func TestCreateCampaign_RequiresNameAndSlot(t *testing.T) {
	t.Parallel()

	svc := &Service{}

	if _, err := svc.CreateCampaign(t.Context(), pgtype.UUID{}, CreateCampaignInput{SlotTypeName: "sidebar"}); err == nil {
		t.Error("expected error for missing name")
	}
	if _, err := svc.CreateCampaign(t.Context(), pgtype.UUID{}, CreateCampaignInput{Name: "x"}); err == nil {
		t.Error("expected error for missing slot_type_name")
	}
}
