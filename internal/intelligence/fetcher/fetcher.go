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
	"errors"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// ErrMethodNotImplemented is returned by Composite.Fetch when the channel's
// crawl_method names a fetch strategy that has no Phase-1 adapter yet
// (render, git, social, search, wayback, xhr_sniff, oneshot). The worker
// treats it like any fetch failure — the channel's consecutive_failures
// counter increments and the circuit-breaker disables the channel at the
// threshold, so unimplemented methods cannot retry-loop forever.
var ErrMethodNotImplemented = errors.New("fetcher: crawl method not implemented")

// Fetcher is the adapter interface. Each implementation fetches a channel's
// entry URL on behalf of its parent source and returns the raw bytes + a
// normalized content hash for change detection. The source supplies
// trust/license context (preflight) and legacy provider_config fallbacks;
// the channel supplies the entry URL, crawl method, and per-channel
// provider_config.
type Fetcher interface {
	Fetch(ctx context.Context, src sources.Source, ch sources.Channel) (FetchResult, error)
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
