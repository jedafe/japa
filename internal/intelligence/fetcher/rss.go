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
	req, err := http.NewRequestWithContext(ctx, "GET", source.SourceUrl, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("rss: new request: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japaconnect.com)")

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("rss: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(NewMaxBodyReader(resp.Body, defaultMaxBodySize))
	if err != nil {
		return FetchResult{}, fmt.Errorf("rss: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return FetchResult{}, fmt.Errorf("rss: unexpected HTTP status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") || looksLikeHTML(body) {
		return FetchResult{}, fmt.Errorf("rss: response is not a valid RSS feed (got HTML/Cloudflare challenge)")
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
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

// Items extracts individual feed items from a raw RSS body — each becomes a
// per-item fetch for the extractor stage.
func (f *RSSFetcher) Items(body []byte) ([]rssItem, error) {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("rss: parse items: %w", err)
	}
	return feed.Items, nil
}
