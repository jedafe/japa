package rss

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
)

type stubRSSChecker struct{ off map[string]bool }

func (s stubRSSChecker) IsEnabled(_ context.Context, key string) bool { return !s.off[key] }

func TestContentTypeGateOff(t *testing.T) {
	c := stubRSSChecker{off: map[string]bool{"story.enabled": true}}

	if !ContentTypeGateOff(context.Background(), c, "story") {
		t.Error("story gated off — expected true")
	}
	if ContentTypeGateOff(context.Background(), c, "video") {
		t.Error("video flag on — expected false")
	}
	if ContentTypeGateOff(context.Background(), c, "guide") {
		t.Error("guide ungated — expected false")
	}
	if ContentTypeGateOff(context.Background(), nil, "story") {
		t.Error("nil checker — expected false (fail-open)")
	}
}

func TestFeedItemsFromAllRowsGated(t *testing.T) {
	rows := []gen.ListRSSFeedItemsRow{
		{ContentType: "story", Title: "s"},
		{ContentType: "guide", Title: "g"},
		{ContentType: "video", Title: "v"},
	}

	gated := func(ct string) bool { return ct == "story" || ct == "video" }
	items := feedItemsFromAllRows(rows, "https://x", gated)
	if len(items) != 1 || items[0].Title != "g" {
		t.Fatalf("expected only the guide item, got %+v", items)
	}

	// nil predicate — unfiltered.
	if items := feedItemsFromAllRows(rows, "https://x", nil); len(items) != 3 {
		t.Fatalf("nil predicate should keep all rows, got %d", len(items))
	}
}
