package fetcher

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// RSSFetcher fetches RSS/Atom feeds via net/http. The raw feed bytes are
// hashed as-is (XML is already stable); individual items are extracted via
// Items for per-item downstream processing.
type RSSFetcher struct {
	client *http.Client
}

// NewRSSFetcher returns an RSSFetcher with a 30s timeout and SSRF-safe
// transport (redirects to private IPs are blocked; response bodies are
// capped at defaultMaxBodySize by the Fetch method).
func NewRSSFetcher() *RSSFetcher {
	return &RSSFetcher{client: NewSafeClient(30*time.Second, defaultMaxBodySize)}
}

type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

// Fetch implements the Fetcher interface for RSS sources.
func (f *RSSFetcher) Fetch(ctx context.Context, source sources.Source) (FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.SourceUrl, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("rss: new request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japa.to)")
	applyConditionals(ctx, req)

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("rss: do: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(NewMaxBodyReader(resp.Body, defaultMaxBodySize))
	if err != nil {
		return FetchResult{}, fmt.Errorf("rss: read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	// 304: the upstream confirmed our validators are current — the feed
	// is unchanged, the crawl counts as a success, nothing is persisted.
	if resp.StatusCode == http.StatusNotModified {
		return FetchResult{HTTPStatus: resp.StatusCode, ContentType: contentType, NotModified: true}, nil
	}

	// Error responses are fetch failures, not documents — an error page is
	// not feed content and must not reset consecutive_failures or feed the
	// pipeline. httpFailureError classifies the failure (throttled /
	// bot-walled / denied / gone) so the worker routes it correctly.
	if resp.StatusCode >= 400 {
		return FetchResult{}, httpFailureError("rss", resp, body, contentType)
	}

	// Feeds are XML; an html-ish response here is a bot-wall challenge, a
	// login wall, or a misconfigured feed_url. A recognized challenge is
	// blocked-class (escalation runs).
	htmlish := strings.Contains(contentType, "text/html") || looksLikeHTML(body)
	if htmlish {
		if sig, ok := DetectWall(resp.StatusCode, resp.Header, body, contentType); ok {
			return FetchResult{}, &WallError{Method: "rss", Status: resp.StatusCode, Signature: sig}
		}
	}

	// A body that literally opens with <html>/<!doctype is a page, not a
	// feed — XHTML parses as XML, so the parse below cannot rescue it —
	// gone-class, so the channel disables fast instead of retrying a
	// dead URL.
	if looksLikeHTML(body) {
		return FetchResult{}, fmt.Errorf("rss: %w: endpoint returned HTML, expected feed XML", ErrContentGone)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		// The Content-Type label alone is not authoritative — mislabeled
		// feeds (WordPress serving XML as text/html) are common, so
		// non-XML garbage only goes gone-class when the label claimed
		// HTML. A mislabeled VALID feed parses fine and is accepted.
		if htmlish {
			return FetchResult{}, fmt.Errorf("rss: %w: endpoint returned HTML, expected feed XML", ErrContentGone)
		}

		return FetchResult{}, fmt.Errorf("rss: parse: %w", err)
	}

	return FetchResult{
		HTTPStatus:  resp.StatusCode,
		Headers:     flattenHeaders(resp.Header),
		ContentType: contentType,
		Body:        body,
		ContentHash: NormalizeForHash(body, contentType),
	}, nil
}

// looksLikeHTML returns true when a response body starts with an HTML
// doctype or <html> tag. RSS sources sometimes return Cloudflare challenges
// or 404 HTML pages; parsing those with encoding/xml produces the confusing
// "XML syntax error" messages we see in Asynq logs.
func looksLikeHTML(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}

	s := strings.ToLower(string(trimmed[:min(len(trimmed), 128)]))

	return strings.HasPrefix(s, "<!doctype html") || strings.HasPrefix(s, "<html")
}
