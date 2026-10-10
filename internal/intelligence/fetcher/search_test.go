package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// TestMain sets JC_ENV before any test runs so crypto.Encrypt can use the
// dev-fallback key material — t.Setenv is unavailable inside parallel
// tests and crypto memoizes its key set on first use.
func TestMain(m *testing.M) {
	_ = os.Setenv("JC_ENV", "test")
	os.Exit(m.Run())
}

// searchTestSettings returns settings carrying the ciphertext api_key for
// the given provider — crypto.Encrypt round-trips under the dev-fallback
// key permitted in test binaries (search.go is the only crypto consumer
// in this package).
func searchTestSettings(t *testing.T, provider string) mockAdapterSettings {
	t.Helper()

	ct, err := crypto.Encrypt("test-api-key")
	if err != nil {
		t.Fatalf("crypto.Encrypt: %v", err)
	}

	return mockAdapterSettings{
		"intel.search.provider": provider,
		"intel.search.api_key":  ct,
	}
}

func searchChannel(t *testing.T, cfg string) sources.Channel {
	t.Helper()
	return sources.Channel{CrawlMethod: "search", EntryUrl: "https://example.com/serp", ProviderConfig: []byte(cfg)}
}

// TestSearchFetcher_Disabled covers the config gates: no provider
// anywhere → ErrAdapterDisabled; provider but no api_key →
// ErrAdapterDisabled. Both are benign skips — the worker must not count
// them as channel failures.
func TestSearchFetcher_Disabled(t *testing.T) {
	t.Parallel()

	ch := searchChannel(t, `{"query":"japa news"}`)

	for _, tc := range []struct {
		name     string
		settings AdapterSettings
	}{
		{"nil_settings", nil},
		{"empty_settings", mockAdapterSettings{}},
		{"provider_no_key", mockAdapterSettings{"intel.search.provider": "brave"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := NewSearchFetcher(http.DefaultClient, tc.settings)
			_, err := f.Fetch(context.Background(), searchSource(ch))
			if !errors.Is(err, ErrAdapterDisabled) {
				t.Fatalf("err = %v, want ErrAdapterDisabled", err)
			}
		})
	}
}

// TestSearchFetcher_MissingQuery rejects a channel with no
// provider_config.query — deterministic validation, not a fetch failure.
func TestSearchFetcher_MissingQuery(t *testing.T) {
	t.Parallel()

	settings := searchTestSettings(t, "brave")
	f := NewSearchFetcher(http.DefaultClient, settings)

	_, err := f.Fetch(context.Background(), searchSource(searchChannel(t, `{}`)))
	if err == nil || errors.Is(err, ErrAdapterDisabled) || !strings.Contains(err.Error(), "query") {
		t.Fatalf("err = %v, want query validation error", err)
	}
}

// TestSearchFetcher_UnknownProvider rejects a provider_config naming an
// unregistered provider.
func TestSearchFetcher_UnknownProvider(t *testing.T) {
	t.Parallel()

	settings := searchTestSettings(t, "brave")
	f := NewSearchFetcher(http.DefaultClient, settings)

	_, err := f.Fetch(context.Background(), searchSource(searchChannel(t,
		`{"query":"x","provider":"askjeeves"}`)))
	if err == nil || !strings.Contains(err.Error(), "unknown search provider") {
		t.Fatalf("err = %v, want unknown-provider error", err)
	}
}

// TestSearchFetcher_Brave exercises the brave provider contract:
// X-Subscription-Token header, q/count/freshness params, response shape
// mapped to NDJSON.
func TestSearchFetcher_Brave(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		gotToken = r.Header.Get("X-Subscription-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web":{"results":[
			{"title":"Scholarship A","url":"https://a.example/x","description":"First hit","age":"2 days ago"},
			{"title":"Scholarship B","url":"https://b.example/y","description":"Second hit"}
		]}}`))
	}))
	defer srv.Close()

	settings := searchTestSettings(t, "brave")
	f := NewSearchFetcher(srv.Client(), settings)

	ch := searchChannel(t, `{"query":"japa scholarships","result_count":20,"recency":"week","endpoint_override":"`+srv.URL+`"}`)
	res, err := f.Fetch(context.Background(), searchSource(ch))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if gotToken != "test-api-key" {
		t.Errorf("X-Subscription-Token = %q, want decrypted key", gotToken)
	}
	if gotQuery.Get("q") != "japa scholarships" {
		t.Errorf("q = %q", gotQuery.Get("q"))
	}
	if gotQuery.Get("count") != "20" {
		t.Errorf("count = %q, want 20", gotQuery.Get("count"))
	}
	if gotQuery.Get("freshness") != "pw" {
		t.Errorf("freshness = %q, want pw", gotQuery.Get("freshness"))
	}

	if res.ContentType != SearchContentType {
		t.Errorf("ContentType = %q, want %q", res.ContentType, SearchContentType)
	}
	if res.Headers["X-Intel-Search-Provider"] != "brave" {
		t.Errorf("provider header = %q", res.Headers["X-Intel-Search-Provider"])
	}

	lines := strings.Split(strings.TrimSpace(string(res.Body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d, want 2 (body=%s)", len(lines), res.Body)
	}
	var hit searchResult
	if err := json.Unmarshal([]byte(lines[0]), &hit); err != nil {
		t.Fatalf("decode line: %v", err)
	}
	if hit.URL != "https://a.example/x" || hit.Title != "Scholarship A" || hit.PublishedAt != "2 days ago" {
		t.Errorf("hit = %+v", hit)
	}
}

// TestSearchFetcher_GoogleCSE exercises the google_cse contract: key/cx
// params from api_key + provider_config.engine_id, num capped at 10,
// dateRestrict mapped from recency.
func TestSearchFetcher_GoogleCSE(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"title":"T","link":"https://g.example/1","snippet":"S"}]}`))
	}))
	defer srv.Close()

	settings := searchTestSettings(t, "google_cse")
	f := NewSearchFetcher(srv.Client(), settings)

	ch := searchChannel(t, `{"query":"visa jobs","result_count":30,"recency":"month","engine_id":"cx-123","endpoint_override":"`+srv.URL+`"}`)
	res, err := f.Fetch(context.Background(), searchSource(ch))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if gotQuery.Get("key") != "test-api-key" {
		t.Errorf("key param = %q, want decrypted key", gotQuery.Get("key"))
	}
	if gotQuery.Get("cx") != "cx-123" {
		t.Errorf("cx = %q", gotQuery.Get("cx"))
	}
	if gotQuery.Get("num") != "10" {
		t.Errorf("num = %q, want 10 (CSE cap)", gotQuery.Get("num"))
	}
	if gotQuery.Get("dateRestrict") != "m1" {
		t.Errorf("dateRestrict = %q, want m1", gotQuery.Get("dateRestrict"))
	}
	if !strings.Contains(string(res.Body), "https://g.example/1") {
		t.Errorf("body missing result URL: %s", res.Body)
	}
}

// TestSearchFetcher_GoogleCSE_NoEngineID requires engine_id for CSE.
func TestSearchFetcher_GoogleCSE_NoEngineID(t *testing.T) {
	t.Parallel()

	settings := searchTestSettings(t, "google_cse")
	f := NewSearchFetcher(http.DefaultClient, settings)

	_, err := f.Fetch(context.Background(), searchSource(searchChannel(t, `{"query":"x"}`)))
	if err == nil || !strings.Contains(err.Error(), "engine_id") {
		t.Fatalf("err = %v, want engine_id error", err)
	}
}

// TestSearchFetcher_SerpAPI exercises the serpapi contract: api_key in
// the query, organic_results parsed.
func TestSearchFetcher_SerpAPI(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"organic_results":[
			{"title":"R1","link":"https://s.example/1","snippet":"snip","date":"Jan 2025"}
		]}`))
	}))
	defer srv.Close()

	settings := searchTestSettings(t, "serpapi")
	f := NewSearchFetcher(srv.Client(), settings)

	ch := searchChannel(t, `{"query":"remittance rates","result_count":5,"endpoint_override":"`+srv.URL+`"}`)
	res, err := f.Fetch(context.Background(), searchSource(ch))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if gotQuery.Get("api_key") != "test-api-key" {
		t.Errorf("api_key param = %q", gotQuery.Get("api_key"))
	}
	if gotQuery.Get("engine") != "google" {
		t.Errorf("engine = %q", gotQuery.Get("engine"))
	}
	if gotQuery.Get("num") != "5" {
		t.Errorf("num = %q", gotQuery.Get("num"))
	}
	if !strings.Contains(string(res.Body), "Jan 2025") {
		t.Errorf("body missing published_at: %s", res.Body)
	}
}

// TestSearchFetcher_ResultCountCap bounds provider_config.result_count —
// 50 platform-wide, 20 for Brave (provider cap). An oversized channel
// config must not buy an unbounded SERP page or a provider 400.
func TestSearchFetcher_ResultCountCap(t *testing.T) {
	t.Parallel()

	var gotCount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCount = r.URL.Query().Get("count")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web":{"results":[]}}`))
	}))
	defer srv.Close()

	settings := searchTestSettings(t, "brave")
	f := NewSearchFetcher(srv.Client(), settings)

	ch := searchChannel(t, `{"query":"x","result_count":500,"endpoint_override":"`+srv.URL+`"}`)
	if _, err := f.Fetch(context.Background(), searchSource(ch)); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotCount != "20" {
		t.Errorf("count = %q, want 20 (Brave provider cap)", gotCount)
	}
}

// TestSearchFetcher_EndpointOverrideScheme gates plaintext overrides: the
// request carries the decrypted API key, so non-local http endpoints are
// rejected. Loopback/private-literal hosts (tests, local gateways) may
// still use http.
func TestSearchFetcher_EndpointOverrideScheme(t *testing.T) {
	t.Parallel()

	settings := searchTestSettings(t, "brave")
	f := NewSearchFetcher(http.DefaultClient, settings)

	for _, tc := range []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"public http rejected", "http://serp.attacker.example/x", true},
		{"non-url scheme rejected", "file:///etc/passwd", true},
		{"public https allowed", "https://gateway.example/serp", false},
		{"loopback http allowed", "http://127.0.0.1:9999/serp", false},
		{"localhost http allowed", "http://localhost:9999/serp", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := f.validateSearchEndpointOverride(tc.endpoint); (err != nil) != tc.wantErr {
				t.Errorf("validateSearchEndpointOverride(%q) err = %v, wantErr %v", tc.endpoint, err, tc.wantErr)
			}
		})
	}

	// And through Fetch: a rejected override fails before any request.
	ch := searchChannel(t, `{"query":"x","endpoint_override":"http://serp.attacker.example/x"}`)
	if _, err := f.Fetch(context.Background(), searchSource(ch)); err == nil ||
		!strings.Contains(err.Error(), "endpoint_override") {
		t.Fatalf("Fetch with http override = %v, want endpoint_override error", err)
	}
}

// TestSearchFetcher_Non200 surfaces provider errors as fetch failures
// (retryable — rate limits and 5xx should not be silently dropped).
func TestSearchFetcher_Non200(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	settings := searchTestSettings(t, "brave")
	f := NewSearchFetcher(srv.Client(), settings)

	ch := searchChannel(t, `{"query":"x","endpoint_override":"`+srv.URL+`"}`)
	_, err := f.Fetch(context.Background(), searchSource(ch))
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v, want provider-status error", err)
	}
}

// TestSearchNDJSON_DropsNonHTTP verifies the URL guard: non-http(s)
// result URLs (javascript:, relative, malformed) never reach the record
// stream.
func TestSearchNDJSON_DropsNonHTTP(t *testing.T) {
	t.Parallel()

	out, err := searchNDJSON([]searchResult{
		{Title: "ok", URL: "https://a.example/1"},
		{Title: "js", URL: "javascript:alert(1)"},
		{Title: "rel", URL: "/relative/path"},
		{Title: "bad", URL: "://nope"},
		{Title: "ok2", URL: "http://b.example/2"},
	})
	if err != nil {
		t.Fatalf("searchNDJSON: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2 (out=%s)", len(lines), out)
	}
	if !strings.Contains(lines[0], "a.example") || !strings.Contains(lines[1], "b.example") {
		t.Errorf("unexpected lines: %v", lines)
	}
}

// searchSource folds a channel into the effective-source shape the
// adapters consume (Composite does this in prod; tests go direct).
func searchSource(ch sources.Channel) sources.Source {
	return sources.Source{
		SourceUrl:      ch.EntryUrl,
		CrawlMethod:    ch.CrawlMethod,
		ProviderConfig: ch.ProviderConfig,
	}
}
