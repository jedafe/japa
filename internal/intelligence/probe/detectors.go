package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// feedDetector discovers RSS/Atom/JSON feeds: the entry document itself when
// it is a feed, <link rel="alternate"> tags in the page head, and — when no
// link was found and budget remains — HEAD probes of common feed paths.
// noPathProbe disables the common-path HEAD probing (verify sub-pages only
// scan the document — the category/discovery paths are GET-probed directly).
type feedDetector struct {
	run         *runState
	noPathProbe bool
}

// feedLinkTypes maps <link rel="alternate"> type attributes to feed labels.
// application/json on an alternate link is the JSON Feed convention.
var feedLinkTypes = map[string]string{
	"application/rss+xml":   "RSS feed",
	"application/atom+xml":  "Atom feed",
	"application/feed+json": "JSON feed",
	"application/json":      "JSON feed",
}

// commonFeedPaths is the HEAD-probe shortlist, in hit-probability order.
// Probing stops at the first hit to preserve budget for other detectors.
var commonFeedPaths = []string{"/feed", "/feed.xml", "/rss.xml", "/atom.xml", "/rss"}

// Detect implements Detector.
func (d *feedDetector) Detect(ctx context.Context, page *FetchedPage, out *Report) {
	// The submitted URL may itself be a feed endpoint.
	if isFeedContent(page.ContentType) || looksLikeFeedBody(page.Body) {
		d.addFeed(out, page.FinalURL, feedLabel(page.ContentType), "entry document is a feed")

		return
	}

	doc := page.HTML()
	if doc == nil {
		return
	}

	walk(doc, func(n *html.Node) {
		if n.Data != "link" {
			return
		}

		if !strings.Contains(strings.ToLower(attr(n, "rel")), "alternate") {
			return
		}

		label, ok := feedLinkTypes[strings.ToLower(attr(n, "type"))]
		if !ok {
			return
		}

		if href := absURL(page, attr(n, "href")); href != "" {
			d.addFeed(out, href, label,
				fmt.Sprintf("link[rel=alternate][type=%s]", strings.ToLower(attr(n, "type"))))
		}
	})

	// Guess common feed paths only when the page declared none — discovered
	// links are authoritative and probing would just burn budget.
	if len(out.FeedURLs) == 0 && !d.noPathProbe {
		d.probeCommonPaths(ctx, page, out)
	}
}

// probeCommonPaths HEAD-checks the conventional feed locations until one
// responds with a non-HTML success or the budget runs out.
func (d *feedDetector) probeCommonPaths(ctx context.Context, page *FetchedPage, out *Report) {
	u, err := url.Parse(page.FinalURL)
	if err != nil {
		return
	}

	for _, p := range commonFeedPaths {
		candidate := u.Scheme + "://" + u.Host + p

		status, header, err := d.run.head(ctx, candidate)
		if errors.Is(err, errBudgetExhausted) {
			d.run.noteBudget(out)

			return
		}

		if err != nil {
			continue
		}

		ct := header.Get("Content-Type")
		// SPA servers answer 200 text/html for every path — reject HTML so a
		// catch-all route cannot masquerade as a feed.
		if status == http.StatusOK && plausiblyFeed(ct, p) {
			d.addFeed(out, candidate, feedLabel(ct), "HEAD probe "+p)

			return
		}
	}
}

// addFeed records a discovered feed endpoint, its method evidence, and its
// channel proposal.
func (d *feedDetector) addFeed(out *Report, feedURL, label, evidence string) {
	if slices.Contains(out.FeedURLs, feedURL) {
		return
	}

	out.FeedURLs = append(out.FeedURLs, feedURL)

	out.addMethod(string(sources.CrawlMethodRSS), weightFeed, evidence)
	out.Channels = append(out.Channels, ChannelProposal{
		Label:       label,
		EntryURL:    feedURL,
		CrawlMethod: string(sources.CrawlMethodRSS),
		Confidence:  weightFeed,
	})
}

// jsonldDetector extracts schema.org @type values from application/ld+json
// script blocks — the strongest signal for typed-record pages.
type jsonldDetector struct{}

// Detect implements Detector.
func (jsonldDetector) Detect(_ context.Context, page *FetchedPage, out *Report) {
	doc := page.HTML()
	if doc == nil {
		return
	}

	walk(doc, func(n *html.Node) {
		if n.Data != "script" || !strings.Contains(strings.ToLower(attr(n, "type")), "ld+json") {
			return
		}

		var b strings.Builder

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				b.WriteString(c.Data)
			}
		}

		for _, typ := range extractJSONLDTypes([]byte(b.String())) {
			if !slices.Contains(out.JSONLDTypes, typ) {
				out.JSONLDTypes = append(out.JSONLDTypes, typ)
			}
		}
	})

	if len(out.JSONLDTypes) > 0 {
		out.addMethod(string(sources.CrawlMethodJSONLD), weightJSONLD,
			"ld+json @type="+strings.Join(out.JSONLDTypes, ","))
	}
}

// extractJSONLDTypes walks a decoded ld+json document collecting every
// @type value — handles top-level objects, arrays, @graph containers, and
// nested entities (spec §2: "schema.org @types found").
func extractJSONLDTypes(raw []byte) []string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}

	var types []string

	var rec func(node any)
	rec = func(node any) {
		switch t := node.(type) {
		case map[string]any:
			switch tv := t["@type"].(type) {
			case string:
				types = append(types, tv)
			case []any:
				for _, e := range tv {
					if s, ok := e.(string); ok {
						types = append(types, s)
					}
				}
			}

			for _, val := range t {
				rec(val)
			}
		case []any:
			for _, e := range t {
				rec(e)
			}
		}
	}
	rec(v)

	return types
}

// sitemapDetector collects sitemap endpoints declared in robots.txt and —
// when none are declared and budget remains — probes /sitemap.xml. An entry
// document that is itself a sitemap (urlset/sitemapindex) is also recorded.
type sitemapDetector struct {
	run *runState
}

// Detect implements Detector.
func (d *sitemapDetector) Detect(ctx context.Context, page *FetchedPage, out *Report) {
	if looksLikeSitemapBody(page.Body) {
		d.addSitemap(out, page.FinalURL, "entry document is a sitemap")
	}

	for _, sm := range out.Robots.SitemapURLs {
		d.addSitemap(out, sm, "robots.txt Sitemap: declaration")
	}

	if len(out.SitemapURLs) > 0 {
		return
	}

	u, err := url.Parse(page.FinalURL)
	if err != nil {
		return
	}

	candidate := u.Scheme + "://" + u.Host + "/sitemap.xml"

	status, header, err := d.run.head(ctx, candidate)
	if errors.Is(err, errBudgetExhausted) {
		d.run.noteBudget(out)

		return
	}

	if err != nil || status != http.StatusOK {
		return
	}

	ct := header.Get("Content-Type")
	if isHTMLContent(ct) {
		// SPA catch-all — not a real sitemap.
		return
	}

	if ct == "" || strings.Contains(ct, "xml") || strings.Contains(ct, "octet-stream") || strings.Contains(ct, "text/plain") {
		d.addSitemap(out, candidate, "HEAD /sitemap.xml")
	}
}

// addSitemap records a discovered sitemap endpoint, method evidence, and a
// sitemap channel proposal (a discovery multiplier — always allowed).
func (d *sitemapDetector) addSitemap(out *Report, sitemapURL, evidence string) {
	if slices.Contains(out.SitemapURLs, sitemapURL) {
		return
	}

	out.SitemapURLs = append(out.SitemapURLs, sitemapURL)

	out.addMethod(string(sources.CrawlMethodSitemap), weightSitemap, evidence)
	out.Channels = append(out.Channels, ChannelProposal{
		Label:       "Sitemap manifest",
		EntryURL:    sitemapURL,
		CrawlMethod: string(sources.CrawlMethodSitemap),
		Confidence:  weightSitemap,
	})
}

// documentsDetector finds document endpoints linked from the page —
// PDF/DOCX (pdf method), CSV/TSV/XLSX (tabular), ICS (ical) — and recognizes
// when the entry URL itself is such a document.
type documentsDetector struct{}

// docExtensions maps link file extensions to the crawl method that handles
// them.
var docExtensions = map[string]sources.CrawlMethod{
	".pdf":  sources.CrawlMethodPDF,
	".doc":  sources.CrawlMethodPDF,
	".docx": sources.CrawlMethodPDF,
	".csv":  sources.CrawlMethodTabular,
	".tsv":  sources.CrawlMethodTabular,
	".xlsx": sources.CrawlMethodTabular,
	".ics":  sources.CrawlMethodICal,
	".ical": sources.CrawlMethodICal,
}

// docContentTypes recognizes document content types on the entry response.
var docContentTypes = map[string]sources.CrawlMethod{
	"application/pdf":           sources.CrawlMethodPDF,
	"text/csv":                  sources.CrawlMethodTabular,
	"text/tab-separated-values": sources.CrawlMethodTabular,
	"text/calendar":             sources.CrawlMethodICal,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       sources.CrawlMethodTabular,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": sources.CrawlMethodPDF,
}

// Detect implements Detector.
func (documentsDetector) Detect(_ context.Context, page *FetchedPage, out *Report) {
	if method, ok := docContentTypes[mediaType(page.ContentType)]; ok {
		addDocProposal(out, page.FinalURL, method, weightAPIEntry,
			"entry document is "+mediaType(page.ContentType))

		return
	}

	doc := page.HTML()
	if doc == nil {
		return
	}

	// First discovered URL per method becomes the proposal entry point.
	found := map[sources.CrawlMethod]string{}

	walk(doc, func(n *html.Node) {
		if n.Data != "a" {
			return
		}

		href := absURL(page, attr(n, "href"))
		if href == "" {
			return
		}

		ext := strings.ToLower(path.Ext(hrefPath(href)))
		if method, ok := docExtensions[ext]; ok {
			if _, seen := found[method]; !seen {
				found[method] = href
			}
		}
	})

	for method, docURL := range found {
		addDocProposal(out, docURL, method, weightDocs, "linked document "+docURL)
	}
}

// addDocProposal records a document-method detection plus its proposal.
// ical is weighted higher — an events feed is a strong structured source.
func addDocProposal(out *Report, entryURL string, method sources.CrawlMethod, confidence float64, evidence string) {
	if method == sources.CrawlMethodICal {
		confidence = weightICal
	}

	out.addMethod(string(method), confidence, evidence)
	out.Channels = append(out.Channels, ChannelProposal{
		Label:       docLabel(method),
		EntryURL:    entryURL,
		CrawlMethod: string(method),
		Confidence:  confidence,
	})
}

// docLabels are proposal labels for document crawl methods.
var docLabels = map[sources.CrawlMethod]string{
	sources.CrawlMethodPDF:     "PDF documents",
	sources.CrawlMethodTabular: "Tabular data",
	sources.CrawlMethodICal:    "iCal events",
}

func docLabel(method sources.CrawlMethod) string {
	if label, ok := docLabels[method]; ok {
		return label
	}

	return "Documents"
}

// webSubDetector recognizes WebSub (PubSubHubbub) hub advertisements in
// Link response headers or <link rel="hub"> tags — a push channel candidate.
type webSubDetector struct{}

// linkHeaderRe matches one Link-header value pair: <url>; rel="hub".
var linkHeaderRe = regexp.MustCompile(`<([^>]+)>\s*;\s*rel\s*=\s*"?([^";]+)"?`)

// Detect implements Detector.
func (webSubDetector) Detect(_ context.Context, page *FetchedPage, out *Report) {
	hub, topic := parseWebSubLinks(page.Headers.Values("Link"))

	if hub == "" {
		hub = hubFromHTML(page)
	}

	if hub == "" {
		return
	}

	if topic == "" {
		topic = page.FinalURL
	}

	out.HasWebSub = true
	out.addMethod(string(sources.CrawlMethodWebhook), weightWebSub, "WebSub hub "+hub)
	out.Channels = append(out.Channels, ChannelProposal{
		Label:       "WebSub push",
		EntryURL:    topic,
		CrawlMethod: string(sources.CrawlMethodWebhook),
		Confidence:  weightWebSub,
		ProviderConfig: map[string]any{
			"hub":   hub,
			"topic": topic,
		},
	})
}

// hubFromHTML finds a <link rel="hub" href="…"> tag — the HTML variant of
// the WebSub hub advertisement.
func hubFromHTML(page *FetchedPage) string {
	doc := page.HTML()
	if doc == nil {
		return ""
	}

	var hub string

	walk(doc, func(n *html.Node) {
		if hub != "" || n.Data != "link" {
			return
		}

		if strings.EqualFold(strings.TrimSpace(attr(n, "rel")), "hub") {
			hub = absURL(page, attr(n, "href"))
		}
	})

	return hub
}

// parseWebSubLinks scans Link header values for rel="hub" / rel="self"
// targets (the WebSub discovery contract).
func parseWebSubLinks(headers []string) (hub, self string) {
	for _, h := range headers {
		for _, m := range linkHeaderRe.FindAllStringSubmatch(h, -1) {
			switch strings.ToLower(strings.TrimSpace(m[2])) {
			case "hub":
				if hub == "" {
					hub = m[1]
				}
			case "self":
				if self == "" {
					self = m[1]
				}
			}
		}
	}

	return hub, self
}

// apiSniffDetector recognizes JSON API surfaces: the entry document itself
// when it is JSON, and well-known OpenAPI endpoints probed under budget.
type apiSniffDetector struct {
	run *runState
}

// apiProbePaths is the OpenAPI/spec shortlist; probing stops at first hit.
var apiProbePaths = []string{"/openapi.json", "/api/openapi.json", "/swagger.json"}

// Detect implements Detector.
func (d *apiSniffDetector) Detect(ctx context.Context, page *FetchedPage, out *Report) {
	if mediaType(page.ContentType) == "application/json" {
		out.addMethod(string(sources.CrawlMethodAPI), weightAPIEntry, "entry document is JSON")
		out.Channels = append(out.Channels, ChannelProposal{
			Label:       "JSON API endpoint",
			EntryURL:    page.FinalURL,
			CrawlMethod: string(sources.CrawlMethodAPI),
			Confidence:  weightAPIEntry,
		})

		return
	}

	u, err := url.Parse(page.FinalURL)
	if err != nil {
		return
	}

	for _, p := range apiProbePaths {
		candidate := u.Scheme + "://" + u.Host + p

		status, header, err := d.run.head(ctx, candidate)
		if errors.Is(err, errBudgetExhausted) {
			d.run.noteBudget(out)

			return
		}

		if err != nil {
			continue
		}

		ct := mediaType(header.Get("Content-Type"))
		if status == http.StatusOK && apiSpecContent(ct) {
			out.addMethod(string(sources.CrawlMethodAPI), weightAPIProbe, "HEAD "+p)
			out.Channels = append(out.Channels, ChannelProposal{
				Label:       "API endpoint",
				EntryURL:    candidate,
				CrawlMethod: string(sources.CrawlMethodAPI),
				Confidence:  weightAPIProbe,
				ProviderConfig: map[string]any{
					"endpoint": candidate,
					"spec":     true,
				},
			})

			return
		}
	}
}

// apiSpecContent reports whether a probed-path content type plausibly names
// an API spec (JSON/YAML spec documents or unlabelled downloads).
func apiSpecContent(mediaType string) bool {
	switch mediaType {
	case "application/json", "application/octet-stream",
		"text/yaml", "application/yaml", "text/x-yaml":
		return true
	}

	return false
}

// spaDetector recognizes client-rendered shells: a negligible text body plus
// mount-point/framework markers means the real content needs a render pass.
type spaDetector struct{}

// spaMountIDs are the conventional JS framework mount-point element ids.
var spaMountIDs = map[string]bool{
	"root": true, "app": true, "__next": true, "__nuxt": true,
	"q-app": true, "ember-view": true,
}

// spaMarkerAttrs are presence-only framework signatures.
var spaMarkerAttrs = []string{"ng-app", "data-reactroot", "data-v-app", "ng-version"}

const (
	// spaTextFloor: below this many visible text bytes the page is a shell
	// regardless of markers.
	spaTextFloor = 80
	// spaTextMarkerCeiling: a mount marker plus text below this still counts
	// as a shell (e.g. a loader message).
	spaTextMarkerCeiling = 400
)

// Detect implements Detector.
func (spaDetector) Detect(_ context.Context, page *FetchedPage, out *Report) {
	doc := page.HTML()
	if doc == nil {
		return
	}

	marker := false

	walk(doc, func(n *html.Node) {
		if marker {
			return
		}

		if n.Data == "script" && attr(n, "id") == "__NEXT_DATA__" {
			marker = true

			return
		}

		if spaMountIDs[attr(n, "id")] {
			marker = true

			return
		}

		for _, a := range spaMarkerAttrs {
			if attr(n, a) != "" {
				marker = true

				return
			}
		}
	})

	textLen := visibleTextLen(doc)
	if textLen < spaTextFloor || (marker && textLen < spaTextMarkerCeiling) {
		out.RequiresJS = true
		out.addMethod(string(sources.CrawlMethodRender), weightRender,
			"SPA shell markers + negligible text")
	}
}

// emailHintDetector finds mailto: links and newsletter signup forms — the
// signal for an inbound email channel an admin can wire to a mailbox.
type emailHintDetector struct{}

// newsletterMarkers are form-action substrings indicating a subscription
// endpoint rather than a contact form.
var newsletterMarkers = []string{
	"subscribe", "newsletter", "mailchimp", "list-manage", "substack",
	"convertkit", "buttondown", "beehiiv",
}

// Detect implements Detector.
func (emailHintDetector) Detect(_ context.Context, page *FetchedPage, out *Report) {
	doc := page.HTML()
	if doc == nil {
		return
	}

	mailto, formHint := scanEmailHints(doc)
	if mailto == "" && formHint == "" {
		return
	}

	entry := page.FinalURL
	hint := map[string]any{}

	var evidence string

	if mailto != "" {
		// Inbound channels accept non-URL entry points — the mailto address
		// is the most useful prefill for the admin-wired mailbox.
		entry = "mailto:" + mailto
		hint["mailto"] = mailto
		evidence = "mailto:" + mailto
	}

	if formHint != "" {
		hint["newsletter_form"] = formHint

		if evidence != "" {
			evidence += "; "
		}

		evidence += "newsletter form " + formHint
	}

	out.addMethod(string(sources.CrawlMethodEmail), weightEmailHint, evidence)
	out.Channels = append(out.Channels, ChannelProposal{
		Label:          "Email / newsletter inbox",
		EntryURL:       entry,
		CrawlMethod:    string(sources.CrawlMethodEmail),
		Confidence:     weightEmailHint,
		ProviderConfig: map[string]any{"hint": hint},
	})
}

// scanEmailHints finds the first mailto: link address and the first
// newsletter-ish form action in the document.
func scanEmailHints(doc *html.Node) (mailto, formHint string) {
	walk(doc, func(n *html.Node) {
		switch n.Data {
		case "a":
			if mailto == "" {
				if addr, ok := mailtoAddress(attr(n, "href")); ok {
					mailto = addr
				}
			}
		case "form":
			if formHint == "" && isNewsletterForm(attr(n, "action")) {
				formHint = attr(n, "action")
			}
		}
	})

	return mailto, formHint
}

// isNewsletterForm reports whether a form action looks like a subscription
// endpoint rather than a contact form.
func isNewsletterForm(action string) bool {
	action = strings.ToLower(action)

	for _, m := range newsletterMarkers {
		if strings.Contains(action, m) {
			return true
		}
	}

	return false
}

// mailtoAddress extracts the address from a mailto: href, stripping query
// params. Returns ok=false for non-mailto hrefs.
func mailtoAddress(href string) (string, bool) {
	href = strings.TrimSpace(href)
	if !strings.HasPrefix(strings.ToLower(href), "mailto:") {
		return "", false
	}

	addr := href[len("mailto:"):]
	if i := strings.IndexByte(addr, '?'); i >= 0 {
		addr = addr[:i]
	}

	addr = strings.TrimSpace(addr)

	return addr, addr != ""
}

// --- shared helpers -------------------------------------------------------

// mediaType strips parameters from a Content-Type header value.
func mediaType(contentType string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
}

func isHTMLContent(contentType string) bool {
	mt := mediaType(contentType)

	return mt == "text/html" || mt == "application/xhtml+xml"
}

// isFeedContent reports whether a Content-Type unambiguously names a feed.
// Unlike feedLinkTypes it deliberately excludes bare application/json: on an
// entry document that more likely means an API endpoint (apiSniffDetector's
// claim), while on a <link rel="alternate"> it is the JSON Feed convention.
// A real JSON Feed served as application/json is still caught by
// looksLikeFeedBody's jsonfeed.org marker.
func isFeedContent(contentType string) bool {
	switch mediaType(contentType) {
	case "application/rss+xml", "application/atom+xml",
		"application/feed+json", "text/rss+xml":
		return true
	}

	return false
}

// looksLikeFeedBody sniffs an ambiguous body (xml/text/octet-stream) for
// feed root elements — used when the Content-Type alone cannot decide.
func looksLikeFeedBody(body []byte) bool {
	snip := strings.TrimSpace(string(body[:min(512, len(body))]))

	return strings.Contains(snip, "<rss") || strings.Contains(snip, "<feed") ||
		strings.Contains(snip, "<rdf:RDF") || strings.Contains(snip, "jsonfeed.org")
}

// looksLikeSitemapBody sniffs the body for sitemap root elements.
func looksLikeSitemapBody(body []byte) bool {
	snip := strings.TrimSpace(string(body[:min(512, len(body))]))

	return strings.Contains(snip, "<urlset") || strings.Contains(snip, "<sitemapindex")
}

// plausiblyFeed reports whether a HEAD-probed path plausibly serves a feed:
// never text/html (SPA catch-all); otherwise a feed/xml/json/octet content
// type or a feed-ish file extension.
func plausiblyFeed(contentType, p string) bool {
	if isHTMLContent(contentType) {
		return false
	}

	ct := mediaType(contentType)

	return ct == "" ||
		strings.Contains(ct, "xml") || strings.Contains(ct, "rss") ||
		strings.Contains(ct, "atom") || strings.Contains(ct, "json") ||
		ct == "application/octet-stream" || ct == "text/plain" ||
		strings.HasSuffix(p, ".xml")
}

func feedLabel(contentType string) string {
	if label, ok := feedLinkTypes[mediaType(contentType)]; ok {
		return label
	}

	return "Feed"
}

// hrefPath returns the path portion of an absolute URL for extension
// matching; unparseable input yields the raw string (path.Ext then finds no
// dot-suffix on a bare host).
func hrefPath(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Path
	}

	return rawURL
}

// attr returns the value of the named attribute on an element node.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}

// walk calls fn for every element node in the tree (depth-first).
func walk(n *html.Node, fn func(*html.Node)) {
	if n == nil {
		return
	}

	if n.Type == html.ElementNode {
		fn(n)
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

// absURL resolves href against the page's final URL; returns "" for
// non-http(s), javascript:, mailto:, or unparseable refs.
func absURL(page *FetchedPage, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}

	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}

	base, err := url.Parse(page.FinalURL)
	if err != nil {
		return ""
	}

	abs := base.ResolveReference(ref)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}

	return abs.String()
}

// visibleTextLen counts text bytes outside script/style/noscript/template —
// the SPA signal: client-rendered shells ship negligible text.
func visibleTextLen(doc *html.Node) int {
	skip := map[string]bool{"script": true, "style": true, "noscript": true, "template": true}

	var n int

	var rec func(*html.Node)
	rec = func(node *html.Node) {
		if node.Type == html.TextNode {
			n += len(strings.TrimSpace(node.Data))
		}

		if node.Type == html.ElementNode && skip[node.Data] {
			return
		}

		for c := node.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(doc)

	return n
}
