package fetcher

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFFetcher extracts text from PDF bodies. It is a text-extraction utility
// used by the worker after a static fetch returns a PDF content-type, so it
// does not implement the Fetcher interface directly. When the embedded text
// layer is empty or minimal (scanned PDFs), it falls back to OCR.
type PDFFetcher struct {
	ocrFallback *OCRFetcher
}

// NewPDFFetcher returns a PDFFetcher. If ocr is non-nil, it is used as the
// fallback when text extraction yields < 50 chars.
func NewPDFFetcher(ocr *OCRFetcher) *PDFFetcher {
	return &PDFFetcher{ocrFallback: ocr}
}

// ExtractText pulls text from a PDF body. Falls back to OCR if extraction
// returns empty/minimal text (scanned PDFs).
func (f *PDFFetcher) ExtractText(body []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("pdf: new reader: %w", err)
	}

	var b strings.Builder

	totalPage := r.NumPage()
	for i := 1; i <= totalPage; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}

		b.WriteString(text)
	}

	text := b.String()
	if len(strings.TrimSpace(text)) < 50 && f.ocrFallback != nil {
		return f.ocrFallback.ExtractText(body)
	}

	return text, nil
}
