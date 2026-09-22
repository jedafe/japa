package fetcher

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// TestStaticFetcher_NormalizationStable verifies that running normalizeHTML
// twice produces the same hash — i.e. normalization is idempotent, so
// re-crawling an unchanged page does not trigger a false change.
func TestStaticFetcher_NormalizationStable(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/scholarship_daad.html")
	if err != nil {
		t.Fatal(err)
	}

	h1 := NormalizeForHash(normalizeHTML(body), "text/html")
	h2 := NormalizeForHash(normalizeHTML(normalizeHTML(body)), "text/html")

	if h1 != h2 {
		t.Errorf("normalization not stable: %q != %q", h1, h2)
	}
}

// TestStaticFetcher_NormalizationStripsScripts confirms script/style content
// does not leak into the normalized text used for hashing.
func TestStaticFetcher_NormalizationStripsScripts(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/scholarship_daad.html")
	if err != nil {
		t.Fatal(err)
	}

	normalized := string(normalizeHTML(body))
	if strings.Contains(normalized, "analytics") {
		t.Errorf("script content leaked into normalized text: %q", normalized)
	}

	if strings.Contains(normalized, "display:none") {
		t.Errorf("style content leaked into normalized text: %q", normalized)
	}
}

// TestStaticFetcher_ErrorStatus: non-2xx responses are fetch failures, not
// documents — an error page must not be stored as channel content.
func TestStaticFetcher_ErrorStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<html><body>500 error page</body></html>"))
	}))
	t.Cleanup(srv.Close)

	f := &StaticFetcher{client: srv.Client()}
	if _, err := f.Fetch(t.Context(), sources.Source{SourceUrl: srv.URL}); err == nil {
		t.Fatal("500 response: expected fetch error, got nil")
	}
}
