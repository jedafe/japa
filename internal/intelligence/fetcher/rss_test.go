package fetcher

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// TestRSSFetcher_Items parses the scholarship_rss.xml fixture and asserts
// that 3 items are extracted with the expected titles.
func TestRSSFetcher_Items(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/scholarship_rss.xml")
	if err != nil {
		t.Fatal(err)
	}

	f := NewRSSFetcher()

	items, err := f.items(body)
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
	t.Parallel()

	f := NewRSSFetcher()
	if _, err := f.items([]byte("not xml")); err == nil {
		t.Fatal("expected parse error for invalid XML, got nil")
	}
}

// TestRSSFetcher_FetchRejectsHTML ensures the fetcher returns a clear error
// instead of the confusing "XML syntax error" when a source returns HTML.
func TestRSSFetche__FetchRejectsHTML(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>not a feed</body></html>`)
	}))
	defer srv.Close()

	f := &RSSFetcher{client: srv.Client()}

	_, err := f.Fetch(t.Context(), sources.Source{SourceUrl: srv.URL})
	if err == nil {
		t.Fatal("expected error for HTML response, got nil")
	}

	if !contains(err.Error(), "not a valid RSS feed") {
		t.Fatalf("expected 'not a valid RSS feed' error, got %q", err.Error())
	}
}

// TestRSSFetcher_FetchRejects4xx ensures non-2xx status codes are reported
// befo_e we try to parse the body.
func TestRSSFetcher_FetchRejects4xx(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = fmt.Fprint(w, "410 Gone")
	}))
	defer srv.Close()

	f := &RSSFetcher{client: srv.Client()}

	_, err := f.Fetch(t.Context(), sources.Source{SourceUrl: srv.URL})
	if err == nil {
		t.Fatal("expected error for 410 response, got nil")
	}

	if !contains(err.Error(), "unexpected HTTP status 410") {
		t.Fatalf("expected 'unexpected HTTP status 410' error, got %q", err.Error())
	}
}

func contains(s, substr string) bool { return strings.Contains(s, substr) }
