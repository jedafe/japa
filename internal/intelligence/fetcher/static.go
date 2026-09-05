package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// StaticFetcher fetches plain HTML pages via net/http and normalizes the
// body with goquery (stripping script/style/noscript) before hashing.
type StaticFetcher struct {
	client *http.Client
}

// NewStaticFetcher returns a StaticFetcher with a 30s timeout and SSRF-safe
// transport (redirects to private IPs are blocked; response bodies are
// capped at defaultMaxBodySize by the Fetch method).
func NewStaticFetcher() *StaticFetcher {
	return &StaticFetcher{client: NewSafeClient(30*time.Second, defaultMaxBodySize)}
}

// Fetch implements the Fetcher interface for static HTML sources.
func (f *StaticFetcher) Fetch(ctx context.Context, source sources.Source) (FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.SourceUrl, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("static: new request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japaconnect.com)")

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("static: do: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(NewMaxBodyReader(resp.Body, defaultMaxBodySize))
	if err != nil {
		return FetchResult{}, fmt.Errorf("static: read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	hash := NormalizeForHash(body, contentType)

	return FetchResult{
		HTTPStatus:  resp.StatusCode,
		Headers:     flattenHeaders(resp.Header),
		ContentType: contentType,
		Body:        body,
		ContentHash: hash,
	}, nil
}

// normalizeHTML strips script/style/noscript elements and collapses whitespace
// so that cosmetic HTML changes do not produce a different content hash.
func normalizeHTML(body []byte) []byte {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return body
	}

	doc.Find("script, style, noscript").Remove()
	text := doc.Text()

	return []byte(strings.Join(strings.Fields(text), " "))
}

// flattenHeaders reduces a multi-value http.Header to the first value per key.
func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))

	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}

	return out
}
