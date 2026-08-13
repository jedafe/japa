package fetcher

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestOCRFetcher_Available checks that Available() reflects whether
// tesseract + pdftoppm are installed. The test skips gracefully if the
// binaries are absent (dev environments without OCR tooling).
func TestOCRFetcher_Available(t *testing.T) {
	f := NewOCRFetcher()
	if !f.Available() {
		t.Skip("tesseract/pdftoppm not installed, skipping OCR availability test")
	}
}

// TestOCRFetcher_ExtractText runs the full OCR pipeline against the scanned
// PDF fixture. Skips if tesseract or pdftoppm is not in PATH.
func TestOCRFetcher_ExtractText(t *testing.T) {
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
	f := NewOCRFetcher()
	if f.Available() {
		t.Skip("tesseract installed; cannot test unavailable path")
	}
	if _, err := f.ExtractText([]byte("dummy")); err == nil {
		t.Fatal("expected error when tesseract not available, got nil")
	}
}

// TestOCRFetcher_WithCacheStoresTenantID verifies that
// NewOCRFetcherWithCache stores the tenantID on the struct so that the
// OCR cache key is tenant-scoped (ADR-112 Amendment 2).
func TestOCRFetcher_WithCacheStoresTenantID(t *testing.T) {
	tid := pgtype.UUID{Bytes: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Valid: true}
	f := NewOCRFetcherWithCache(startTestValkey(t), tid)
	if !f.tenantID.Valid {
		t.Fatal("tenantID should be valid after NewOCRFetcherWithCache")
	}
	if f.tenantID.String() != tid.String() {
		t.Errorf("tenantID: got %s, want %s", f.tenantID.String(), tid.String())
	}
	if f.rdb == nil {
		t.Fatal("rdb should be set after NewOCRFetcherWithCache")
	}
}
