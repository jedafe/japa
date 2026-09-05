package fetcher

import (
	"os"
	"strings"
	"testing"
)

// TestStaticFetcher_NormalizationStable verifies that running normalizeHTML
// twice produces the same hash — i.e. normalization is idempotent, so
// re-crawling an unchanged page does not trigger a false change.
func TestStaticFetcher_NormalizationStable(t *testing.T) {
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
