package fetcher

import (
	"context"
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
// should use: the channel's own document when it parses as a valid API
// provider config, otherwise the parent source's. Backfilled default
// channels carry '{}' — for API channels the source's config holds the real
// endpoint — while probe-materialized channels carry their own endpoint
// and must win over a stale source-level document.
func ResolveChannelProviderConfig(ch sources.Channel, src sources.Source) []byte {
	if _, err := ParseProviderConfig(ch.ProviderConfig); err == nil {
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

// fetchSitemapManifest retrieves the sitemap via the static path and rewrites
// the body as a URL+lastmod manifest (one "url<TAB>lastmod" line per entry,
// lastmod empty when absent) — the compact document the curator diffs and
// walks. Sitemap indexes are flattened the same way (member sitemap URLs
// become manifest rows; the curator decides whether to crawl them).
func (c *Composite) fetchSitemapManifest(ctx context.Context, src sources.Source) (FetchResult, error) {
	res, err := c.static.Fetch(ctx, src)
	if err != nil {
		return FetchResult{}, err
	}

	manifest, err := sitemapManifest(res.Body)
	if err != nil {
		return FetchResult{}, err
	}

	contentType := "text/plain; charset=utf-8"

	return FetchResult{
		HTTPStatus:  res.HTTPStatus,
		Headers:     res.Headers,
		ContentType: contentType,
		Body:        manifest,
		ContentHash: NormalizeForHash(manifest, contentType),
	}, nil
}

// sitemapManifest parses a sitemap urlset or sitemapindex document and
// returns the URL+lastmod manifest. An empty urlset is an error — an
// empty sitemap is almost always a fetch or parse defect, and surfacing it
// beats hashing an empty manifest into a "changed" detection cycle.
func sitemapManifest(body []byte) ([]byte, error) {
	var b strings.Builder

	var urlset sitemapURLSet
	if err := xml.Unmarshal(body, &urlset); err == nil && len(urlset.URLs) > 0 {
		for _, u := range urlset.URLs {
			fmt.Fprintf(&b, "%s\t%s\n", strings.TrimSpace(u.Loc), strings.TrimSpace(u.LastMod))
		}

		return []byte(b.String()), nil
	}

	var index sitemapIndex
	if err := xml.Unmarshal(body, &index); err == nil && len(index.Sitemaps) > 0 {
		for _, sm := range index.Sitemaps {
			fmt.Fprintf(&b, "%s\t%s\n", strings.TrimSpace(sm.Loc), strings.TrimSpace(sm.LastMod))
		}

		return []byte(b.String()), nil
	}

	return nil, errors.New("fetcher: sitemap: no url or sitemap entries found")
}
