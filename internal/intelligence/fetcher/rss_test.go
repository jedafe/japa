package fetcher

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

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
