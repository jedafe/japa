// Package fetcher implements the fetcher adapter framework for the
// Intelligence System. Five adapters (static, rss, api, pdf, ocr) implement
// the Fetcher interface. The scheduler selects the adapter by source.crawl_method.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 4)
package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Fetcher is the adapter interface. Each implementation fetches a source
// and returns the raw bytes + a normalized content hash for change detection.
type Fetcher interface {
	Fetch(ctx context.Context, source sources.Source) (FetchResult, error)
}

// FetchResult is the output of a successful fetch.
type FetchResult struct {
	HTTPStatus  int
	Headers     map[string]string
	ContentType string
	Body        []byte
	ContentHash string
}

// NormalizeForHash produces a stable hash key from raw bytes.
// Strips scripts/styles and collapses whitespace for HTML; for other
// content types, hashes the raw bytes.
func NormalizeForHash(body []byte, contentType string) string {
	h := sha256.Sum256(normalizeBody(body, contentType))
	return hex.EncodeToString(h[:])
}

func normalizeBody(body []byte, contentType string) []byte {
	if strings.Contains(contentType, "text/html") {
		return normalizeHTML(body)
	}

	return body
}
