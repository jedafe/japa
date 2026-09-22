package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

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
	static  *StaticFetcher
	rss     *RSSFetcher
	api     *APIFetcher
	render  *RenderFetcher
	git     *GitFetcher
	social  *SocialFetcher
	wayback *WaybackFetcher
	search  *SearchFetcher
}

// NewComposite returns a Composite fetcher with all adapters wired, each
// backed by an SSRF-safe client (dial-time private-IP guard + redirect
// guard). settings feeds the config-gated adapters (render service URL,
// social provider allowlist) — nil leaves them disabled; rdb backs the
// shared render circuit breaker (nil = no circuit).
func NewComposite(settings AdapterSettings, rdb valkeycompat.Cmdable) *Composite {
	return &Composite{
		static: NewStaticFetcher(),
		rss:    NewRSSFetcher(),
		api:    NewAPIFetcher(),
		// The render service is operator-deployed infra — typically a
		// private/internal host — so its client skips the private-IP dial
		// guard (AllowPrivate); the service URL is admin-configured, not
		// attacker-controlled.
		render: NewRenderFetcher(
			NewSafeClientOpts(60*time.Second, defaultMaxBodySize, SafeClientOpts{AllowPrivate: true}),
			settings, rdb),
		git:     NewGitFetcher(),
		social:  NewSocialFetcher(nil, settings),
		wayback: NewWaybackFetcher(),
		search:  NewSearchFetcher(nil, settings),
	}
}

// NewCompositeWithClient returns a Composite whose HTTP adapters all share
// the given client — the test-only opt-out for httptest fixtures bound to
// 127.0.0.1. Production wiring uses NewComposite (SSRF-safe clients).
// Tests typically pass NewSafeClientOpts(…, SafeClientOpts{AllowPrivate: true})
// or the fixture's srv.Client().
func NewCompositeWithClient(client *http.Client, settings AdapterSettings, rdb valkeycompat.Cmdable) *Composite {
	return &Composite{
		static:  &StaticFetcher{client: client},
		rss:     &RSSFetcher{client: client},
		api:     &APIFetcher{client: client},
		render:  NewRenderFetcher(client, settings, rdb),
		git:     NewGitFetcher(),
		social:  NewSocialFetcher(client, settings),
		wayback: &WaybackFetcher{client: client},
		search:  NewSearchFetcher(client, settings),
	}
}

// pendingFetchMethods are the valid channel crawl methods whose fetch
// adapter has not landed — Composite.Fetch answers
// ErrMethodNotImplemented for them (the switch's default arm covers the
// same set; both must stay in sync). Inbound methods (email/webhook) are
// deliberately absent: they are never fetched but are valid channel rows.
// Phase 3 emptied the set; the probe filter below keeps working against
// any future pending method.
var pendingFetchMethods = map[sources.CrawlMethod]bool{}

// PendingFetchMethod reports whether m is a known channel crawl method whose
// fetch adapter is a Phase-3 stub (Composite.Fetch → ErrMethodNotImplemented).
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
// Dispatch (channel design §3):
//   - static/pdf/ocr/oneshot → StaticFetcher (pdf/ocr bytes only; text
//     extraction is a downstream concern; oneshot is a plain page fetch)
//   - ical/tabular → StaticFetcher byte retrieval; parsing is the curator's
//     job — fetch=bytes, curate=parse
//   - rss → RSSFetcher
//   - api → APIFetcher (provider_config resolved channel-first)
//   - jsonld → static fetch + ld+json block extraction
//   - sitemap → static fetch + URL/lastmod manifest
//   - render → RenderFetcher (headless service; ErrAdapterDisabled when
//     intel.render.service_url is unset or the circuit is open)
//   - git → GitFetcher (in-memory clone / tag ls-remote)
//   - social → SocialFetcher (provider allowlist via
//     intel.social.enabled_providers)
//   - wayback → WaybackFetcher (CDX snapshot manifest)
//   - search → SearchFetcher (SERP poll → NDJSON result page)
//   - manual/email/webhook/xhr_sniff → hard error (never fetched — inbound
//     or task-owned; ListDueChannels excludes them)
//
// On a fetch error, provider_config.fallback_methods is tried in order
// (spec §3 degradation ladder — e.g. rss → jsonld → render → static):
// each hop rewrites the method on the effective source and re-dispatches.
// ErrAdapterDisabled and config/validation errors do not trigger the
// ladder — they are deterministic, not transient.
func (c *Composite) Fetch(ctx context.Context, src sources.Source, ch sources.Channel) (FetchResult, error) {
	eff := src
	eff.SourceUrl = ch.EntryUrl
	eff.CrawlMethod = ch.CrawlMethod
	eff.ProviderConfig = ResolveChannelProviderConfig(ch, src)

	// Wayback incremental window (spec §3): when neither the channel's nor
	// the source's provider_config sets `since`, inject the channel's
	// last_success_at so the CDX manifest lists only captures newer than
	// the last successful crawl. Applies when wayback is the primary
	// method or any hop of the fallback ladder.
	if ch.LastSuccessAt.Valid && waybackInPlay(ch.CrawlMethod, eff.ProviderConfig) {
		eff.ProviderConfig = injectWaybackSince(eff.ProviderConfig, ch.LastSuccessAt.Time)
	}

	result, err := c.fetchMethod(ctx, eff, sources.CrawlMethod(ch.CrawlMethod))
	if err == nil {
		return result, nil
	}
	if errors.Is(err, ErrMethodNotImplemented) {
		return FetchResult{}, err
	}

	// Degradation ladder (spec §3): try each configured fallback method in
	// order — bounded, no repeats. ErrAdapterDisabled on the primary also
	// enters the ladder: render-unconfigured → static is exactly the
	// designed degradation; if every hop is disabled/fails the ORIGINAL
	// error is returned, preserving its ErrAdapterDisabled benign-skip
	// semantics.
	seen := map[string]bool{ch.CrawlMethod: true}
	hops := 0
	for _, m := range fallbackMethods(eff.ProviderConfig) {
		if hops >= 3 {
			break
		}
		if seen[m] {
			continue
		}
		seen[m] = true
		hops++

		eff.CrawlMethod = m
		result, ferr := c.fetchMethod(ctx, eff, sources.CrawlMethod(m))
		if ferr == nil {
			return result, nil
		}
		if errors.Is(ferr, ErrAdapterDisabled) {
			continue // config-gated hop — try the next one
		}
	}

	return FetchResult{}, err
}

// fetchMethod dispatches one concrete method — extracted so the fallback
// ladder can reuse it without re-resolving the effective source.
func (c *Composite) fetchMethod(ctx context.Context, eff sources.Source, method sources.CrawlMethod) (FetchResult, error) {
	switch method {
	case sources.CrawlMethodStatic, sources.CrawlMethodPDF, sources.CrawlMethodOCR,
		sources.CrawlMethodICal, sources.CrawlMethodTabular, sources.CrawlMethodOneshot:
		return c.static.Fetch(ctx, eff)
	case sources.CrawlMethodRSS:
		return c.rss.Fetch(ctx, eff)
	case sources.CrawlMethodAPI:
		return c.api.Fetch(ctx, eff)
	case sources.CrawlMethodJSONLD:
		return c.fetchJSONLD(ctx, eff)
	case sources.CrawlMethodSitemap:
		return c.fetchSitemapManifest(ctx, eff)
	case sources.CrawlMethodRender:
		return c.render.Fetch(ctx, eff)
	case sources.CrawlMethodGit:
		return c.git.Fetch(ctx, eff)
	case sources.CrawlMethodSocial:
		return c.social.Fetch(ctx, eff)
	case sources.CrawlMethodWayback:
		return c.wayback.Fetch(ctx, eff)
	case sources.CrawlMethodSearch:
		return c.search.Fetch(ctx, eff)
	case sources.CrawlMethodManual, sources.CrawlMethodEmail, sources.CrawlMethodWebhook,
		sources.CrawlMethodXHRSniff:
		return FetchResult{}, fmt.Errorf("fetcher: crawl_method %q is not fetchable (manual/inbound/task)", method)
	default:
		return FetchResult{}, fmt.Errorf("%w: %q", ErrMethodNotImplemented, method)
	}
}

// fallbackMethods reads provider_config.fallback_methods — a []string of
// crawl methods to try when the primary fetch fails. Non-fetchable methods
// are validated out: manual/inbound (email, webhook), task-owned
// (xhr_sniff), and trigger-only (oneshot) methods can never produce a fetch
// result, so a ladder entry naming one would only waste a hop.
func fallbackMethods(providerConfig []byte) []string {
	var cfg struct {
		FallbackMethods []string `json:"fallback_methods"`
	}
	if err := json.Unmarshal(providerConfig, &cfg); err != nil {
		return nil
	}

	out := cfg.FallbackMethods[:0]
	for _, m := range cfg.FallbackMethods {
		method := sources.CrawlMethod(m)
		if !method.IsValid() || !fetchableMethod(method) {
			continue
		}
		out = append(out, m)
	}

	return out
}

// fetchableMethod reports whether a crawl method can produce a fetch
// result — excludes the non-fetchable set (manual, inbound email/webhook,
// task-owned xhr_sniff, trigger-only oneshot) that fetchMethod hard-errors
// on or that are semantically meaningless as fallbacks.
func fetchableMethod(m sources.CrawlMethod) bool {
	switch m {
	case sources.CrawlMethodManual, sources.CrawlMethodEmail, sources.CrawlMethodWebhook,
		sources.CrawlMethodXHRSniff, sources.CrawlMethodOneshot:
		return false
	}

	return true
}

// waybackInPlay reports whether the wayback adapter can be reached on this
// fetch — as the primary method or via the validated fallback ladder.
func waybackInPlay(primary string, providerConfig []byte) bool {
	if sources.CrawlMethod(primary) == sources.CrawlMethodWayback {
		return true
	}
	for _, m := range fallbackMethods(providerConfig) {
		if m == string(sources.CrawlMethodWayback) {
			return true
		}
	}

	return false
}

// injectWaybackSince merges "since" (CDX timestamp) into a provider_config
// when the key is absent — an explicit provider_config.since always wins.
func injectWaybackSince(providerConfig []byte, t time.Time) []byte {
	var m map[string]any
	if err := json.Unmarshal(providerConfig, &m); err != nil || m == nil {
		m = map[string]any{}
	}
	if _, ok := m["since"]; ok {
		return providerConfig
	}
	m["since"] = t.UTC().Format("20060102150405")

	b, err := json.Marshal(m)
	if err != nil {
		return providerConfig
	}

	return b
}
