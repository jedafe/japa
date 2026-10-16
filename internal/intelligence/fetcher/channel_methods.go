package fetcher

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// channel_methods.go — Phase-1 fetch adapters for the probe-discovered
// channel crawl methods (channel design §4). jsonld and sitemap get thin
// transformers on top of StaticFetcher byte retrieval; ical and tabular
// route straight through the static adapter in Composite.Fetch — the
// curator (Task 9) owns parsing, the fetch step only retrieves bytes +
// computes the change-detection hash.

// ResolveChannelProviderConfig returns the provider_config a channel fetch
// should use. Backfilled default channels carry '{}' — the source's config
// holds the real values — while probe-materialized channels carry their
// own config and must win over a stale source-level document.
//
// Validity is method-aware: api channels keep the strict API-config check
// (a channel carrying only probe metadata like {"probe_confidence":0.9}
// still inherits the source's endpoint); every other method accepts any
// non-empty JSON object — render/social/git configs would not parse as an
// API config but are complete channel configs.
func ResolveChannelProviderConfig(ch sources.Channel, src sources.Source) []byte {
	if sources.CrawlMethod(ch.CrawlMethod) == sources.CrawlMethodAPI {
		if _, err := ParseProviderConfig(ch.ProviderConfig); err == nil {
			return ch.ProviderConfig
		}

		return src.ProviderConfig
	}

	var m map[string]any
	if err := json.Unmarshal(ch.ProviderConfig, &m); err == nil && len(m) > 0 {
		return ch.ProviderConfig
	}

	return src.ProviderConfig
}

// fetchJSONLD retrieves the page via the static path, then replaces the body
// with the raw <script type="application/ld+json"> blocks (newline-joined)
// so the raw document stores exactly the structured data the curator parses.
// The content hash is recomputed over the extracted blocks — cosmetic page
// churn outside the ld+json payloads does not trigger detect_changes.
func (c *Composite) fetchJSONLD(ctx context.Context, src sources.Source) (FetchResult, error) {
	res, err := c.static.Fetch(ctx, src)
	if err != nil {
		return FetchResult{}, err
	}

	blocks, err := extractLDJSONBlocks(res.Body)
	if err != nil {
		return FetchResult{}, err
	}

	if len(blocks) == 0 {
		return FetchResult{}, errors.New("fetcher: jsonld: no application/ld+json blocks found")
	}

	body := []byte(strings.Join(blocks, "\n"))
	contentType := "application/ld+json"

	return FetchResult{
		HTTPStatus:  res.HTTPStatus,
		Headers:     res.Headers,
		ContentType: contentType,
		Body:        body,
		ContentHash: NormalizeForHash(body, contentType),
	}, nil
}

// extractLDJSONBlocks returns the verbatim text of every
// <script type="application/ld+json"> element in the HTML body.
func extractLDJSONBlocks(body []byte) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("fetcher: jsonld: parse html: %w", err)
	}

	var blocks []string

	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, s *goquery.Selection) {
		if text := strings.TrimSpace(s.Text()); text != "" {
			blocks = append(blocks, text)
		}
	})

	return blocks, nil
}

// ManifestContentType is the internal marker content type stamped on
// sitemap/wayback manifest documents — url<TAB>stamp change-detection
// bodies that must never reach curate/extract. detect_changes identifies
// manifests by this marker (independent of the channel row, which can be
// deleted/re-purposed between fetch and detect) — a manifest doc is
// ManifestContentType AND parent_document_id IS NULL.
const ManifestContentType = "application/x-intel-manifest"

// sitemapURLSet / sitemapIndex mirror the two sitemap document shapes.
type sitemapURLSet struct {
	URLs []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"url"`
}

type sitemapIndex struct {
	Sitemaps []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"sitemap"`
}

// manifestKindURLSet / manifestKindIndex identify the sitemap document
// shape a manifest was built from. The value rides the raw doc's
// http_headers as X-Intel-Manifest-Kind so detect_changes knows whether
// fanned-out children are content pages (urlset → static fetch) or
// member sitemaps (index → sitemap fetch, producing a nested manifest).
const (
	// ManifestKindURLSet marks a manifest built from a sitemap urlset —
	// fanned-out children are content pages fetched via the static adapter.
	ManifestKindURLSet = "urlset"
	// ManifestKindIndex marks a manifest built from a sitemapindex —
	// fanned-out children are member sitemaps fetched via the sitemap
	// adapter, producing nested manifest docs.
	ManifestKindIndex = "index"

	// ManifestKindHeader is the http_headers key recording the manifest's
	// source shape on the persisted raw document.
	ManifestKindHeader = "X-Intel-Manifest-Kind"
)

// fetchSitemapManifest retrieves the sitemap via the static path and rewrites
// the body as a URL+lastmod manifest (one "url<TAB>lastmod" line per entry,
// lastmod empty when absent) — the compact document the curator diffs and
// walks. Sitemap indexes are flattened the same way, but the doc records
// the source shape so member sitemaps fan out as nested manifests rather
// than static page fetches.
func (c *Composite) fetchSitemapManifest(ctx context.Context, src sources.Source) (FetchResult, error) {
	res, err := c.static.Fetch(ctx, src)
	if err != nil {
		return FetchResult{}, err
	}

	manifest, kind, err := sitemapManifest(res.Body)
	if err != nil {
		return FetchResult{}, err
	}

	if res.Headers == nil {
		res.Headers = map[string]string{}
	}

	res.Headers[ManifestKindHeader] = kind

	return FetchResult{
		HTTPStatus:  res.HTTPStatus,
		Headers:     res.Headers,
		ContentType: ManifestContentType,
		Body:        manifest,
		ContentHash: NormalizeForHash(manifest, ManifestContentType),
	}, nil
}

// ParseManifest decodes the url<TAB>stamp manifest format shared by the
// sitemap and wayback adapters (Phase 2): one "url<TAB>stamp" line per
// entry — lastmod for sitemaps, snapshot timestamp for wayback. Returns
// url → stamp so callers can diff captures.
func ParseManifest(body []byte) map[string]string {
	out := make(map[string]string)

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		u, stamp, _ := strings.Cut(line, "\t")
		if u = strings.TrimSpace(u); u != "" {
			out[u] = strings.TrimSpace(stamp)
		}
	}

	return out
}

// DiffManifest returns the URLs in cur that are absent from prev or carry
// a different stamp — the new/changed set for the Phase-2 manifest
// fan-out. Input order is preserved.
func DiffManifest(cur, prev []byte) []string {
	prevMap := ParseManifest(prev)

	var changed []string

	seen := map[string]bool{}

	for _, line := range strings.Split(string(cur), "\n") {
		u, stamp, _ := strings.Cut(strings.TrimSpace(line), "\t")
		if u = strings.TrimSpace(u); u == "" || seen[u] {
			continue
		}

		seen[u] = true

		if old, ok := prevMap[u]; !ok || old != strings.TrimSpace(stamp) {
			changed = append(changed, u)
		}
	}

	return changed
}

// sitemapManifest parses a sitemap urlset or sitemapindex document and
// returns the URL+lastmod manifest plus the document kind ("urlset" or
// "index"). An empty urlset is an error — an empty sitemap is almost
// always a fetch or parse defect, and surfacing it beats hashing an empty
// manifest into a "changed" detection cycle.
func sitemapManifest(body []byte) (manifest []byte, kind string, err error) {
	var b strings.Builder

	var urlset sitemapURLSet
	if err := xml.Unmarshal(body, &urlset); err == nil && len(urlset.URLs) > 0 {
		for _, u := range urlset.URLs {
			fmt.Fprintf(&b, "%s\t%s\n", strings.TrimSpace(u.Loc), strings.TrimSpace(u.LastMod))
		}

		return []byte(b.String()), ManifestKindURLSet, nil
	}

	var index sitemapIndex
	if err := xml.Unmarshal(body, &index); err == nil && len(index.Sitemaps) > 0 {
		for _, sm := range index.Sitemaps {
			fmt.Fprintf(&b, "%s\t%s\n", strings.TrimSpace(sm.Loc), strings.TrimSpace(sm.LastMod))
		}

		return []byte(b.String()), ManifestKindIndex, nil
	}

	return nil, "", errors.New("fetcher: sitemap: no url or sitemap entries found")
}
