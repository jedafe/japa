package fetcher

import (
	"os"
	"strings"
	"testing"
)

// TestOCRFetcher_Available checks that Available() reflects whether
// tesseract + pdftoppm are installed. The test skips gracefully if the
// binaries are absent (dev environments without OCR tooling).
func TestOCRFetcher_Available(t *testing.T) {
	t.Parallel()

	f := NewOCRFetcher()
	if !f.Available() {
		t.Skip("tesseract/pdftoppm not installed, skipping OCR availability test")
	}
}

// TestOCRFetcher_ExtractText runs the full OCR pipeline against the scanned
// PDF fixture. Skips if tesseract or pdftoppm is not in PATH.
func TestOCRFetcher_ExtractText(t *testing.T) {
	t.Parallel()

	f := NewOCRFetcher()
	if !f.Available() {
		t.Skip("tesseract/pdftoppm not installed, skipping OCR extraction test")
	}

	body, err := os.ReadFile("testdata/scholarship_scanned.pdf")
	if err != nil {
		t.Fatal(err)
	}

	text, err := f.ExtractText(body)
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}

	if strings.TrimSpace(text) == "" {
		t.Fatal("OCR returned empty text")
	}
}

// TestOCRFetcher_ExtractTextNotAvailable ensures that when tesseract is
// absent, ExtractText returns an error (callers route to REVIEW_REQUIRED).
func TestOCRFetcher_ExtractTextNotAvailable(t *testing.T) {
	t.Parallel()

	f := NewOCRFetcher()
	if f.Available() {
		t.Skip("tesseract installed; cannot test unavailable path")
	}

	if _, err := f.ExtractText([]byte("dummy")); err == nil {
		t.Fatal("expected error when tesseract not available, got nil")
	}
}

// TestOCRFetcher_WithCacheStoresTenantID verifies that
func TestOCRFetcher_WithCache(t *testing.T) {
	t.Parallel()

	f := NewOCRFetcherWithCache(startTestValkey(t))
	if f.rdb == nil {
		t.Fatal("rdb should be set after NewOCRFetcherWithCache")
	}
}
