package fetcher

import (
	"os"
	"testing"
)

// TestRSSFetcher_Items parses the scholarship_rss.xml fixture and asserts
// that 3 items are extracted with the expected titles.
func TestRSSFetcher_Items(t *testing.T) {
	body, err := os.ReadFile("testdata/scholarship_rss.xml")
	if err != nil {
		t.Fatal(err)
	}

	f := NewRSSFetcher()
	items, err := f.Items(body)
	if err != nil {
		t.Fatalf("parse items: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	want := []string{
		"DAAD Scholarship for Developing Countries 2026",
		"MEXT Scholarship Japan 2026",
		"Joint Japan/World Bank Graduate Scholarship",
	}
	for i, w := range want {
		if items[i].Title != w {
			t.Errorf("item %d title = %q, want %q", i, items[i].Title, w)
		}
		if items[i].Link == "" {
			t.Errorf("item %d link is empty", i)
		}
	}
}

// TestRSSFetcher_ItemsInvalidXML ensures a parse error is surfaced.
func TestRSSFetcher_ItemsInvalidXML(t *testing.T) {
	f := NewRSSFetcher()
	if _, err := f.Items([]byte("not xml")); err == nil {
		t.Fatal("expected parse error for invalid XML, got nil")
	}
}
