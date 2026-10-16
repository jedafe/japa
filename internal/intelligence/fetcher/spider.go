package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// spider.go — Phase-4 domain-scoped crawl (intel-channels spec §8). The
// spider adapter fetches the channel's entry page like a static fetch,
// then rewrites it as a manifest of its own outbound anchor graph: every
// normalized same-host link becomes one url<TAB>anchor-stamp line that
// detect_changes diffs and fans out exactly like a sitemap manifest —
// each changed child enqueues a derived fetch (static page at the depth
// ceiling, nested spider manifest below it). External links ride the
// X-Intel-Outbound-Links header for the intel:harvest_outbound discovery
// task; they are never fetched.
//
// Scope rules (all enforced at extraction; expansion re-checks the
// same-host invariant on the persisted doc — manifests are content):
//   - same host only: exact case-insensitive hostname match against the
//     post-redirect page host — subdomains are a separate channel's job
//   - depth: provider_config.max_depth (1–2, default 1 = entry + children)
//   - breadth: provider_config.max_urls caps the manifest (default 200,
//     hard cap 1000); the per-crawl fan-out cap is a worker setting
//     (intel.spider.max_children_per_crawl)
//   - provider_config.max_outbound caps the discovery header (default 50,
//     hard cap 200)
//   - provider_config.spider_depth is worker-injected on derived fetches —
//     it stamps X-Intel-Spider-Depth so expansion knows where the page sat
//
// Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md §8.

const (
	// ManifestKindSpider marks a manifest built from a spider crawl —
	// fanned-out children are content pages (static) or nested spider
	// manifests (below max_depth), never sitemaps.
	ManifestKindSpider = "spider"

	// SpiderDepthHeader records the depth of the page that produced the
	// manifest (entry fetch = 0). Expansion enqueues children at
	// depth+1 — as nested spider manifests while under max_depth, static
	// content fetches at the ceiling.
	SpiderDepthHeader = "X-Intel-Spider-Depth"

	// OutboundLinksHeader carries a JSON []string of normalized external
	// URLs — one per distinct foreign host, first-seen order — harvested
	// by intel:harvest_outbound into proposed intel_sources rows.
	OutboundLinksHeader = "X-Intel-Outbound-Links"

	// SpiderScopeHeader records the normalized post-redirect base URL the
	// link graph was scoped to. The raw doc's url field stays the entry
	// URL (derived-fetch bookkeeping keys on it), so expansion must scope
	// against this header — an example.com → www.example.com redirect
	// otherwise drops every manifest child at the scope re-check.
	SpiderScopeHeader = "X-Intel-Spider-Scope"

	// spiderMaxDepthCeiling bounds provider_config.max_depth — a deeper
	// same-host walk is a crawler, not a channel.
	spiderMaxDepthCeiling = 2
	// spiderDefaultMaxURLs / spiderMaxURLsCeiling bound the manifest size
	// (provider_config.max_urls).
	spiderDefaultMaxURLs = 200
	spiderMaxURLsCeiling = 1000
	// spiderDefaultMaxOutbound / spiderMaxOutboundCeiling bound the
	// X-Intel-Outbound-Links discovery list.
	spiderDefaultMaxOutbound = 50
	spiderMaxOutboundCeiling = 200
)

// spiderAssetExts are path extensions that can never be crawlable content —
// media, fonts, archives, and static assets are dropped before scope checks.
var spiderAssetExts = map[string]bool{
	".css": true, ".js": true, ".mjs": true, ".map": true,
	".ico": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".avif": true, ".svg": true, ".bmp": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".mp3": true, ".mp4": true, ".webm": true, ".wav": true, ".mov": true,
	".zip": true, ".gz": true, ".tar": true, ".rar": true, ".7z": true,
	".xml": true, ".json": true, ".webmanifest": true,
}

// spiderTrackingParams are query params stripped during normalization —
// they never change the page a URL points at and only pollute the manifest.
var spiderTrackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "dclid": true, "msclkid": true,
	"mc_cid": true, "mc_eid": true, "_hsenc": true, "_hsmi": true,
}

// fetchSpider retrieves the entry page via the static path and rewrites it
// as a url<TAB>anchor-stamp manifest (same document shape the sitemap and
// wayback adapters produce — ParseManifest/DiffManifest apply unchanged).
// The manifest body plus the outbound list feed the content hash, so a page
// whose only change is a new outbound link still marks the crawl changed
// and re-runs source discovery.
func (c *Composite) fetchSpider(ctx context.Context, eff sources.Source) (FetchResult, error) {
	res, err := c.static.Fetch(ctx, eff)
	if err != nil {
		return FetchResult{}, err
	}

	if !isHTMLDoc(res.ContentType) {
		return FetchResult{}, fmt.Errorf("fetcher: spider: entry URL served %q, expected HTML", res.ContentType)
	}

	base := res.FinalURL
	if base == "" {
		base = eff.SourceUrl
	}

	baseURL, _ := url.Parse(base)
	scope := ""

	if baseURL != nil {
		scope = normalizeLinkURL(baseURL)
	}

	var cfg map[string]any
	_ = json.Unmarshal(eff.ProviderConfig, &cfg)

	maxURLs := clampSpiderInt(cfgInt(cfg, "max_urls", spiderDefaultMaxURLs), 1, spiderMaxURLsCeiling)
	maxOutbound := clampSpiderInt(cfgInt(cfg, "max_outbound", spiderDefaultMaxOutbound), 0, spiderMaxOutboundCeiling)
	depth := cfgInt(cfg, "spider_depth", 0)

	internal, external := extractPageLinks(res.Body, base, maxURLs, maxOutbound)

	var b strings.Builder
	for _, l := range internal {
		fmt.Fprintf(&b, "%s\t%s\n", l.url, l.stamp)
	}

	manifest := []byte(b.String())

	outboundJSON, _ := json.Marshal(external)

	// The hash input folds the manifest and the outbound list together:
	// a link-graph-only change (new external domain, no new internal URLs)
	// must still trigger detect_changes → intel:harvest_outbound.
	var hashIn []byte
	hashIn = append(hashIn, manifest...)
	hashIn = append(hashIn, '\n')
	hashIn = append(hashIn, outboundJSON...)
	hash := NormalizeForHash(hashIn, ManifestContentType)

	if res.Headers == nil {
		res.Headers = map[string]string{}
	}

	res.Headers[ManifestKindHeader] = ManifestKindSpider
	res.Headers[SpiderDepthHeader] = strconv.Itoa(depth)

	if scope != "" {
		res.Headers[SpiderScopeHeader] = scope
	}

	if len(external) > 0 {
		res.Headers[OutboundLinksHeader] = string(outboundJSON)
	}

	return FetchResult{
		HTTPStatus:  res.HTTPStatus,
		Headers:     res.Headers,
		ContentType: ManifestContentType,
		Body:        manifest,
		ContentHash: hash,
		FinalURL:    res.FinalURL,
	}, nil
}

// SpiderManifestDepth reads the depth stamp a spider fetch recorded on the
// persisted manifest doc — 0 (entry) when the header is absent or invalid.
func SpiderManifestDepth(headers map[string]string) int {
	n, err := strconv.Atoi(strings.TrimSpace(headers[SpiderDepthHeader]))
	if err != nil || n < 0 {
		return 0
	}

	return n
}

// SpiderMaxDepth resolves provider_config.max_depth for a spider channel —
// clamped to [1, spiderMaxDepthCeiling]; the worker clamps the same way so
// the expansion ceiling can never exceed what the adapter stamps.
func SpiderMaxDepth(providerConfig []byte) int {
	var cfg map[string]any
	_ = json.Unmarshal(providerConfig, &cfg)

	return clampSpiderInt(cfgInt(cfg, "max_depth", 1), 1, spiderMaxDepthCeiling)
}

// spiderLink is one normalized anchor target plus its change stamp — the
// manifest's stamp column. The stamp is a truncated hash of the anchor
// text so a label rename (new scholarship title on a stable URL) still
// marks the child changed.
type spiderLink struct {
	url   string
	stamp string
}

// spiderScopePort returns the port a URL's authority resolves to — the
// explicit port or the scheme default. Same-host crawling is
// origin-scoped: host:8080 is a different service than host:443, and a
// page must not nominate its host's sibling services as manifest children.
func spiderScopePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}

	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	}

	return ""
}

// SpiderSameScope reports whether childURL sits inside baseURL's crawl
// scope — same hostname AND same effective port (http/80 vs https/443 vs
// explicit ports are distinct origins). Used at extraction AND at worker
// expansion (defense in depth on persisted manifests).
func SpiderSameScope(childURL, baseURL string) bool {
	cu, err := url.Parse(childURL)
	if err != nil {
		return false
	}

	bu, err := url.Parse(baseURL)
	if err != nil {
		return false
	}

	return strings.EqualFold(cu.Hostname(), bu.Hostname()) &&
		spiderScopePort(cu) == spiderScopePort(bu)
}

// extractPageLinks walks the document's <a href> anchors and splits them
// into same-host (internal) and foreign-host (external) link sets.
// Internal links dedupe by normalized URL; external links dedupe by host
// (first-seen representative URL). Both caps are applied AFTER filtering —
// a cap only ever binds real candidates.
func extractPageLinks(body []byte, base string, maxInternal, maxExternal int) (internal []spiderLink, external []string) {
	baseURL, err := url.Parse(base)
	if err != nil || baseURL.Host == "" {
		return nil, nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, nil
	}

	baseHost := strings.ToLower(baseURL.Hostname())
	self := normalizeLinkURL(baseURL)

	seenInternal := make(map[string]bool)
	seenExternalHost := make(map[string]bool)

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")

		link := normalizeAnchor(baseURL, href, self)
		if link == "" {
			return
		}

		u, _ := url.Parse(link)
		if strings.EqualFold(u.Hostname(), baseHost) {
			if len(internal) < maxInternal && !seenInternal[link] {
				seenInternal[link] = true
				stamp := sha256.Sum256([]byte(strings.Join(strings.Fields(s.Text()), " ")))
				internal = append(internal, spiderLink{url: link, stamp: hex.EncodeToString(stamp[:])[:12]})
			}

			return
		}

		if len(external) < maxExternal {
			if h := strings.ToLower(u.Hostname()); !seenExternalHost[h] {
				seenExternalHost[h] = true

				external = append(external, link)
			}
		}
	})

	// Stable order: the manifest hash must not churn on map/iteration
	// nondeterminism or DOM-adjacent reorderings.
	slices.SortFunc(internal, func(a, b spiderLink) int {
		return strings.Compare(a.url, b.url)
	})

	return internal, external
}

// ExtractOutboundLinks returns the foreign-host links of an HTML document —
// one normalized URL per distinct external host, first-seen order, capped
// at max. It is the harvest task's body-parse path for raw docs that carry
// no X-Intel-Outbound-Links header (static/render/oneshot pages).
func ExtractOutboundLinks(body []byte, base string, max int) []string {
	_, external := extractPageLinks(body, base, 0, max)

	return external
}

// normalizeAnchor resolves one raw href against the page's final URL and
// normalizes it — "" for anything that can never be a crawlable link
// (non-http schemes, fragments-only, assets, private literals, self-links).
func normalizeAnchor(base *url.URL, href, self string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}

	u, err := base.Parse(href)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ""
	}

	// A same-host link rides the entry fetch's SSRF vetting — the base
	// host was already dialed through the safe transport. Foreign hosts
	// need the literal check here so a link graph cannot nominate a
	// private address as a manifest child or discovery candidate.
	if !strings.EqualFold(u.Hostname(), base.Hostname()) && sources.IsPrivateIPLiteral(u.Hostname()) {
		return ""
	}

	// Same hostname but a different effective port is a sibling service,
	// not the site — neither a manifest child nor a discovery candidate
	// (the harvest gate would reject it as same-host anyway).
	if strings.EqualFold(u.Hostname(), base.Hostname()) && spiderScopePort(u) != spiderScopePort(base) {
		return ""
	}

	norm := normalizeLinkURL(u)
	if norm == "" || norm == self {
		return ""
	}

	if spiderAssetExts[strings.ToLower(path.Ext(u.EscapedPath()))] {
		return ""
	}

	return norm
}

// normalizeLinkURL canonicalizes a crawlable URL for dedupe and manifest
// stability: lowercase scheme/host, no fragment, no default port, tracking
// params stripped, empty path → "/".
func normalizeLinkURL(u *url.URL) string {
	v := *u
	v.Scheme = strings.ToLower(v.Scheme)
	v.Host = strings.ToLower(v.Host)
	v.Fragment = ""
	v.RawFragment = ""
	v.User = nil

	host := v.Hostname()
	port := v.Port()

	if (v.Scheme == "http" && port == "80") || (v.Scheme == "https" && port == "443") {
		v.Host = host
	}

	if v.Path == "" {
		v.Path = "/"
	}

	q := v.Query()
	for key := range q {
		lk := strings.ToLower(key)
		if spiderTrackingParams[lk] || strings.HasPrefix(lk, "utm_") {
			q.Del(key)
		}
	}

	v.RawQuery = q.Encode()

	return v.String()
}

// isHTMLDoc reports whether a fetched body's content type is HTML — the
// only document shape the spider can extract links from.
func isHTMLDoc(contentType string) bool {
	mt := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))

	return mt == "text/html" || mt == "application/xhtml+xml"
}

// cfgInt reads a loosely-typed numeric provider_config key.
func cfgInt(cfg map[string]any, key string, fallback int) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}

	return fallback
}

func clampSpiderInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}

	if v > hi {
		return hi
	}

	return v
}
