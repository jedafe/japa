package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// WaybackFetcher is the Phase-2 wayback adapter (channel design §3): it
// queries the Internet Archive CDX API for captures of the channel's
// entry_url and emits the shared url<TAB>stamp manifest — each line is the
// fetchable snapshot URL (https://web.archive.org/web/{ts}id_/{original})
// with the capture timestamp as its stamp. detect_changes diffs the
// manifest like a sitemap and fans out a derived fetch per new capture —
// "what changed" intel for pages that vanish or shift.
//
// provider_config keys:
//
//	{"since": "20250101",              // earliest capture (CDX timestamp);
//	                                  // Composite injects the channel's
//	                                  // last_success_at when unset
//	 "match_type": "exact|prefix",     // CDX matchType (default exact)
//	 "max_snapshots": 25,              // cap on manifest size (most recent N)
//	 "cdx_endpoint": "..."}            // CDX endpoint override (tests)
//
// The CDX API is a public upstream — requests go through the safetransport
// client like every other outbound call.
type WaybackFetcher struct {
	client *http.Client
	// allowPrivate relaxes the insert-time endpoint guard for httptest
	// loopback servers (see SearchFetcher.allowPrivate).
	allowPrivate bool
}

// NewWaybackFetcher returns a WaybackFetcher backed by an SSRF-safe client.
func NewWaybackFetcher() *WaybackFetcher {
	return &WaybackFetcher{client: NewSafeClient(30*time.Second, defaultMaxBodySize)}
}

const cdxEndpoint = "https://web.archive.org/cdx/search/cdx"

// Fetch implements Fetcher.
func (f *WaybackFetcher) Fetch(ctx context.Context, src sources.Source) (FetchResult, error) {
	var cfg map[string]any
	_ = json.Unmarshal(src.ProviderConfig, &cfg)

	q := url.Values{}
	q.Set("url", src.SourceUrl)
	q.Set("output", "json")
	q.Set("fl", "timestamp,original,statuscode")
	q.Set("filter", "statuscode:200")
	q.Set("collapse", "digest") // one row per unique capture body

	if since := stringFromCfg(cfg, "since"); since != "" {
		q.Set("since", since)
	}

	if mt := stringFromCfg(cfg, "match_type"); mt != "" {
		q.Set("matchType", mt)
	}

	if n := intFromCfg(cfg, "max_snapshots", 25); n > 0 {
		// CDX limit=N returns the FIRST N (oldest) captures — a positive
		// limit would pin the manifest to a page's earliest snapshots and
		// new captures would never appear once the page exceeds the cap.
		// Negative limit returns the LAST N (most recent), which is what
		// change detection needs.
		q.Set("limit", fmt.Sprintf("-%d", n))
	}

	endpoint := strings.TrimRight(stringFromCfg(cfg, "cdx_endpoint"), "/")
	if endpoint == "" {
		endpoint = cdxEndpoint
	} else if err := f.validateWaybackEndpoint(endpoint); err != nil {
		return FetchResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: wayback cdx: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: wayback cdx: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(NewMaxBodyReader(resp.Body, defaultMaxBodySize))
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: wayback read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("fetcher: wayback cdx: status %d", resp.StatusCode)
	}

	manifest, err := waybackManifest(body)
	if err != nil {
		return FetchResult{}, err
	}

	return FetchResult{
		HTTPStatus:  resp.StatusCode,
		Headers:     map[string]string{"Content-Type": ManifestContentType},
		ContentType: ManifestContentType,
		Body:        manifest,
		ContentHash: NormalizeForHash(manifest, ManifestContentType),
	}, nil
}

// waybackManifest converts a CDX JSON response ([["timestamp","original"],
// ...]) into the shared manifest format — one "{snapshot_url}\t{timestamp}"
// line per capture, oldest first. An empty capture set yields an empty
// manifest (a page with no snapshots is a valid answer, not an error).
func waybackManifest(body []byte) ([]byte, error) {
	var rows [][]string
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("fetcher: wayback cdx parse: %w", err)
	}
	// First row is the header when fl fields are named.
	if len(rows) > 0 && len(rows[0]) >= 2 && rows[0][0] == "timestamp" {
		rows = rows[1:]
	}

	var b strings.Builder

	for _, row := range rows {
		if len(row) < 2 {
			continue
		}

		ts, orig := row[0], row[1]
		// Belt-and-braces with the filter param: a capture that 404'd at
		// the Wayback edge is not a snapshot worth crawling.
		if ts == "" || orig == "" || (len(row) >= 3 && row[2] != "" && row[2] != "200") {
			continue
		}

		fmt.Fprintf(&b, "https://web.archive.org/web/%sid_/%s\t%s\n", ts, orig, ts)
	}

	return []byte(b.String()), nil
}

// validateWaybackEndpoint constrains provider_config.cdx_endpoint: https
// only, no userinfo, no private-literal or numeric-IP hosts — on top of
// the SafeClient dial guard. Test-injected clients (allowPrivate) accept
// loopback http for httptest servers. The default web.archive.org
// endpoint bypasses validation (constant, not operator input).
func (f *WaybackFetcher) validateWaybackEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return fmt.Errorf("fetcher: wayback cdx_endpoint must be a valid URL: %q", endpoint)
	}

	if u.Scheme != "https" {
		if !f.allowPrivate || u.Scheme != "http" {
			return fmt.Errorf("fetcher: wayback cdx_endpoint must be https: %q", endpoint)
		}

		if !isLoopbackOrPrivateLiteral(u.Hostname()) {
			return fmt.Errorf("fetcher: wayback cdx_endpoint must be https for non-local hosts: %q", endpoint)
		}

		return nil
	}

	if u.User != nil {
		return errors.New("fetcher: wayback cdx_endpoint must not embed credentials")
	}

	host := u.Hostname()
	if sources.IsPrivateIPLiteral(host) || isNumericIPPrivate(host) {
		return fmt.Errorf("fetcher: wayback cdx_endpoint targets a private address: %q", endpoint)
	}

	return nil
}
