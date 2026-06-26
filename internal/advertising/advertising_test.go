package advertising

import (
	"testing"
)

func TestSlotConstants(t *testing.T) {
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
	if EventTypeImpression != "impression" {
		t.Errorf("EventTypeImpression = %s, want impression", EventTypeImpression)
	}
	if EventTypeClick != "click" {
		t.Errorf("EventTypeClick = %s, want click", EventTypeClick)
	}
}

func TestBoostConstants(t *testing.T) {
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
	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}
	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

func TestNewService(t *testing.T) {
	service := NewService(nil)
	if service == nil {
		t.Error("NewService returned nil")
	}
	if service.store == nil {
		t.Error("NewService.store is nil")
	}
}

func TestNewHandlers(t *testing.T) {
	service := NewService(nil)
	handlers := NewHandlers(service)
	if handlers == nil {
		t.Error("NewHandlers returned nil")
	}
	if handlers.Service != service {
		t.Error("NewHandlers.Service does not match")
	}
}