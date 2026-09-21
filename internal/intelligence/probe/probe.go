package probe

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

const (
	// DefaultMaxRequests is the default preview request budget — entry GET +
	// robots.txt + up to three detector probes (spec §2 "Budget ~5
	// requests"). The verify stage passes a larger budget via NewProber.
	DefaultMaxRequests = 5

	// maxBodyBytes caps each fetched body. The probe needs the document head
	// and links, not multi-megabyte pages; the safetransport client callers
	// inject already applies its own cap on top of this.
	maxBodyBytes = 4 * 1024 * 1024

	// userAgent identifies the probe on outbound requests.
	userAgent = "JapaConnect-Intelligence-Probe/1.0 (+https://japaconnect.com)"
)

// errBudgetExhausted is returned by runState fetch helpers when the shared
// request budget is spent — detectors treat it as "skip this probe" rather
// than a failure.
var errBudgetExhausted = errors.New("probe: request budget exhausted")

// errBodyTooLarge mirrors fetcher.ErrBodyTooLarge so the probe does not need
// to import the fetcher package solely for the sentinel.
var errBodyTooLarge = errors.New("probe: response body exceeds max size")

// Confidence weights per spec §2 step 3 ("confidence = detector weight").
const (
	weightChannelSync = 0.98 // platform signature — host is ground truth
	weightFeed        = 0.95
	weightJSONLD      = 0.9
	weightAPIEntry    = 0.8 // entry URL is itself a JSON endpoint
	weightWebSub      = 0.8
	weightSocial      = 0.8 // platform social adapters (reddit, telegram)
	weightGit         = 0.85
	weightICal        = 0.75
	weightSitemap     = 0.7
	weightAPIProbe    = 0.65 // well-known OpenAPI path responded
	weightRender      = 0.6
	weightDocs        = 0.5
	weightEmailHint   = 0.4
	weightStatic      = 0.3 // fallback
)

// Prober runs source-URL probes. It is stateless across runs — per-run state
// (request budget, resolved category map) lives on runState.
type Prober struct {
	client      *http.Client
	maxRequests int
}

// NewProber returns a Prober. client MUST be SSRF-hardened — wire
// fetcher.NewSafeClient(timeout, maxBody) — every probe request (entry,
// robots.txt, detector HEAD probes) goes through it, including redirect
// validation. maxRequests <= 0 selects DefaultMaxRequests.
func NewProber(client *http.Client, maxRequests int) *Prober {
	if maxRequests <= 0 {
		maxRequests = DefaultMaxRequests
	}

	return &Prober{client: client, maxRequests: maxRequests}
}

// Preview runs the bounded single-shot probe over rawURL: entry GET,
// robots.txt, then the Detector set — all under one shared request budget.
// kind is the submitter's source_kind hint (e.g. "channel_profile") used for
// advisory warnings; categories selects which crawl methods may be proposed
// via the sources.ResolveCategoryMap intersection (empty = no restriction).
//
// Transport-level failures (DNS, connect, SSRF rejection by the injected
// client) abort the probe and propagate the error. HTTP error statuses do
// NOT abort — URL/host-level detection (platform signatures) still runs and
// the status is recorded as a warning.
func (p *Prober) Preview(ctx context.Context, rawURL, kind string, categories []string) (Report, error) {
	return p.run(ctx, rawURL, kind, categories, p.maxRequests, nil)
}

// run is the shared probe driver behind Preview and Verify: validate the
// entry URL, seed the run state with the request budget, fetch entry +
// robots.txt, run the detector set, then let an optional deep pass (verify)
// spend the remaining budget before finalize ranks the proposals.
func (p *Prober) run(ctx context.Context, rawURL, kind string, categories []string, budget int, deep func(context.Context, *runState, *FetchedPage, *Report)) (Report, error) {
	if p.client == nil {
		return Report{}, fmt.Errorf("probe: %w: http client (inject fetcher.NewSafeClient)", errpkg.ErrRequired)
	}

	entryURL, err := normalizeURL(rawURL)
	if err != nil {
		return Report{}, err
	}

	run := &runState{
		client:     p.client,
		kind:       kind,
		categories: categories,
		resolved:   sources.ResolveCategoryMap(nil, categories),
	}
	run.budget.Store(int64(budget))

	report := Report{
		Robots:   RobotVerdict{Allowed: true},
		ProbedAt: time.Now().UTC(),
		Warnings: slices.Clone(run.resolved.Warnings),
	}

	page, err := run.get(ctx, entryURL)
	if err != nil {
		return Report{}, fmt.Errorf("probe: fetch %q: %w", entryURL, err)
	}

	run.markFetched(entryURL)
	run.markFetched(page.FinalURL)

	// Record the entry hostname (post-redirect) so the verify deep pass can
	// tell same-host discovered URLs — which are as trustworthy as the
	// entry fetch itself — from foreign private-IP literals smuggled into
	// sitemaps/feed links (SSRF, review C1d).
	if u, uerr := url.Parse(page.FinalURL); uerr == nil {
		run.entryHost = u.Hostname()
	}

	report.FinalURL = page.FinalURL
	if !page.OK() {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("entry URL returned HTTP %d; detection limited to URL/host signals", page.StatusCode))
	}

	run.detectRobots(ctx, page, &report)

	if page.OK() {
		for _, d := range run.detectors() {
			d.Detect(ctx, page, &report)
		}
	} else {
		// Error pages carry no usable content signals — only the URL-level
		// platform signature still applies.
		platformDetector{}.Detect(ctx, page, &report)
	}

	if deep != nil {
		deep(ctx, run, page, &report)
	}

	run.finalize(&report, page)

	return report, nil
}

// runState carries per-probe shared state: the injected client, the
// remaining request budget (atomic so detectors may run concurrently), the
// submitter's source_kind hint, the selected categories (verify uses them
// for per-category discovery-path probing), and the resolved
// category→method map used to rank and filter proposals.
type runState struct {
	client     *http.Client
	budget     atomic.Int64
	kind       string
	categories []string
	resolved   sources.ResolvedMap

	// fetchedPages tracks page URLs already run through detectors so the
	// verify deep pass does not re-fetch a URL it already analyzed (e.g. a
	// category path also listed in the sitemap). Detectors run
	// sequentially, so a plain map suffices.
	fetchedPages map[string]bool
	warnedBudget atomic.Bool

	// entryHost is the entry page's post-redirect hostname — the baseline
	// the verify deep pass compares discovered URLs against: a private-IP
	// literal on a DIFFERENT hostname is an SSRF attempt and is skipped
	// before any request is spent. Same-hostname literals are as safe as
	// the entry fetch itself — and keep httptest fixtures working (a
	// production entry is never a private literal; the fetcher dial guard
	// would refuse it anyway).
	entryHost string
}

// spend consumes one request from the budget; false when exhausted.
func (r *runState) spend() bool {
	for {
		n := r.budget.Load()
		if n <= 0 {
			return false
		}

		if r.budget.CompareAndSwap(n, n-1) {
			return true
		}
	}
}

// markFetched records a page URL as already detector-analyzed; returns
// true when it was already marked. wasFetched is the non-mutating check
// the verify loops use to skip already-analyzed URLs before spending
// budget on a re-fetch.
func (r *runState) markFetched(u string) bool {
	if r.fetchedPages == nil {
		r.fetchedPages = map[string]bool{}
	}

	if r.fetchedPages[u] {
		return true
	}

	r.fetchedPages[u] = true

	return false
}

// wasFetched reports whether a page URL was already detector-analyzed.
func (r *runState) wasFetched(u string) bool {
	return r.fetchedPages[u]
}

// noteBudget records the budget-exhaustion warning once per run.
func (r *runState) noteBudget(out *Report) {
	if r.warnedBudget.CompareAndSwap(false, true) {
		out.Warnings = append(out.Warnings, "request budget exhausted; remaining probes skipped")
	}
}

// get fetches rawURL through the injected client, spending one unit of the
// shared budget. errBudgetExhausted is returned (no request made) once the
// budget is spent.
func (r *runState) get(ctx context.Context, rawURL string) (*FetchedPage, error) {
	if !r.spend() {
		return nil, errBudgetExhausted
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("probe: new request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/rss+xml,application/atom+xml,application/feed+json,application/xml;q=0.9,*/*;q=0.5")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := readBodyLimited(resp.Body, maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("probe: read body: %w", err)
	}

	return &FetchedPage{
		URL:         rawURL,
		FinalURL:    resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		Headers:     resp.Header,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
	}, nil
}

// head issues a HEAD request against rawURL under the shared budget —
// detectors use it for cheap existence probes of well-known paths.
func (r *runState) head(ctx context.Context, rawURL string) (int, http.Header, error) {
	if !r.spend() {
		return 0, nil, errBudgetExhausted
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("probe: new request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, resp.Header, nil
}

// detectRobots fetches robots.txt for the entry origin (budget permitting)
// and fills report.Robots. An unreachable or absent robots.txt keeps the
// default allow-all verdict — matching fetcher.RobotsChecker's fail-open
// policy; the source's own robots_policy field is the authoritative
// override downstream.
func (r *runState) detectRobots(ctx context.Context, page *FetchedPage, out *Report) {
	u, err := url.Parse(page.FinalURL)
	if err != nil || u.Host == "" {
		return
	}

	rpage, err := r.get(ctx, u.Scheme+"://"+u.Host+"/robots.txt")
	if errors.Is(err, errBudgetExhausted) {
		r.noteBudget(out)

		return
	}

	if err != nil || rpage.StatusCode != http.StatusOK {
		return
	}

	verdict := parseRobots(rpage.Body)
	verdict.Fetched = true
	out.Robots = verdict
}

// detectors returns the preview detector set in run order. Platform
// signatures run first — they seed FeedURLs for e.g. Medium, suppressing
// redundant feed-path probing. The sitemap detector runs before feed's
// path guessing so its single conventional probe (/sitemap.xml) is not
// starved by the multi-path feed shortlist under the shared budget.
func (r *runState) detectors() []Detector {
	return []Detector{
		platformDetector{},
		&sitemapDetector{run: r},
		&feedDetector{run: r},
		jsonldDetector{},
		documentsDetector{},
		webSubDetector{},
		&apiSniffDetector{run: r},
		spaDetector{},
		emailHintDetector{},
	}
}

// finalize turns raw detector output into the ranked report: dedupe and
// sort detected methods, derive method-level proposals from page-level
// signals (JSON-LD types, RequiresJS), intersect proposals with the
// resolved category methods, fill pipeline/entity_types/relevance_topics,
// and append the static fallback when nothing was proposed.
func (r *runState) finalize(out *Report, page *FetchedPage) {
	sortMethods(out.Methods)

	if len(out.JSONLDTypes) > 0 {
		out.Channels = append(out.Channels, ChannelProposal{
			Label:          "JSON-LD structured data",
			EntryURL:       out.FinalURL,
			CrawlMethod:    string(sources.CrawlMethodJSONLD),
			Confidence:     weightJSONLD,
			ProviderConfig: map[string]any{"jsonld_types": out.JSONLDTypes},
		})
	}

	if out.RequiresJS {
		out.Channels = append(out.Channels, ChannelProposal{
			Label:       "Rendered page (JS)",
			EntryURL:    out.FinalURL,
			CrawlMethod: string(sources.CrawlMethodRender),
			Confidence:  weightRender,
		})
	}

	var kept []ChannelProposal

	seen := map[string]bool{}

	for _, prop := range out.Channels {
		if !r.methodAllowed(prop.CrawlMethod) {
			continue
		}

		r.fillProposal(&prop, out)

		key := prop.CrawlMethod + "|" + prop.EntryURL + "|" + strings.Join(prop.EntityTypes, ",")
		if seen[key] {
			continue
		}

		seen[key] = true

		kept = append(kept, prop)
	}

	slices.SortStableFunc(kept, func(a, b ChannelProposal) int {
		if a.Confidence != b.Confidence {
			return cmp.Compare(b.Confidence, a.Confidence)
		}

		return strings.Compare(a.Label, b.Label)
	})

	if len(kept) == 0 && page.OK() {
		prop := ChannelProposal{
			Label:       "Static page",
			EntryURL:    out.FinalURL,
			CrawlMethod: string(sources.CrawlMethodStatic),
			Confidence:  weightStatic,
		}
		r.fillProposal(&prop, out)
		kept = append(kept, prop)
	}

	out.Channels = kept

	if r.kind == string(sources.SourceKindChannelProfile) && len(out.DetectedKinds) == 0 {
		out.Warnings = append(out.Warnings,
			"source_kind channel_profile but no platform signature matched the URL host")
	}
}

// bypassMethods are proposal methods exempt from the resolved-methods
// intersection. channel_sync is a routing marker for the §106 video flow;
// email/webhook are inbound pushes (never listed in category methods);
// sitemap is a discovery multiplier no category lists; and social/git are
// platform-signature methods — the same ground-truth argument as
// channel_sync: when the URL host IS reddit/t.me/github, dropping the
// correct proposal because no selected category lists the method would
// silently lose the source's only viable channel (only dataset lists git;
// nothing lists social).
var bypassMethods = map[string]bool{
	MethodChannelSync:                  true,
	string(sources.CrawlMethodEmail):   true,
	string(sources.CrawlMethodWebhook): true,
	string(sources.CrawlMethodSitemap): true,
	string(sources.CrawlMethodSocial):  true,
	string(sources.CrawlMethodGit):     true,
}

// methodAllowed reports whether a proposal for method survives the
// category→method intersection. With no selected categories (empty resolved
// methods) every method is allowed.
func (r *runState) methodAllowed(method string) bool {
	if bypassMethods[method] || len(r.resolved.Methods) == 0 {
		return true
	}

	return slices.Contains(r.resolved.Methods, method)
}

// jsonldEntityFallback lists methods whose channels fetch the probed page
// itself — when no category supplied entity types, schema.org @types found
// on the page are the best available seed.
var jsonldEntityFallback = map[string]bool{
	string(sources.CrawlMethodRSS):    true,
	string(sources.CrawlMethodJSONLD): true,
	string(sources.CrawlMethodRender): true,
	string(sources.CrawlMethodStatic): true,
}

// fillProposal applies the resolved category map to a detector-produced
// proposal: entity types (the nominating categories' vocabulary first,
// JSON-LD-derived fallback for page-level methods), pipeline (the
// nominating categories' merge, else the method default), and relevance
// topics for category-filtered methods.
func (r *runState) fillProposal(prop *ChannelProposal, out *Report) {
	if len(prop.EntityTypes) == 0 {
		prop.EntityTypes = r.entityTypesFor(prop.CrawlMethod)
	}

	if len(prop.EntityTypes) == 0 && jsonldEntityFallback[prop.CrawlMethod] {
		prop.EntityTypes = entityTypesFromJSONLD(out.JSONLDTypes)
	}

	if prop.Pipeline == "" {
		prop.Pipeline = r.pipelineFor(prop.CrawlMethod)
	}

	if len(prop.RelevanceTopics) == 0 && !bypassMethods[prop.CrawlMethod] {
		prop.RelevanceTopics = slices.Clone(r.resolved.Topics)
	}
}

// entityTypesFor narrows a proposal's entity vocabulary to the categories
// that nominated its crawl method — on a [news, job] source the /feed rss
// channel is nominated by 'news' alone, so it carries [news] and feed
// items type as news articles rather than inheriting the source-wide
// union (where a leading 'job' would mistype them). Methods no category
// nominated (bypass/platform methods, unmapped selections) keep the
// merged union.
func (r *runState) entityTypesFor(method string) []string {
	if !bypassMethods[method] {
		if m, ok := r.resolved.ForMethod(method); ok && len(m.EntityTypes) > 0 {
			return m.EntityTypes
		}
	}

	return slices.Clone(r.resolved.EntityTypes)
}

// pipelineFor picks the proposal pipeline. Non-bypass methods take the
// pipeline of the categories that nominated them — extract when ANY
// nominator maps to extract — so a /feed rss channel on a [news, job]
// source curates (news) while its sibling /jobs jsonld channel extracts
// (job): the intel-channels exit scenario. The merged source-level
// pipeline remains the fallback when no category nominated the method
// (empty or unmapped selections); bypass methods use the method default.
func (r *runState) pipelineFor(method string) string {
	if bypassMethods[method] {
		return defaultPipeline(method)
	}
	if m, ok := r.resolved.ForMethod(method); ok && m.Pipeline.IsValid() {
		return string(m.Pipeline)
	}
	if r.resolved.Pipeline.IsValid() {
		return string(r.resolved.Pipeline)
	}

	return defaultPipeline(method)
}

// defaultPipeline is the per-method pipeline used when no category resolved
// (or for bypass methods). Itemized/structured methods curate records
// deterministically; page-level and raw-document methods go through the
// extractor.
func defaultPipeline(method string) string {
	if method == MethodChannelSync || curateDefaultMethods[sources.CrawlMethod(method)] {
		// channel_sync video proposals carry itemized records from
		// video:sync_channels — curate like every other itemized method.
		return string(sources.PipelineCurate)
	}

	// render, static, api, pdf, ocr, email, xhr_sniff, oneshot, manual —
	// plus unknown strings — extract.
	return string(sources.PipelineExtract)
}

// curateDefaultMethods are the crawl methods whose records are itemized or
// structured enough to bypass the LLM extractor by default.
var curateDefaultMethods = map[sources.CrawlMethod]bool{
	sources.CrawlMethodRSS:     true,
	sources.CrawlMethodJSONLD:  true,
	sources.CrawlMethodICal:    true,
	sources.CrawlMethodTabular: true,
	sources.CrawlMethodSitemap: true,
	sources.CrawlMethodGit:     true,
	sources.CrawlMethodSocial:  true,
	sources.CrawlMethodWebhook: true,
	sources.CrawlMethodSearch:  true,
	sources.CrawlMethodWayback: true,
}

// entityTypesFromJSONLD maps schema.org @types to schemas-registry entity
// keys (news, job, article, event, dataset…). Unmapped types contribute no
// entity type — they remain visible in Report.JSONLDTypes.
var jsonldEntityTypes = map[string]string{
	"JobPosting":       "job",
	"NewsArticle":      "news",
	"Article":          "article",
	"BlogPosting":      "article",
	"ScholarlyArticle": "article",
	"Event":            "event",
	"Dataset":          "dataset",
}

func entityTypesFromJSONLD(types []string) []string {
	var out []string

	for _, t := range types {
		if et, ok := jsonldEntityTypes[t]; ok && !slices.Contains(out, et) {
			out = append(out, et)
		}
	}

	return out
}

// sortMethods ranks detected methods by confidence (desc), then name.
func sortMethods(methods []DetectedMethod) {
	slices.SortStableFunc(methods, func(a, b DetectedMethod) int {
		if a.Confidence != b.Confidence {
			return cmp.Compare(b.Confidence, a.Confidence)
		}

		return strings.Compare(a.Method, b.Method)
	})
}

// normalizeURL validates the candidate URL: scheme optional (https assumed),
// http(s) required. file:, ftp:, javascript: and other schemes are rejected
// here so they never reach the client.
func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("probe: %w: url", errpkg.ErrRequired)
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("probe: %w: url %q", errpkg.ErrInvalid, raw)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("probe: %w: url scheme %q must be http(s)", errpkg.ErrInvalid, u.Scheme)
	}

	return u.String(), nil
}

// readBodyLimited reads up to limit bytes, returning errBodyTooLarge when
// the source exceeds the cap (the fetcher.MaxBodyReader contract — a
// truncated body must not drive detection).
func readBodyLimited(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}

	if int64(len(body)) > limit {
		return nil, errBodyTooLarge
	}

	return body, nil
}

// robotsParser accumulates the verdict for one robots.txt document while
// tracking which User-agent block each rule belongs to — mirroring
// fetcher's robotsParser semantics: a dedicated japaconnect* block wins
// over the wildcard block, wildcard rules stop accumulating once a
// JapaConnect rule is seen, and rules outside any user-agent block are
// ignored. Sitemap: declarations are collected regardless of block.
type robotsParser struct {
	verdict    RobotVerdict
	disallowed []string
	currentUA  string
	inOurBlock bool
	inWildcard bool
	// seenOurRule flips on the first rule applied inside the dedicated
	// JapaConnect block — the block is authoritative, so wildcard rules
	// collected earlier are discarded and wildcard rules stop collecting.
	seenOurRule bool
}

// parseRobots parses a robots.txt body into a RobotVerdict.
func parseRobots(body []byte) RobotVerdict {
	p := &robotsParser{verdict: RobotVerdict{Allowed: true}}

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		p.applyLine(line, strings.ToLower(line))
	}

	p.verdict.DisallowedPaths = p.disallowed

	return p.verdict
}

// applyLine dispatches one robots.txt line.
func (p *robotsParser) applyLine(line, lower string) {
	switch {
	case strings.HasPrefix(lower, "user-agent:"):
		p.applyUserAgent(lower)
	case strings.HasPrefix(lower, "sitemap:"):
		if s := strings.TrimSpace(line[len("Sitemap:"):]); s != "" {
			p.verdict.SitemapURLs = append(p.verdict.SitemapURLs, s)
		}
	default:
		p.applyRule(line, lower)
	}
}

// applyUserAgent starts a new User-agent block.
func (p *robotsParser) applyUserAgent(lower string) {
	p.currentUA = strings.TrimSpace(lower[len("user-agent:"):])
	p.inOurBlock = p.currentUA == "japaconnect" || p.currentUA == "japaconnect-intelligence"
	p.inWildcard = p.currentUA == "*"
}

// applyRule applies a Disallow/Crawl-delay line when the current block
// applies to our crawler.
func (p *robotsParser) applyRule(line, lower string) {
	collect := p.inOurBlock || (p.inWildcard && !p.seenOurRule)
	if !collect && p.currentUA == "" {
		return
	}

	if p.inOurBlock && !p.seenOurRule {
		// First rule of our dedicated block: it supersedes every wildcard
		// rule collected so far (robots.txt group semantics).
		p.seenOurRule = true
		p.disallowed = nil
	}

	switch {
	case strings.HasPrefix(lower, "disallow:"):
		p.applyDisallow(line, collect)
	case strings.HasPrefix(lower, "crawl-delay:"):
		if collect {
			if n, err := strconv.Atoi(strings.TrimSpace(line[len("Crawl-delay:"):])); err == nil {
				p.verdict.CrawlDelaySecs = n
			}
		}
	}
}

// applyDisallow records a Disallow rule. An empty value means "allow all"
// and clears previously collected paths — but only when the rule belongs to
// a block that applies to us; a foreign user-agent's bare Disallow must not
// wipe our collected rules (matching fetcher's parser).
func (p *robotsParser) applyDisallow(line string, collect bool) {
	path := strings.TrimSpace(line[len("Disallow:"):])
	if path == "" {
		if collect {
			p.verdict.Allowed = true
			p.disallowed = nil
		}

		return
	}

	if collect {
		p.disallowed = append(p.disallowed, path)
		if path == "/" {
			p.verdict.Allowed = false
		}
	}
}
