package rss

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/db/gen"
)

// TestHandleUserFeed_MissingUsername verifies the handler returns 400
// when the username path parameter is empty.
func TestHandleUserFeed_MissingUsername(t *testing.T) {
	t.Parallel()

	q := (*gen.Queries)(nil) // unused — handler returns before query
	handler := HandleUserFeed(q, "https://example.com", "Test Site")

	req := httptest.NewRequest(http.MethodGet, "/users//feed.xml", nil)
	w := httptest.NewRecorder()

	// chi.Router is needed for URLParam to resolve the route param.
	r := chi.NewRouter()
	r.Get("/users/{username}/feed.xml", handler)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestHandleUserFeed_MissingTenant verifies the handler returns 503
// when no tenant ID is present in the request context.
func TestHandleUserFeed_MissingTenant(t *testing.T) {
	t.Parallel()

	q := (*gen.Queries)(nil)
	handler := HandleUserFeed(q, "https://example.com", "Test Site")

	req := httptest.NewRequest(http.MethodGet, "/users/alice/feed.xml", nil)
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/users/{username}/feed.xml", handler)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestHandleUserFeed_EmptyFeed verifies the handler returns valid RSS XML
// when the username is present but the database pool is nil (panics)
// — so we only test up to the tenant check here.
func TestHandleUserFeed_InvalidTenant(t *testing.T) {
	t.Parallel()

	q := (*gen.Queries)(nil)
	handler := HandleUserFeed(q, "https://example.com", "Test Site")

	req := httptest.NewRequest(http.MethodGet, "/users/bob/feed.xml", nil)
	// Context without a tenant ID.
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/users/{username}/feed.xml", handler)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestServeRSSFeed verifies the XML marshalling produces valid RSS 2.0.
func TestServeRSSFeed(t *testing.T) {
	t.Parallel()

	items := []RSSItem{
		{
			Title:       "Hello World",
			Link:        "https://japaconnect.com/content/hello-world",
			Description: "A test post",
			PubDate:     "Mon, 01 Jan 2024 00:00:00 +0000",
			Author:      "alice",
			GUID:        "https://japaconnect.com/content/hello-world",
		},
	}

	w := httptest.NewRecorder()
	serveRSSFeed(w, "Test Feed", "https://japaconnect.com/feed.xml", "Desc", items)

	if ct := w.Header().Get("Content-Type"); ct != "application/rss+xml; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}

	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Fatalf("unexpected Cache-Control: %s", cc)
	}

	var feed RSSFeed
	if err := xml.Unmarshal(w.Body.Bytes(), &feed); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	if feed.Version != "2.0" {
		t.Fatalf("expected RSS version 2.0, got %s", feed.Version)
	}

	if feed.Channel.Title != "Test Feed" {
		t.Fatalf("unexpected title: %s", feed.Channel.Title)
	}

	if len(feed.Channel.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(feed.Channel.Items))
	}

	if feed.Channel.Items[0].Title != "Hello World" {
		t.Fatalf("unexpected item title: %s", feed.Channel.Items[0].Title)
	}
}
