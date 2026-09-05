package fetcher

import (
	"os"
	"strings"
	"testing"
)

// TestPDFFetcher_ExtractText reads the embedded-text PDF fixture and asserts
// that the extracted text contains expected keywords and exceeds the 50-char
// threshold (so no OCR fallback is triggered).
func TestPDFFetcher_ExtractText(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/scholarship_gov_pdf.pdf")
	if err != nil {
		t.Fatal(err)
	}

	f := NewPDFFetcher(nil)

	text, err := f.ExtractText(body)
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}

	if len(strings.TrimSpace(text)) < 50 {
		t.Fatalf("extracted text too short (%d chars), expected >= 50", len(strings.TrimSpace(text)))
	}

	if !strings.Contains(strings.ToLower(text), "mext") {
		t.Errorf("extracted text missing 'mext': %q", text)
	}

	if !strings.Contains(strings.ToLower(text), "scholarship") {
		t.Errorf("extracted text missing 'scholarship': %q", text)
	}
}

// TestPDFFetcher_ExtractTextInvalid ensures a non-PDF body returns an error.
func TestPDFFetcher_ExtractTextInvalid(t *testing.T) {
	t.Parallel()

	f := NewPDFFetcher(nil)
	if _, err := f.ExtractText([]byte("not a pdf")); err == nil {
		t.Fatal("expected error for invalid PDF body, got nil")
	}
}

// TestPDFFetcher_ExtractTextStable verifies that extracting text twice from
// the same PDF yields identical output.
func TestPDFFetcher_ExtractTextStable(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/scholarship_gov_pdf.pdf")
	if err != nil {
		t.Fatal(err)
	}

	f := NewPDFFetcher(nil)

	t1, err := f.ExtractText(body)
	if err != nil {
		t.Fatalf("first extract: %v", err)
	}

	t2, err := f.ExtractText(body)
	if err != nil {
		t.Fatalf("second extract: %v", err)
	}

	if t1 != t2 {
		t.Errorf("text extraction not stable:\n--- first ---\n%s\n--- second ---\n%s", t1, t2)
	}
}
