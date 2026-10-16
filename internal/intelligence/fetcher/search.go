package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// SearchFetcher polls a SERP provider for a channel's query and emits the
// result page as NDJSON (one {"title","url","snippet","published_at?"}
// object per line, content_type application/x-ndjson). The downstream
// curator keys records on the result URL, so re-polls dedupe cleanly.
//
// Provider resolution (spec §3): provider_config.provider wins, then the
// intel.search.provider system setting. The API key always comes from
// intel.search.api_key (AES-256-GCM at rest — the payment-secret pattern);
// decrypt failure disables the adapter rather than treating ciphertext as
// plaintext. Missing provider/key → ErrAdapterDisabled (benign skip).
//
// provider_config fields:
//
//	{ "query": "japa scholarships",            // required
//	  "provider": "brave",                      // optional; default = setting
//	  "result_count": 20,                       // optional; capped at 50
//	  "recency": "week",                        // day|week|month|year
//	  "engine_id": "cx-token",                  // google_cse required
//	  "endpoint_override": "https://..." }      // tests / self-hosted gateways
type SearchFetcher struct {
	client   *http.Client
	settings AdapterSettings
	// allowPrivate relaxes the insert-time endpoint guard for httptest
	// loopback servers. Set only via test-injected clients
	// (NewCompositeWithClient); production wiring leaves it false and the
	// SafeClient dial guard stays on regardless.
	allowPrivate bool
}

// NewSearchFetcher returns a SearchFetcher backed by an SSRF-safe client
// (nil → the shared safe default). settings feeds provider/key lookups —
// nil leaves the adapter disabled.
func NewSearchFetcher(client *http.Client, settings AdapterSettings) *SearchFetcher {
	if client == nil {
		return &SearchFetcher{client: NewSafeClient(30*time.Second, defaultMaxBodySize), settings: settings}
	}

	return &SearchFetcher{client: client, settings: settings, allowPrivate: true}
}

// searchEndpoints are the built-in provider bases — overridable per
// channel via provider_config.endpoint_override (tests, regional
// gateways). Never user-facing input.
var searchEndpoints = map[string]string{
	"brave":      "https://api.search.brave.com/res/v1/web/search",
	"google_cse": "https://www.googleapis.com/customsearch/v1",
	"serpapi":    "https://serpapi.com/search.json",
}

const (
	searchMaxResults = 50
	searchMaxBody    = 4 << 20 // SERP pages stay small; cap is defensive
)

// Fetch runs one SERP query for the channel and returns the NDJSON result
// document.
func (f *SearchFetcher) Fetch(ctx context.Context, src sources.Source) (FetchResult, error) {
	var cfg struct {
		Query            string `json:"query"`
		Provider         string `json:"provider"`
		ResultCount      int    `json:"result_count"`
		Recency          string `json:"recency"`
		EngineID         string `json:"engine_id"`
		EndpointOverride string `json:"endpoint_override"`
	}

	if err := json.Unmarshal(src.ProviderConfig, &cfg); err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: search provider_config: %w", err)
	}

	if strings.TrimSpace(cfg.Query) == "" {
		return FetchResult{}, errors.New("fetcher: search channel has no provider_config.query")
	}

	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" && f.settings != nil {
		provider = strings.TrimSpace(f.settings.GetStringOr(ctx, "intel.search.provider", ""))
	}

	if provider == "" {
		return FetchResult{}, ErrAdapterDisabled
	}

	if _, ok := searchEndpoints[provider]; !ok {
		return FetchResult{}, fmt.Errorf("fetcher: unknown search provider %q", provider)
	}

	key, err := f.searchAPIKey(ctx)
	if err != nil {
		return FetchResult{}, err
	}

	if key == "" {
		return FetchResult{}, ErrAdapterDisabled
	}

	count := cfg.ResultCount

	switch {
	case count <= 0:
		count = 10
	case count > searchMaxResults:
		count = searchMaxResults
	}

	endpoint := strings.TrimRight(cfg.EndpointOverride, "/")
	if endpoint == "" {
		endpoint = searchEndpoints[provider]
	} else if err := f.validateSearchEndpointOverride(endpoint); err != nil {
		return FetchResult{}, err
	}

	req, err := f.buildRequest(ctx, provider, endpoint, key, cfg.Query, count, cfg.Recency, cfg.EngineID)
	if err != nil {
		return FetchResult{}, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		// *url.Error formats the full request URL — for serpapi/google_cse
		// that URL carries the API key in the query string. Unwrap it so
		// task errors and logs never persist the credential.
		var ue *url.Error
		if errors.As(err, &ue) {
			return FetchResult{}, fmt.Errorf("fetcher: search request: %w", ue.Err)
		}

		return FetchResult{}, fmt.Errorf("fetcher: search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, searchMaxBody))
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: search read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("fetcher: search provider returned %d", resp.StatusCode)
	}

	results, err := parseSearchResults(provider, body)
	if err != nil {
		return FetchResult{}, err
	}

	ndjson, err := searchNDJSON(results)
	if err != nil {
		return FetchResult{}, err
	}

	return FetchResult{
		HTTPStatus:  resp.StatusCode,
		Headers:     map[string]string{"X-Intel-Search-Provider": provider},
		ContentType: SearchContentType,
		Body:        ndjson,
		ContentHash: NormalizeForHash(ndjson, SearchContentType),
	}, nil
}

// SearchContentType marks a raw doc holding a SERP result page — one
// NDJSON result object per line; the search curator parses it.
const SearchContentType = "application/x-ndjson"

// searchAPIKey decrypts intel.search.api_key. An empty setting disables
// the adapter ("" returned); a decrypt failure disables it too — never
// fall back to using ciphertext as the key.
func (f *SearchFetcher) searchAPIKey(ctx context.Context) (string, error) {
	if f.settings == nil {
		return "", nil
	}

	raw := strings.TrimSpace(f.settings.GetStringOr(ctx, "intel.search.api_key", ""))
	if raw == "" {
		return "", nil
	}

	key, err := crypto.Decrypt(raw)
	if err != nil {
		// Ciphertext that won't decrypt means a rotated/corrupt key —
		// treat as "not configured" so the poll skips benignly instead
		// of burning the channel's consecutive-failure budget.
		slog.Warn("fetcher: intel.search.api_key decrypt failed; search disabled", "error", err)
		return "", nil
	}

	return key, nil
}

// buildRequest assembles the provider-shaped GET with query, result
// count, and recency mapped onto each provider's parameter names.
func (f *SearchFetcher) buildRequest(ctx context.Context, provider, endpoint, key, query string, count int, recency, engineID string) (*http.Request, error) {
	q := url.Values{}

	var req *http.Request

	var err error

	switch provider {
	case "brave":
		q.Set("q", query)
		q.Set("count", strconv.Itoa(min(count, 20))) // Brave caps count at 20

		if fr := braveFreshness(recency); fr != "" {
			q.Set("freshness", fr)
		}

		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
		if err == nil {
			req.Header.Set("X-Subscription-Token", key)
			req.Header.Set("Accept", "application/json")
		}
	case "google_cse":
		if engineID == "" {
			return nil, errors.New("fetcher: google_cse requires provider_config.engine_id")
		}

		q.Set("key", key)
		q.Set("cx", engineID)
		q.Set("q", query)
		q.Set("num", strconv.Itoa(min(count, 10))) // CSE caps num at 10

		if dr := cseDateRestrict(recency); dr != "" {
			q.Set("dateRestrict", dr)
		}

		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	case "serpapi":
		q.Set("engine", "google")
		q.Set("q", query)
		q.Set("num", strconv.Itoa(count))
		q.Set("api_key", key)
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	}

	if err != nil {
		return nil, fmt.Errorf("fetcher: build search request: %w", err)
	}

	return req, nil
}

// validateSearchEndpointOverride constrains provider_config.endpoint_override:
// the request carries the decrypted intel.search.api_key (query param or
// header), so plaintext http would leak the key in transit — https only.
// Userinfo is rejected (credential leak into logs), and private-literal +
// numeric-IP hosts are rejected at insert time on top of the SSRF dial
// guard. Test-injected clients (allowPrivate) relax the loopback/private
// rule for httptest servers; production wiring never sets it.
func (f *SearchFetcher) validateSearchEndpointOverride(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return fmt.Errorf("fetcher: search endpoint_override must be a valid URL: %q", endpoint)
	}

	if u.Scheme != "https" {
		if !f.allowPrivate || u.Scheme != "http" {
			return fmt.Errorf("fetcher: search endpoint_override must be https: %q", endpoint)
		}

		host := u.Hostname()
		if !isLoopbackOrPrivateLiteral(host) {
			return fmt.Errorf("fetcher: search endpoint_override must be https for non-local hosts: %q", endpoint)
		}

		return nil
	}

	if u.User != nil {
		return errors.New("fetcher: search endpoint_override must not embed credentials")
	}

	host := u.Hostname()
	if sources.IsPrivateIPLiteral(host) || isNumericIPPrivate(host) {
		return fmt.Errorf("fetcher: search endpoint_override targets a private address: %q", endpoint)
	}

	return nil
}

// isLoopbackOrPrivateLiteral reports whether host is localhost or a
// private-literal/numeric IP — the test-only http exception set.
func isLoopbackOrPrivateLiteral(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}

	return sources.IsPrivateIPLiteral(host) || isNumericIPPrivate(host)
}

// braveFreshness maps the recency enum onto Brave's freshness param.
func braveFreshness(recency string) string {
	switch strings.ToLower(strings.TrimSpace(recency)) {
	case "day":
		return "pd"
	case "week":
		return "pw"
	case "month":
		return "pm"
	case "year":
		return "py"
	}

	return ""
}

// cseDateRestrict maps the recency enum onto Google CSE's dateRestrict.
func cseDateRestrict(recency string) string {
	switch strings.ToLower(strings.TrimSpace(recency)) {
	case "day":
		return "d1"
	case "week":
		return "w1"
	case "month":
		return "m1"
	case "year":
		return "y1"
	}

	return ""
}

// searchResult is the normalized SERP hit all providers parse into.
type searchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet"`
	PublishedAt string `json:"published_at,omitempty"`
}

// parseSearchResults decodes each provider's response shape into the
// normalized hit list.
func parseSearchResults(provider string, body []byte) ([]searchResult, error) {
	switch provider {
	case "brave":
		var r struct {
			Web struct {
				Results []struct {
					Title       string `json:"title"`
					URL         string `json:"url"`
					Description string `json:"description"`
					Age         string `json:"age"`
				} `json:"results"`
			} `json:"web"`
		}

		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("fetcher: brave response: %w", err)
		}

		out := make([]searchResult, 0, len(r.Web.Results))
		for _, it := range r.Web.Results {
			out = append(out, searchResult{Title: it.Title, URL: it.URL, Snippet: it.Description, PublishedAt: it.Age})
		}

		return out, nil
	case "google_cse":
		var r struct {
			Items []struct {
				Title   string `json:"title"`
				Link    string `json:"link"`
				Snippet string `json:"snippet"`
			} `json:"items"`
		}

		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("fetcher: google_cse response: %w", err)
		}

		out := make([]searchResult, 0, len(r.Items))
		for _, it := range r.Items {
			out = append(out, searchResult{Title: it.Title, URL: it.Link, Snippet: it.Snippet})
		}

		return out, nil
	case "serpapi":
		var r struct {
			Organic []struct {
				Title   string `json:"title"`
				Link    string `json:"link"`
				Snippet string `json:"snippet"`
				Date    string `json:"date"`
			} `json:"organic_results"`
		}

		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("fetcher: serpapi response: %w", err)
		}

		out := make([]searchResult, 0, len(r.Organic))
		for _, it := range r.Organic {
			out = append(out, searchResult{Title: it.Title, URL: it.Link, Snippet: it.Snippet, PublishedAt: it.Date})
		}

		return out, nil
	}

	return nil, fmt.Errorf("fetcher: unknown search provider %q", provider)
}

// searchNDJSON marshals the hit list to one JSON object per line; result
// URLs that are not absolute http(s) are dropped (a provider quirk must
// not smuggle javascript:/file: links into the record stream).
func searchNDJSON(results []searchResult) ([]byte, error) {
	var b strings.Builder

	for _, r := range results {
		u, err := url.Parse(r.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}

		line, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("fetcher: marshal result: %w", err)
		}

		b.Write(line)
		b.WriteByte('\n')
	}

	return []byte(b.String()), nil
}
