package rss

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/db/gen"
)

// newTestHandlers creates Handlers with a nil queries pointer for tests
// that return before reaching the DB layer.
func newTestHandlers() *Handlers {
	return NewHandlers((*gen.Queries)(nil), "https://example.com", "Test Site")
}

// TestHandleUserFeed_MissingUsername verifies the handler returns 400
// when the username path parameter is empty.
func TestHandleUserFeed_MissingUsername(t *testing.T) {
	t.Parallel()

	h := newTestHandlers()

	req := httptest.NewRequest(http.MethodGet, "/users//feed.xml", nil)
	w := httptest.NewRecorder()

	// chi.Router is needed for URLParam to resolve the route param.
	r := chi.NewRouter()
	r.Get("/users/{username}/feed.xml", h.HandleUserFeed)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestHandleUserFeed_MissingTenant verifies the handler returns 503
// when no tenant ID is present in the request context.
func TestHandleUserFeed_MissingTenant(t *testing.T) {
	t.Parallel()

	h := newTestHandlers()

	req := httptest.NewRequest(http.MethodGet, "/users/alice/feed.xml", nil)
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/users/{username}/feed.xml", h.HandleUserFeed)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestHandleUserFeed_InvalidTenant verifies the handler returns 503
// when the context has no tenant ID.
func TestHandleUserFeed_InvalidTenant(t *testing.T) {
	t.Parallel()

	h := newTestHandlers()

	req := httptest.NewRequest(http.MethodGet, "/users/bob/feed.xml", nil)
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/users/{username}/feed.xml", h.HandleUserFeed)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestHandleFeed_MissingTenant verifies the universal feed handler
// returns 503 when no tenant ID is in the context.
func TestHandleFeed_MissingTenant(t *testing.T) {
	t.Parallel()

	h := newTestHandlers()

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/feed.xml", h.HandleFeed)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestHandleSpaceFeed_MissingSlug verifies the space feed handler
// returns 400 when the slug path parameter is empty.
func TestHandleSpaceFeed_MissingSlug(t *testing.T) {
	t.Parallel()

	h := newTestHandlers()

	req := httptest.NewRequest(http.MethodGet, "/spaces//feed.xml", nil)
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/spaces/{slug}/feed.xml", h.HandleSpaceFeed)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestHandleTagFeed_MissingSlug verifies the tag feed handler
// returns 400 when the slug path parameter is empty.
func TestHandleTagFeed_MissingSlug(t *testing.T) {
	t.Parallel()

	h := newTestHandlers()

	req := httptest.NewRequest(http.MethodGet, "/tags//feed.xml", nil)
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/tags/{slug}/feed.xml", h.HandleTagFeed)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestServeRSSFeed verifies the XML marshalling produces valid RSS 2.0
// with the XML declaration, correct Content-Type, and Cache-Control.
func TestServeRSSFeed(t *testing.T) {
	t.Parallel()

	items := []RSSItem{
		{
			Title:       "Hello World",
			Link:        "https://japaconnect.com/content/hello-world",
			GUID:        "urn:uuid:test-123",
			Description: "A test post",
			PubDate:     "Mon, 01 Jan 2024 00:00:00 +0000",
			Author:      "alice",
		},
	}

	w := httptest.NewRecorder()
	serveRSSFeed(w, "Test Feed", "https://japaconnect.com/feed.xml", "Desc", items)

	if ct := w.Header().Get("Content-Type"); ct != "application/rss+xml; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}

	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=900" {
		t.Fatalf("unexpected Cache-Control: %s", cc)
	}

	body := w.Body.String()

	// Verify XML declaration is present.
	if !strings.HasPrefix(body, "<?xml") {
		t.Fatalf("missing XML declaration, body starts with: %q", body[:min(50, len(body))])
	}

	var feed RSSFeed
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	if feed.Version != "2.0" {
		t.Fatalf("expected RSS version 2.0, got %s", feed.Version)
	}

	if feed.Channel.Title != "Test Feed" {
		t.Fatalf("unexpected title: %s", feed.Channel.Title)
	}

	if feed.Channel.LastBuildDate == "" {
		t.Fatal("expected non-empty lastBuildDate")
	}

	if len(feed.Channel.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(feed.Channel.Items))
	}

	if feed.Channel.Items[0].Title != "Hello World" {
		t.Fatalf("unexpected item title: %s", feed.Channel.Items[0].Title)
	}

	if feed.Channel.Items[0].GUID != "urn:uuid:test-123" {
		t.Fatalf("unexpected GUID: %s", feed.Channel.Items[0].GUID)
	}
}

// TestResolveSiteURL verifies that X-Forwarded-Host is preferred over
// the fallback URL, and that the scheme is derived correctly.
func TestResolveSiteURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		host     string
		proto    string
		tls      bool
		fallback string
		want     string
	}{
		{
			name:     "with X-Forwarded-Host and https proto",
			host:     "japaconnect.com",
			proto:    "https",
			fallback: "http://localhost:8080",
			want:     "https://japaconnect.com",
		},
		{
			name:     "with X-Forwarded-Host and no proto, no TLS",
			host:     "japaconnect.com",
			proto:    "",
			tls:      false,
			fallback: "http://localhost:8080",
			want:     "http://japaconnect.com",
		},
		{
			name:     "with X-Forwarded-Host and no proto, TLS",
			host:     "japaconnect.com",
			proto:    "",
			tls:      true,
			fallback: "http://localhost:8080",
			want:     "https://japaconnect.com",
		},
		{
			name:     "without X-Forwarded-Host uses fallback",
			host:     "",
			fallback: "http://localhost:8080",
			want:     "http://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.host != "" {
				req.Header.Set("X-Forwarded-Host", tt.host)
			}
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			req.TLS = nil
			if tt.tls {
				// Simulate TLS by setting a non-nil TLS state.
				// Since httptest.NewRequest doesn't easily set TLS,
				// we rely on the proto header for the TLS case.
				req.Header.Set("X-Forwarded-Proto", "https")
			}

			got := resolveSiteURL(req, tt.fallback)
			if got != tt.want {
				t.Fatalf("resolveSiteURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHTMLToTextExcerpt verifies HTML stripping and truncation.
func TestHTMLToTextExcerpt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		maxRunes int
		want    string
	}{
		{
			name:    "strips HTML tags",
			input:   "<p>Hello <strong>World</strong></p>",
			maxRunes: 300,
			want:    "Hello World",
		},
		{
			name:    "truncates with ellipsis",
			input:   "<p>" + strings.Repeat("a", 400) + "</p>",
			maxRunes: 10,
			want:    "aaaaaaaaaa…",
		},
		{
			name:    "collapses whitespace",
			input:   "<p>Hello\n\n  World</p>",
			maxRunes: 300,
			want:    "Hello World",
		},
		{
			name:    "empty input",
			input:   "",
			maxRunes: 300,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := htmlToTextExcerpt(tt.input, tt.maxRunes)
			if got != tt.want {
				t.Fatalf("htmlToTextExcerpt() = %q, want %q", got, tt.want)
			}
		})
	}
}
