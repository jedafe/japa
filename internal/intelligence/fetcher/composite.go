package fetcher

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Composite is the fetcher adapter that dispatches to the correct per-method
// fetcher based on the channel's crawl_method. PDF and OCR methods use the
// StaticFetcher to download raw bytes; text extraction happens in the
// extract step. It is the single Fetcher implementation wired into the worker
// via RegisterIntelDependencies.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 4).
type Composite struct {
	static *StaticFetcher
	rss    *RSSFetcher
	api    *APIFetcher
}

// NewComposite returns a Composite fetcher with all adapters wired, each
// backed by an SSRF-safe client (dial-time private-IP guard + redirect
// guard).
func NewComposite() *Composite {
	return &Composite{
		static: NewStaticFetcher(),
		rss:    NewRSSFetcher(),
		api:    NewAPIFetcher(),
	}
}

// NewCompositeWithClient returns a Composite whose adapters all share the
// given HTTP client — the test-only opt-out for httptest fixtures bound to
// 127.0.0.1. Production wiring uses NewComposite (SSRF-safe clients).
// Tests typically pass NewSafeClientOpts(…, SafeClientOpts{AllowPrivate: true})
// or the fixture's srv.Client().
func NewCompositeWithClient(client *http.Client) *Composite {
	return &Composite{
		static: &StaticFetcher{client: client},
		rss:    &RSSFetcher{client: client},
		api:    &APIFetcher{client: client},
	}
}

// pendingFetchMethods are the valid channel crawl methods whose fetch
// adapters land in Phase 2 — Composite.Fetch answers
// ErrMethodNotImplemented for them (the switch's default arm covers the
// same set; both must stay in sync). Inbound methods (email/webhook) are
// deliberately absent: they are never fetched but are valid channel rows.
var pendingFetchMethods = map[sources.CrawlMethod]bool{
	sources.CrawlMethodRender:   true,
	sources.CrawlMethodGit:      true,
	sources.CrawlMethodSocial:   true,
	sources.CrawlMethodSearch:   true,
	sources.CrawlMethodWayback:  true,
	sources.CrawlMethodXHRSniff: true,
	sources.CrawlMethodOneshot:  true,
}

// PendingFetchMethod reports whether m is a known channel crawl method whose
// fetch adapter is a Phase-2 stub (Composite.Fetch → ErrMethodNotImplemented).
// The intel:probe_verify worker filters these out of channel proposals so an
// approved source never gains a channel that can only ever fail.
func PendingFetchMethod(m string) bool {
	return pendingFetchMethods[sources.CrawlMethod(m)]
}

// Fetch selects the adapter by ch.CrawlMethod and delegates. The concrete
// adapters still take a sources.Source, so the channel is folded into an
// "effective" source: entry_url → SourceUrl, crawl_method → CrawlMethod,
// and provider_config resolved channel-first (ResolveChannelProviderConfig)
// so a probe-materialized API channel uses its own endpoint while a
// backfilled default channel ('{}' config) inherits the source's.
//
// Phase-1 dispatch (channel design §4):
//   - static/pdf/ocr → StaticFetcher (pdf/ocr bytes only; text extraction
//     is a downstream concern)
//   - ical/tabular → StaticFetcher byte retrieval; parsing is the curator's
//     job (Task 9) — fetch=bytes, curate=parse
//   - rss → RSSFetcher
//   - api → APIFetcher (provider_config resolved channel-first)
//   - jsonld → static fetch + ld+json block extraction
//   - sitemap → static fetch + URL/lastmod manifest
//   - manual/email/webhook → hard error (never fetchable — the scheduler's
//     ListDueChannels already excludes them; this is the stray-enqueue guard)
//   - render/git/social/search/wayback/xhr_sniff/oneshot →
//     ErrMethodNotImplemented until their Phase-2 adapters land
func (c *Composite) Fetch(ctx context.Context, src sources.Source, ch sources.Channel) (FetchResult, error) {
	eff := src
	eff.SourceUrl = ch.EntryUrl
	eff.CrawlMethod = ch.CrawlMethod
	eff.ProviderConfig = ResolveChannelProviderConfig(ch, src)

	switch sources.CrawlMethod(ch.CrawlMethod) {
	case sources.CrawlMethodStatic, sources.CrawlMethodPDF, sources.CrawlMethodOCR,
		sources.CrawlMethodICal, sources.CrawlMethodTabular:
		return c.static.Fetch(ctx, eff)
	case sources.CrawlMethodRSS:
		return c.rss.Fetch(ctx, eff)
	case sources.CrawlMethodAPI:
		return c.api.Fetch(ctx, eff)
	case sources.CrawlMethodJSONLD:
		return c.fetchJSONLD(ctx, eff)
	case sources.CrawlMethodSitemap:
		return c.fetchSitemapManifest(ctx, eff)
	case sources.CrawlMethodManual, sources.CrawlMethodEmail, sources.CrawlMethodWebhook:
		return FetchResult{}, fmt.Errorf("fetcher: crawl_method %q is not fetchable (manual/inbound)", ch.CrawlMethod)
	default:
		return FetchResult{}, fmt.Errorf("%w: %q", ErrMethodNotImplemented, ch.CrawlMethod)
	}
}
