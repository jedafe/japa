package probe

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

const (
	// DefaultVerifyMaxRequests is the verify-stage request budget (spec §2:
	// verify is the deeper pass — sitemap walk + category-path probes + feed
	// re-checks on top of the preview detector set). The worker wires the
	// verify prober with intel.probe.verify_max_requests; VerifyOpts can
	// override per call.
	DefaultVerifyMaxRequests = 25

	// DefaultVerifyMaxURLs caps how many page URLs the sitemap walk
	// collects before fetching (intel.probe.max_urls setting, spec §2
	// "walks sitemap"). Fetching is additionally bounded by the request
	// budget, so the effective crawl is min(maxURLs, remaining budget).
	DefaultVerifyMaxURLs = 50

	// maxVerifySitemaps bounds how many sitemap documents are fetched —
	// a sitemapindex can fan out to hundreds of children; the walk only
	// needs enough manifest to seed sub-page detection.
	maxVerifySitemaps = 4
)

// VerifyOpts tunes the deep verify pass (worker intel:probe_verify).
type VerifyOpts struct {
	// MaxRequests overrides the per-run request budget. <=0 falls back to
	// the prober's configured budget, raised to at least
	// DefaultVerifyMaxRequests.
	MaxRequests int
	// MaxURLs caps sitemap-walked page URLs. <=0 selects
	// DefaultVerifyMaxURLs.
	MaxURLs int
}

// Verify runs the deep post-approval probe (spec §2 "verify"): the full
// preview detector pass under a larger budget, then —
//
//   - re-checks every discovered feed URL with a real GET; feeds that no
//     longer answer with feed content are dropped (proposal + feed_urls)
//     and noted as a warning,
//   - GET-probes per-category discovery paths (/jobs, /careers, /blog,
//     /events … plus the conventional /feed, /rss) and runs the page-level
//     detectors on each hit — a JobPosting ld+json on /jobs produces a
//     channel for that path,
//   - walks discovered sitemaps (robots.txt declarations, probed
//     /sitemap.xml, sitemapindex children) collecting up to maxURLs page
//     URLs and running the same detectors on each — budget permitting.
//
// All fetches go through the injected safetransport client under the same
// atomic budget as the preview pass; exhaustion degrades to a warning, not
// an error.
func (p *Prober) Verify(ctx context.Context, rawURL, kind string, categories []string, opts VerifyOpts) (Report, error) {
	budget := opts.MaxRequests
	if budget <= 0 {
		budget = p.maxRequests
		if budget < DefaultVerifyMaxRequests {
			budget = DefaultVerifyMaxRequests
		}
	}

	maxURLs := opts.MaxURLs
	if maxURLs <= 0 {
		maxURLs = DefaultVerifyMaxURLs
	}

	return p.run(ctx, rawURL, kind, categories, budget, func(dctx context.Context, run *runState, page *FetchedPage, report *Report) {
		p.deepen(dctx, run, page, report, maxURLs)
	})
}

// deepen is the verify stage appended after the preview detectors: feed
// re-checks, per-category path probes, then the sitemap walk — in that
// order so cheap, high-signal checks claim budget before the walk.
func (p *Prober) deepen(ctx context.Context, run *runState, page *FetchedPage, out *Report, maxURLs int) {
	if !page.OK() {
		// No usable entry document — path probes and sitemap fetches would
		// almost certainly fail the same way; host-level signals already ran.
		return
	}

	run.recheckFeeds(ctx, out)
	run.probeCategoryPaths(ctx, page, out)
	run.walkSitemaps(ctx, page, maxURLs, out)
}

// verifyPageDetectors returns the content-level detector set run on
// verify-fetched sub-pages (category paths, sitemap URLs): feed detection
// without its own path probing (the path IS the probe), JSON-LD typing,
// and linked-document discovery. Platform/host, sitemap, api-sniff, SPA,
// and email-hint detectors are entry-page concerns — re-running them per
// sub-page would duplicate or mislead.
func (r *runState) verifyPageDetectors() []Detector {
	return []Detector{
		&feedDetector{run: r, noPathProbe: true},
		jsonldDetector{},
		documentsDetector{},
	}
}

// detectSubPage runs the verify page-detector set on a fetched sub-page
// and merges the results into out, attributing every signal to the
// sub-page's URL — a JobPosting on /jobs must produce a /jobs channel, not
// inflate the entry page's jsonld_types. Detectors write into a scratch
// report so proposals carry the sub-page URL; method evidence and feed
// URLs merge into the main report. Error pages are skipped silently.
func (r *runState) detectSubPage(ctx context.Context, page *FetchedPage, out *Report) {
	if page == nil || !page.OK() {
		return
	}

	r.markFetched(page.URL)
	r.markFetched(page.FinalURL)

	sub := &Report{}

	for _, d := range r.verifyPageDetectors() {
		d.Detect(ctx, page, sub)
	}

	for _, m := range sub.Methods {
		// jsonld proposals are re-emitted below with the sub-page URL —
		// merge only its method evidence here.
		out.addMethod(m.Method, m.Confidence, m.Evidence+" @ "+page.FinalURL)
	}

	for _, f := range sub.FeedURLs {
		if r.blockedDiscoveredURL(f) {
			// A sub-page <link> can re-smuggle a foreign private-literal
			// feed AFTER the recheck pass — same SSRF rule as sitemap locs.
			continue
		}

		if !slices.Contains(out.FeedURLs, f) {
			out.FeedURLs = append(out.FeedURLs, f)
		}
	}

	for _, prop := range sub.Channels {
		// Drop proposals whose entry point is a foreign private literal —
		// e.g. an rss proposal for a blocked feed URL (SSRF, review C1d).
		if r.blockedDiscoveredURL(prop.EntryURL) {
			continue
		}

		out.Channels = append(out.Channels, prop)
	}

	if len(sub.JSONLDTypes) > 0 {
		out.Channels = append(out.Channels, ChannelProposal{
			Label:       "JSON-LD structured data",
			EntryURL:    page.FinalURL,
			CrawlMethod: string(sources.CrawlMethodJSONLD),
			// Stamp the SUB-page's own detected types — leaving this empty
			// would let fillProposal fall back to out.JSONLDTypes, which is
			// entry-page-only by design; an uncategorized source's /jobs
			// JobPosting proposal would then carry the entry page's types
			// (or none → extract/curate skip at materialize).
			EntityTypes:    entityTypesFromJSONLD(sub.JSONLDTypes),
			Confidence:     weightJSONLD,
			ProviderConfig: map[string]any{"jsonld_types": sub.JSONLDTypes},
		})
	}
}

// blockedDiscoveredURL reports whether a content-discovered URL must not be
// fetched: its host is a private/reserved IP literal on a DIFFERENT host
// than the (already-fetched) entry page. Same-host literals pass — they are
// no riskier than the entry request itself, and tests serve fixtures from
// 127.0.0.1. Named hosts pass here regardless of what they resolve to —
// the fetcher safetransport's dial-time guard rejects private DNS answers.
// (intel-channels final review C1d).
func (r *runState) blockedDiscoveredURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := u.Hostname()
	if host == "" || strings.EqualFold(host, r.entryHost) {
		return false
	}

	return sources.IsPrivateIPLiteral(host)
}

// recheckFeeds GETs each discovered feed URL and drops the ones that no
// longer answer with feed content (dead link-guessed alternates, SPA
// catch-alls, 404s). Only a definitive response — non-2xx status, or a 2xx
// non-feed body — is evidence of death: transport failures and
// budget-exhausted skips keep the feed. Dropped feeds lose both their
// feed_urls entry and their rss channel proposal; methods evidence is
// left as detection history. Feeds pointing at foreign private-IP literals
// are dropped without a request (SSRF — a materialized channel would have
// been rejected at insert anyway).
func (r *runState) recheckFeeds(ctx context.Context, out *Report) {
	if len(out.FeedURLs) == 0 {
		return
	}

	live := make([]string, 0, len(out.FeedURLs))
	dead := map[string]bool{}

	for i, feedURL := range out.FeedURLs {
		if r.blockedDiscoveredURL(feedURL) {
			dead[feedURL] = true

			out.Warnings = append(out.Warnings,
				fmt.Sprintf("verify: feed %s targets a private/reserved address; dropped (SSRF)", feedURL))

			continue
		}

		page, err := r.get(ctx, feedURL)
		if errors.Is(err, errBudgetExhausted) {
			r.noteBudget(out)
			// Unchecked feeds keep their proposals — absence of a re-check
			// is not evidence of death.
			live = append(live, out.FeedURLs[i:]...)

			break
		}

		if err != nil {
			// Transport failure (DNS blip, timeout, reset) is not evidence
			// of death — only a definitive response is. Keep the feed like
			// the unchecked case and note the failure.
			live = append(live, feedURL)
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("verify: feed %s re-check failed (%v); kept", feedURL, err))

			continue
		}

		if !page.OK() || (!isFeedContent(page.ContentType) && !looksLikeFeedBody(page.Body)) {
			dead[feedURL] = true

			continue
		}

		live = append(live, feedURL)
	}

	if len(dead) == 0 {
		return
	}

	out.FeedURLs = live

	kept := out.Channels[:0]

	for _, prop := range out.Channels {
		if dead[prop.EntryURL] && prop.CrawlMethod == string(sources.CrawlMethodRSS) {
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("verify: feed %s did not re-check; proposal dropped", prop.EntryURL))

			continue
		}

		kept = append(kept, prop)
	}

	out.Channels = kept
}

// categoryDiscoveryPaths maps a category to the conventional site paths
// worth GET-probing during verify (spec §2: "/jobs, /blog, /careers when
// categories include job/news").
var categoryDiscoveryPaths = map[string][]string{
	"job":         {"/jobs", "/careers"},
	"news":        {"/news", "/blog"},
	"blog":        {"/blog"},
	"article":     {"/blog"},
	"event":       {"/events"},
	"scholarship": {"/scholarships"},
	"dataset":     {"/datasets", "/data"},
	"grant":       {"/grants"},
	"visa":        {"/visa", "/immigration"},
}

// alwaysProbePaths are probed regardless of category selection — a feed
// that neither the page head nor the preview shortlist found may still
// live at the canonical paths.
var alwaysProbePaths = []string{"/feed", "/rss"}

// probeCategoryPaths GET-probes the discovery paths implied by the
// source's categories on the entry origin and runs the verify page
// detectors on every hit.
func (r *runState) probeCategoryPaths(ctx context.Context, page *FetchedPage, out *Report) {
	u, err := url.Parse(page.FinalURL)
	if err != nil || u.Host == "" {
		return
	}

	origin := u.Scheme + "://" + u.Host

	paths := append([]string{}, alwaysProbePaths...)
	for _, cat := range r.categories {
		paths = append(paths, categoryDiscoveryPaths[strings.ToLower(strings.TrimSpace(cat))]...)
	}

	seen := map[string]bool{}

	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}

		seen[p] = true

		candidate := origin + p
		if r.wasFetched(candidate) {
			continue // already detector-analyzed this run
		}

		sub, err := r.get(ctx, candidate)
		if errors.Is(err, errBudgetExhausted) {
			r.noteBudget(out)

			return
		}

		if err != nil {
			continue
		}

		r.detectSubPage(ctx, sub, out)
	}
}

// sitemapDoc is the minimal XML shape shared by urlset and sitemapindex
// manifests — the walk only needs <loc> values.
type sitemapDoc struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
	Children []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

// parseSitemapLocs extracts page URLs from a urlset and child-sitemap URLs
// from a sitemapindex.
func parseSitemapLocs(body []byte) (pageURLs, childSitemaps []string) {
	var doc sitemapDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, nil
	}

	for _, u := range doc.URLs {
		if loc := strings.TrimSpace(u.Loc); loc != "" {
			pageURLs = append(pageURLs, loc)
		}
	}

	for _, s := range doc.Children {
		if loc := strings.TrimSpace(s.Loc); loc != "" {
			childSitemaps = append(childSitemaps, loc)
		}
	}

	return pageURLs, childSitemaps
}

// walkSitemaps fetches discovered sitemap manifests (bounded by
// maxVerifySitemaps documents, one level of sitemapindex expansion via a
// breadth-first queue), collects up to maxURLs page URLs, and runs the
// verify page detectors on each walked page while the budget lasts.
func (r *runState) walkSitemaps(ctx context.Context, page *FetchedPage, maxURLs int, out *Report) {
	if len(out.SitemapURLs) == 0 {
		return
	}

	var (
		pageURLs []string
		fetched  int
		blocked  int
	)

	seen := map[string]bool{}
	queue := slices.Clone(out.SitemapURLs)

	for len(queue) > 0 && fetched < maxVerifySitemaps && len(pageURLs) < maxURLs {
		smURL := queue[0]
		queue = queue[1:]

		if seen[smURL] {
			continue
		}

		seen[smURL] = true

		if r.blockedDiscoveredURL(smURL) {
			// robots.txt Sitemap: declarations and sitemapindex <loc>s are
			// attacker-controlled — a foreign private-IP literal must never
			// be dialed (SSRF, review C1d).
			blocked++

			continue
		}

		doc, err := r.get(ctx, smURL)
		if errors.Is(err, errBudgetExhausted) {
			r.noteBudget(out)

			break
		}

		if err != nil || !doc.OK() {
			continue
		}

		fetched++

		urls, children := parseSitemapLocs(doc.Body)
		pageURLs = append(pageURLs, urls...)
		queue = append(queue, children...)
	}

	if len(pageURLs) > maxURLs {
		pageURLs = pageURLs[:maxURLs]
		out.Warnings = append(out.Warnings,
			fmt.Sprintf("verify: sitemap walk capped at %d URLs", maxURLs))
	}

	for _, u := range pageURLs {
		if r.wasFetched(u) {
			continue // already detector-analyzed (entry or category path)
		}

		if r.blockedDiscoveredURL(u) {
			// A hostile sitemap can name internal addresses in <loc> —
			// never fetch them (SSRF, review C1d).
			blocked++

			continue
		}

		sub, err := r.get(ctx, u)
		if errors.Is(err, errBudgetExhausted) {
			r.noteBudget(out)

			return
		}

		if err != nil {
			continue
		}

		r.detectSubPage(ctx, sub, out)
	}

	if blocked > 0 {
		out.Warnings = append(out.Warnings,
			fmt.Sprintf("verify: sitemap walk skipped %d private/reserved-address URLs (SSRF)", blocked))
	}
}
