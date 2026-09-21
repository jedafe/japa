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
// crawl_method names a fetch strategy with no adapter yet (search — Phase
// 3). The worker treats it like any fetch failure — the channel's
// consecutive_failures counter increments and the circuit-breaker disables
// the channel at the threshold, so unimplemented methods cannot retry-loop
// forever.
var ErrMethodNotImplemented = errors.New("fetcher: crawl method not implemented")

// ErrAdapterDisabled is returned by adapters that exist but are not
// configured or enabled — render with no intel.render.service_url (or an
// open jc:intel:render:circuit breaker), social with a provider absent
// from intel.social.enabled_providers. The worker treats it like
// ErrCrawlLocked: a benign skip that neither retries nor increments
// consecutive_failures — a missing config is not a source failure.
var ErrAdapterDisabled = errors.New("fetcher: adapter not configured")

// AdapterSettings is the hot-reloadable settings port the Phase-2 adapters
// consult per fetch (render service URL, social provider allowlist).
// *config.Settings satisfies it; a nil reader leaves every config-gated
// adapter disabled. Defined locally to avoid an import cycle on
// internal/config.
type AdapterSettings interface {
	GetStringOr(ctx context.Context, key, fallback string) string
	GetIntOr(ctx context.Context, key string, fallback int) int
}

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
