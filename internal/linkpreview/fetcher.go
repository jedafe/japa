package linkpreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
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
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Image       string   `json:"image"`
	SiteName    string   `json:"site_name"`
	Favicon     string   `json:"favicon"`
	Keywords    []string `json:"keywords,omitempty"`
}

// fetcherSettingsReader is the subset of config.Settings the Fetcher needs to
// read the per-request timeout and domain blacklist from system_settings
// (SEC-013 / Rule 3). *config.Settings satisfies this structurally. When nil,
// compiled-in defaults are used.
type fetcherSettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetSetting(ctx context.Context, key string) (string, error)
}

// defaultFetchTimeoutSeconds is the compiled-in fallback for the HTTP timeout
// when no settings reader is wired (SEC-013).
const defaultFetchTimeoutSeconds = 10

// Fetcher fetches and caches link previews.
type Fetcher struct {
	client    *http.Client
	cache     *valkey.Client
	cacheTTL  time.Duration
	blacklist []string
	settings  fetcherSettingsReader
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

// SetSettings wires the hot-reloadable settings reader for the per-request
// HTTP timeout (SEC-013 / Rule 3). When wired, each Fetch resolves
// `linkpreview.fetch_timeout_seconds` (default 10) and applies it as a
// per-request context deadline; otherwise the compiled-in default is used.
func (f *Fetcher) SetSettings(sr fetcherSettingsReader) { f.settings = sr }

// resolveTimeout returns the per-request fetch timeout. When a settings
// reader is wired, reads `linkpreview.fetch_timeout_seconds` (min 1s, max 120s
// per catalog) for the given platform; otherwise falls back to
// defaultFetchTimeoutSeconds (SEC-013).
func (f *Fetcher) resolveTimeout(ctx context.Context) time.Duration {
	if f.settings == nil {
		return time.Duration(defaultFetchTimeoutSeconds) * time.Second
	}

	secs := f.settings.GetIntOr(ctx, "linkpreview.fetch_timeout_seconds", defaultFetchTimeoutSeconds)
	if secs < 1 {
		secs = 1
	}

	if secs > 120 {
		secs = 120
	}

	return time.Duration(secs) * time.Second
}

// resolveBlacklist returns the domain blacklist for the given platform. When a
// settings reader is wired, reads `content.link_preview.blacklist` (a JSON
// array) per-request so admin changes take effect immediately (BUG-004). When
// no settings reader is wired, falls back to the in-memory blacklist set via
// SetBlacklist (BUG-001: parsed as JSON, not CSV).
func (f *Fetcher) resolveBlacklist(ctx context.Context) []string {
	if f.settings != nil {
		raw, err := f.settings.GetSetting(ctx, "content.link_preview.blacklist")
		if err == nil && raw != "" {
			if bl := ParseBlacklistJSON(raw); len(bl) > 0 {
				return bl
			}
		}
	}

	return f.blacklist
}

// ParseBlacklistJSON parses a system_settings value as a JSON array of
// domains. Tolerates a legacy comma-separated string by splitting on commas
// when JSON parsing fails (BUG-001).
func ParseBlacklistJSON(raw string) []string {
	raw = strings.TrimSpace(raw)

	var domains []string

	if json.Unmarshal([]byte(raw), &domains) == nil {
		out := make([]string, 0, len(domains))

		for _, d := range domains {
			if d = strings.TrimSpace(d); d != "" {
				out = append(out, d)
			}
		}

		return out
	}
	// Legacy CSV fallback for values stored before the JSON convention.
	if raw != "" && raw != "[]" {
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))

		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}

		return out
	}

	return nil
}

// newSSRFSafeClient returns an http.Client configured with a Transport that
// validates every redirect target for private IP ranges and caps redirect depth.
// The per-request timeout is applied via context deadline in fetchAndParse
// (SEC-013), so no fixed Client.Timeout is set here — settings can raise it
// above the compiled-in default when wired.
//
// BUG-006: The Transport uses a custom net.Dialer with a Control function that
// validates the resolved IP at TCP connection time, closing the TOCTOU/DNS
// rebinding window between ValidateURL (DNS lookup at validation time) and the
// actual connection.
func newSSRFSafeClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("unparseable IP at connection time: %s", host)
			}
			if IsPrivateIP(ip) {
				return fmt.Errorf("connection to private IP blocked at dial time: %s", ip)
			}
			return nil
		},
	}

	transport := &ssrfSafeTransport{
		inner: &http.Transport{
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	return &http.Client{
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

// ssrfSafeTransport wraps an inner RoundTripper. The inner transport uses a
// custom Dialer with a Control function that blocks connections to private IP
// ranges at dial time (BUG-006 — DNS rebinding protection). CheckRedirect
// validates each redirect hop's URL before following it.
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
// The ID is used to resolve per-platform settings (fetch timeout, domain
// blacklist) from system_settings (BUG-002, BUG-004).
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Preview, error) {
	// Validate URL for SSRF
	if err := ValidateURL(rawURL); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Check blacklist (resolved per-request from system_settings when wired).
	// Hosts are normalized through normalizeHost (url.Parse Hostname,
	// lowercased, port stripped) so IsBlockedDomain sees the same form the
	// allowlist entries use.
	host := normalizeHost(rawURL)
	blacklist := f.resolveBlacklist(ctx)

	if IsBlockedDomain(host, blacklist) {
		return nil, fmt.Errorf("domain is blacklisted: %s", host)
	}

	// Try cache first
	if f.cache != nil {
		cacheKey := f.cacheKey(rawURL)
		if cached, err := f.cache.Redis().Get(ctx, cacheKey).Result(); err == nil {
			var preview Preview
			if err := json.Unmarshal([]byte(cached), &preview); err != nil {
				slog.Warn("discarding malformed cached link preview", "error", err, "url", rawURL)
			} else {
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

		data, err := json.Marshal(preview)
		if err != nil {
			slog.Warn("marshaling link preview for cache", "error", err, "url", rawURL)
		} else if err := f.cache.Redis().Set(ctx, cacheKey, string(data), f.cacheTTL).Err(); err != nil {
			// Log but don't fail — caching is best-effort.
			slog.Warn("caching link preview", "error", err, "url", rawURL)
		}
	}

	return preview, nil
}

// fetchAndParse fetches the URL and extracts OG tags. The per-request timeout
// is resolved from system_settings when wired (SEC-013), defaulting to 10s.
func (f *Fetcher) fetchAndParse(ctx context.Context, rawURL string) (*Preview, error) {
	// Apply the settings-driven per-request deadline (SEC-013). If the caller
	// already supplied a shorter deadline, WithTimeout keeps the shorter one.
	timeout := f.resolveTimeout(ctx)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// #nosec G704 // pending allowlist validation
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "JapaConnect/1.0 (link preview)")

	// #nosec G704 // pending allowlist validation
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// BUG-005: Only parse HTML responses. Non-HTML content (PDF, image,
	// JSON, etc.) would produce garbage previews from the HTML parser.
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "html") {
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}

	// Limit response size to 1MB
	limited := io.LimitReader(resp.Body, 1<<20)

	// BUG-002: Use the final URL after redirects as the base for resolving
	// relative URLs in OG tags. resp.Request.URL is the last URL fetched.
	baseURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		baseURL = resp.Request.URL.String()
	}

	return parseOGTags(limited, baseURL)
}

// parseOGTags extracts Open Graph meta tags from HTML. The rawURL is the
// final URL after redirects, used as the base for resolving relative URLs
// in og:image and favicon href attributes (BUG-001).
func parseOGTags(r io.Reader, rawURL string) (*Preview, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	preview := &Preview{URL: rawURL}

	// base is the parsed base URL for resolving relative image/favicon URLs.
	base, baseErr := url.Parse(rawURL)

	// resolveURL resolves a potentially relative URL against the page base.
	// If the base URL is invalid or the value is empty, returns the raw value.
	resolveURL := func(val string) string {
		if val == "" || baseErr != nil {
			return val
		}

		ref, err := url.Parse(val)
		if err != nil {
			return val
		}

		return base.ResolveReference(ref).String()
	}

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
				preview.Image = resolveURL(content)
			case "og:site_name":
				preview.SiteName = content
			case "keywords":
				if content != "" {
					parts := strings.Split(content, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							preview.Keywords = append(preview.Keywords, p)
						}
					}
				}
			}
		}

		// BUG-006: Fall back to <title> tag when og:title is missing.
		// Many older sites lack OG tags but have a <title> element.
		if n.Type == html.ElementNode && n.Data == "title" {
			if preview.Title == "" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				preview.Title = strings.TrimSpace(n.FirstChild.Data)
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
				preview.Favicon = resolveURL(href)
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
// Format: jc:link_preview:{sha256_url}. Uses the full SHA-256 hash to
// avoid collision risk from truncation (STD-004). The URL is normalized
// first (lowercased host, sorted query, trimmed trailing slash) so
// equivalent URLs share one cache entry.
func (f *Fetcher) cacheKey(rawURL string) string {
	hash := sha256.Sum256([]byte(normalizeURL(rawURL)))
	return valkey.MustKey("link_preview:%s", hex.EncodeToString(hash[:]))
}

// normalizeHost extracts the hostname via url.Parse (Hostname strips any
// port and lowercases it) and lowercases the result for blocklist
// comparison. Falls back to the legacy string-split for inputs that do
// not parse as URLs.
func normalizeHost(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}

	return strings.ToLower(getHost(rawURL))
}

// normalizeURL canonicalizes a URL for cache-key purposes: lowercase host,
// sorted query parameters, and a trimmed trailing slash on the path. Any
// input that fails to parse is returned unchanged (still hashed safely).
func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}

	u.Host = strings.ToLower(u.Host)
	// url.Values.Encode sorts keys, giving a canonical query form.
	if u.RawQuery != "" {
		u.RawQuery = u.Query().Encode()
	}

	u.Path = strings.TrimSuffix(u.Path, "/")

	return u.String()
}

// getHost extracts the hostname from a URL.
// Deprecated: prefer normalizeHost (url.Parse-based). Retained for
// inputs that do not parse as absolute URLs.
func getHost(rawURL string) string {
	parts := strings.SplitN(rawURL, "://", 2)
	if len(parts) < 2 {
		return rawURL
	}

	host := strings.SplitN(parts[1], "/", 2)[0]

	return strings.SplitN(host, ":", 2)[0]
}
