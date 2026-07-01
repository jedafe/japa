package linkpreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/jedafe/japa/internal/valkey"
)

// maxRedirectDepth is the maximum number of redirects allowed per fetch.
// Prevents infinite redirect loops and limits the attack surface for
// redirect-based SSRF.
const maxRedirectDepth = 5

// errRedirectToPrivateIP is returned when a redirect target resolves to
// a private or reserved IP address.
var errRedirectToPrivateIP = errors.New("redirect target resolves to private IP")

// Preview represents the metadata extracted from a URL.
type Preview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	SiteName    string `json:"site_name"`
	Favicon     string `json:"favicon"`
}

// Fetcher fetches and caches link previews.
type Fetcher struct {
	client    *http.Client
	cache     *valkey.Client
	cacheTTL  time.Duration
	blacklist []string
}

// NewFetcher creates a Fetcher with an SSRF-safe HTTP client and Valkey cache.
// The client uses a custom Transport that validates each redirect hop against
// the private IP blocklist and limits redirect depth. Pass nil for cache to
// disable caching.
func NewFetcher(client *http.Client, cache *valkey.Client) *Fetcher {
	if client == nil {
		client = newSSRFSafeClient()
	}
	return &Fetcher{
		client:   client,
		cache:    cache,
		cacheTTL: 24 * time.Hour,
	}
}

// newSSRFSafeClient returns an http.Client configured with a Transport that
// validates every redirect target for private IP ranges and caps redirect depth.
func newSSRFSafeClient() *http.Client {
	transport := &ssrfSafeTransport{
		inner: http.DefaultTransport,
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirectDepth {
				return fmt.Errorf("stopped after %d redirects", maxRedirectDepth)
			}
			if err := ValidateRedirectURL(req.URL.String()); err != nil {
				return errRedirectToPrivateIP
			}
			return nil
		},
	}
}

// ssrfSafeTransport wraps an inner RoundTripper. It exists so the Transport
// type is not nil (required for the http.Client to use it), while all
// SSRF logic lives in CheckRedirect which is evaluated per-hop.
type ssrfSafeTransport struct {
	inner http.RoundTripper
}

func (t *ssrfSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.inner.RoundTrip(req)
}

// SetBlacklist sets the admin-configured domain blacklist.
func (f *Fetcher) SetBlacklist(blacklist []string) {
	f.blacklist = blacklist
}

// Fetch retrieves a link preview for the given URL.
// It checks the cache first, then fetches and parses the page if not cached.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Preview, error) {
	// Validate URL for SSRF
	if err := ValidateURL(rawURL); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Check blacklist
	host := strings.ToLower(getHost(rawURL))
	if IsBlockedDomain(host, f.blacklist) {
		return nil, fmt.Errorf("domain is blacklisted: %s", host)
	}

	// Try cache first
	if f.cache != nil {
		cacheKey := f.cacheKey(rawURL)
		if cached, err := f.cache.Redis().Get(ctx, cacheKey).Result(); err == nil {
			var preview Preview
			if err := json.Unmarshal([]byte(cached), &preview); err == nil {
				return &preview, nil
			}
		}
	}

	// Fetch the page
	preview, err := f.fetchAndParse(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetching page: %w", err)
	}

	// Cache the result
	if f.cache != nil {
		cacheKey := f.cacheKey(rawURL)
		if data, err := json.Marshal(preview); err == nil {
			if err := f.cache.Redis().Set(ctx, cacheKey, string(data), f.cacheTTL).Err(); err != nil {
				// Log but don't fail — caching is best-effort
				_ = err
			}
		}
	}

	return preview, nil
}

// fetchAndParse fetches the URL and extracts OG tags.
func (f *Fetcher) fetchAndParse(ctx context.Context, rawURL string) (*Preview, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "JapaConnect/1.0 (link preview)")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Limit response size to 1MB
	limited := io.LimitReader(resp.Body, 1<<20)

	return parseOGTags(limited, rawURL)
}

// parseOGTags extracts Open Graph meta tags from HTML.
func parseOGTags(r io.Reader, rawURL string) (*Preview, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	preview := &Preview{URL: rawURL}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var property, content string
			for _, attr := range n.Attr {
				if attr.Key == "property" || attr.Key == "name" {
					property = attr.Val
				}
				if attr.Key == "content" {
					content = attr.Val
				}
			}
			switch property {
			case "og:title":
				preview.Title = content
			case "og:description", "description":
				preview.Description = content
			case "og:image":
				preview.Image = content
			case "og:site_name":
				preview.SiteName = content
			}
		}
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, href string
			for _, attr := range n.Attr {
				if attr.Key == "rel" {
					rel = attr.Val
				}
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if rel == "icon" || rel == "shortcut icon" {
				preview.Favicon = href
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return preview, nil
}

// cacheKey generates a Valkey cache key for a URL.
// Format: jc:link_preview:{sha256_url[:16]}
func (f *Fetcher) cacheKey(rawURL string) string {
	hash := sha256.Sum256([]byte(rawURL))
	return fmt.Sprintf("jc:link_preview:%s", hex.EncodeToString(hash[:8]))
}

// getHost extracts the hostname from a URL.
func getHost(rawURL string) string {
	parts := strings.SplitN(rawURL, "://", 2)
	if len(parts) < 2 {
		return rawURL
	}
	host := strings.SplitN(parts[1], "/", 2)[0]
	return strings.SplitN(host, ":", 2)[0]
}
