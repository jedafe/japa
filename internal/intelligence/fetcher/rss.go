package fetcher

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// RSSFetcher fetches RSS/Atom feeds via net/http. The raw feed bytes are
// hashed as-is (XML is already stable); individual items are extracted via
// Items for per-item downstream processing.
type RSSFetcher struct {
	client *http.Client
}

// NewRSSFetcher returns an RSSFetcher with a 30s timeout.
func NewRSSFetcher() *RSSFetcher {
	return &RSSFetcher{client: &http.Client{Timeout: 30 * time.Second}}
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
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchResult{}, fmt.Errorf("rss: read body: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return FetchResult{}, fmt.Errorf("rss: parse: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return FetchResult{
		HTTPStatus:  resp.StatusCode,
		Headers:     flattenHeaders(resp.Header),
		ContentType: contentType,
		Body:        body,
		ContentHash: NormalizeForHash(body, contentType),
	}, nil
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
