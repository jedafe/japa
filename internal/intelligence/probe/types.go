// Package probe implements the intelligence source auto-detection probe
// (intel-channels design §2): it inspects a candidate source URL and
// proposes intel_source_channels rows — entry URL, crawl method, pipeline,
// entity types — ranked by detection confidence.
//
// The preview stage (Prober.Preview) is a bounded, single-shot inspection:
// one entry GET, one robots.txt fetch, and a few HEAD probes under a shared
// request budget. It powers the live propose-form UX behind
// POST /api/v1/sources/probe. The deeper verify stage runs as the
// intel:probe_verify worker task and reuses the same Detector set.
//
// SSRF hardening: every outbound request goes through the *http.Client
// injected into NewProber — callers MUST wire the fetcher package's
// SSRF-safe client (fetcher.NewSafeClient). The prober never constructs its
// own transport; a nil client makes Preview fail fast instead of fetching
// unsafely.
//
// Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md §2
package probe

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"golang.org/x/net/html"
)

// MethodChannelSync is the pseudo-method marking a video_channel proposal.
// It is deliberately NOT a sources.CrawlMethod — it matches the
// "channel_sync" entry in the category→method map (video category) and
// routes the proposal to the §106 video_channel source flow
// (video:sync_channels owns ingestion). It must never be inserted into
// intel_source_channels.crawl_method; the platform is recorded in
// ChannelProposal.ProviderConfig["platform"].
const MethodChannelSync = "channel_sync"

// Report is the output of a probe run (spec §2 ProbeReport): the detected
// capabilities of the candidate URL plus ranked channel proposals the
// materializer turns into intel_source_channels rows.
type Report struct {
	// FinalURL is the entry URL after redirects.
	FinalURL string `json:"final_url"`
	// DetectedKinds lists platform signatures matched on the URL host
	// (youtube, medium, github, reddit, telegram…).
	DetectedKinds []string `json:"detected_kinds,omitempty"`
	// Methods is the deduplicated set of detected crawl methods, ranked by
	// confidence, each carrying the evidence that produced it.
	Methods []DetectedMethod `json:"methods"`
	// Channels is the ranked channel proposals. Platform/inbound/discovery
	// proposals bypass the category-method intersection (see bypassMethods);
	// everything else is proposed only when its method is listed by the
	// resolved category map (or no categories were selected).
	Channels []ChannelProposal `json:"channels"`
	// JSONLDTypes is every schema.org @type found in ld+json blocks.
	JSONLDTypes []string `json:"jsonld_types,omitempty"`
	// FeedURLs lists discovered RSS/Atom/JSON-feed endpoints.
	FeedURLs []string `json:"feed_urls,omitempty"`
	// SitemapURLs lists discovered sitemap endpoints (robots.txt
	// declarations and the probed /sitemap.xml).
	SitemapURLs []string `json:"sitemap_urls,omitempty"`
	// HasWebSub reports a WebSub (PubSubHubbub) hub advertisement — either a
	// Link response header or a <link rel="hub"> tag.
	HasWebSub bool `json:"has_websub"`
	// RequiresJS reports SPA markers (empty shell, #root/#app mounts,
	// framework signatures) — the page needs a JS render pass.
	RequiresJS bool `json:"requires_js"`
	// Robots is the robots.txt verdict for the entry origin.
	Robots RobotVerdict `json:"robots"`
	// Warnings lists non-fatal probe issues: HTTP errors on the entry URL,
	// request-budget exhaustion, category-map resolution notes.
	Warnings []string `json:"warnings,omitempty"`
	// ProbedAt is when the probe ran (UTC).
	ProbedAt time.Time `json:"probed_at"`
}

// addMethod records a detected method, merging repeated evidence and keeping
// the highest confidence seen.
func (r *Report) addMethod(method string, confidence float64, evidence string) {
	for i := range r.Methods {
		if r.Methods[i].Method == method {
			if confidence > r.Methods[i].Confidence {
				r.Methods[i].Confidence = confidence
			}

			if evidence != "" && r.Methods[i].Evidence != evidence {
				if r.Methods[i].Evidence != "" {
					r.Methods[i].Evidence += "; "
				}

				r.Methods[i].Evidence += evidence
			}

			return
		}
	}

	r.Methods = append(r.Methods, DetectedMethod{
		Method:     method,
		Confidence: confidence,
		Evidence:   evidence,
	})
}

// addKind records a platform signature (youtube, medium, …) once.
func (r *Report) addKind(kind string) {
	for _, k := range r.DetectedKinds {
		if k == kind {
			return
		}
	}

	r.DetectedKinds = append(r.DetectedKinds, kind)
}

// DetectedMethod is one crawl method the probe detected, with a confidence
// weight and the evidence that produced it.
type DetectedMethod struct {
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
}

// ChannelProposal is a proposed intel_source_channels row (spec §2). It is a
// plain struct — the materializer (worker intel:probe_verify task) mirrors
// it into its own input type so this package stays free of store deps.
type ChannelProposal struct {
	Label       string   `json:"label"`
	EntryURL    string   `json:"entry_url"`
	CrawlMethod string   `json:"crawl_method"`
	EntityTypes []string `json:"entity_types,omitempty"`
	// Pipeline is extract|curate|hybrid.
	Pipeline   string  `json:"pipeline"`
	Confidence float64 `json:"confidence"`
	// ProviderConfig carries per-method config seeds (feed hub, jsonld_types,
	// provider/platform markers). Proposals with CrawlMethod ==
	// MethodChannelSync carry ProviderConfig["platform"] and route to the
	// §106 video_channel flow instead of producing a channel row.
	ProviderConfig map[string]any `json:"provider_config,omitempty"`
	// RelevanceTopics seeds channel.relevance_topics from the resolved
	// category map (deterministic relevance gate, spec §4).
	RelevanceTopics []string `json:"relevance_topics,omitempty"`
}

// RobotVerdict is the probe-level summary of the origin's robots.txt — the
// fetcher's RobotsChecker equivalent without the Valkey cache (the probe is
// a one-shot inspection; caching belongs to the scheduler path).
type RobotVerdict struct {
	// Fetched is true when robots.txt returned HTTP 200 and was parsed.
	Fetched bool `json:"fetched"`
	// Allowed is the fail-open default — true unless robots.txt disallows
	// the site root for our/wildcard user agent.
	Allowed         bool     `json:"allowed"`
	CrawlDelaySecs  int      `json:"crawl_delay_secs,omitempty"`
	DisallowedPaths []string `json:"disallowed_paths,omitempty"`
	// SitemapURLs collects every Sitemap: declaration regardless of the
	// user-agent block it appears in.
	SitemapURLs []string `json:"sitemap_urls,omitempty"`
}

// FetchedPage is one fetched document shared by all detectors of a run. The
// HTML parse is lazy — detectors call HTML() and get nil for non-HTML or
// unparseable bodies.
type FetchedPage struct {
	// URL is the requested URL.
	URL string
	// FinalURL is the URL after redirects (resp.Request.URL).
	FinalURL string
	// StatusCode is the HTTP status of the response.
	StatusCode int
	// Headers are the response headers.
	Headers http.Header
	// ContentType is the response Content-Type header (with parameters).
	ContentType string
	// Body is the response body, capped at maxBodyBytes.
	Body []byte

	parsed bool
	doc    *html.Node
}

// OK reports whether the response was a 2xx — content detectors skip error
// pages so a 404's HTML does not produce false signals.
func (p *FetchedPage) OK() bool {
	return p.StatusCode >= http.StatusOK && p.StatusCode < http.StatusMultipleChoices
}

// HTML returns the parsed document tree, or nil when the body is not HTML or
// failed to parse. Parsing happens once on first call.
func (p *FetchedPage) HTML() *html.Node {
	if !p.parsed {
		p.parsed = true

		if p.OK() && len(p.Body) > 0 && isHTMLContent(p.ContentType) {
			if doc, err := html.Parse(bytes.NewReader(p.Body)); err == nil {
				p.doc = doc
			}
		}
	}

	return p.doc
}

// Detector inspects the fetched entry page to contribute detected methods
// and channel proposals to the report. Detectors that need extra outbound
// requests (HEAD probes of well-known paths) hold the run's shared state
// and spend from its request budget; page-only detectors are stateless.
//
// Implementations must be tolerant of partial input: page.Body may be an
// error page, page.HTML() may return nil.
type Detector interface {
	Detect(ctx context.Context, page *FetchedPage, out *Report)
}
