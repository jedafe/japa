package cdn

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestProviderHeaders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		provider, country, tag string
	}{
		{"cloudflare", "CF-IPCountry", "CF-Cache-Tag"},
		{"bunnycdn", "CDN-RequestCountryCode", "CDN-Tag"},
		{"", "", ""},
		{"fastly", "", ""}, // unknown provider → disabled
	}

	for _, tc := range cases {
		if got := countryHeaderFor(tc.provider); got != tc.country {
			t.Errorf("provider %q: countryHeaderFor = %q, want %q", tc.provider, got, tc.country)
		}

		if got := cacheTagHeaderFor(tc.provider); got != tc.tag {
			t.Errorf("provider %q: cacheTagHeaderFor = %q, want %q", tc.provider, got, tc.tag)
		}
	}
}

func TestProviderOf(t *testing.T) {
	t.Parallel()

	if got := providerOf(Config{}); got != "" {
		t.Fatalf("providerOf(zero) = %q, want empty", got)
	}

	if got := providerOf(Config{Provider: " CloudFlare "}); got != ProviderCloudflare {
		t.Fatalf("providerOf should normalise case/space, got %q", got)
	}
}

func TestSiteURLFor(t *testing.T) {
	t.Parallel()

	if got := siteURLFor(Config{SiteURL: "https://japa.to/"}); got != "https://japa.to" {
		t.Fatalf("siteURLFor = %q (trailing slash must be trimmed)", got)
	}

	if got := siteURLFor(Config{SiteURL: ""}); got != "" {
		t.Fatalf("siteURLFor(empty) = %q", got)
	}
}

func TestCloudflarePurge(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	var bodies [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)

		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer auth")
		}

		_, _ = w.Write([]byte(`{"success":true,"errors":[]}`))
	}))

	defer srv.Close()

	cfg := Config{
		Provider:           ProviderCloudflare,
		CloudflareZoneID:   "zone",
		CloudflareAPIToken: "tok",
	}

	urls := make([]string, 35) // forces the 30-file chunk split
	for i := range urls {
		urls[i] = srv.URL + "/p"
	}

	old := cfAPIBase
	cfAPIBase = srv.URL

	defer func() { cfAPIBase = old }()

	defer func() {
		cfTagPurgeUnsupported.Store(false)
		cfTagRetryAt.Store(0)
	}()

	err := purgeCloudflare(context.Background(), cfg, PurgeRequest{
		URLs: urls,
		Tags: []string{"content:1"},
	})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}

	if calls.Load() != 3 { // 30 + 5 files + 1 tags call
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}

	var last map[string]any
	if err := json.Unmarshal(bodies[len(bodies)-1], &last); err != nil {
		t.Fatal(err)
	}

	if _, ok := last["tags"]; !ok {
		t.Fatal("expected tags purge body")
	}
}

func TestBunnyPurge(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		if r.Header.Get("AccessKey") != "key" {
			t.Errorf("missing AccessKey")
		}

		w.WriteHeader(http.StatusOK)
	}))

	defer srv.Close()

	cfg := Config{
		Provider:        ProviderBunnyCDN,
		BunnyPullZoneID: "42",
		BunnyAPIKey:     "key",
	}

	old := bunnyAPIBase
	bunnyAPIBase = srv.URL

	defer func() { bunnyAPIBase = old }()

	err := purgeBunny(context.Background(), cfg, PurgeRequest{
		URLs: []string{"https://site.test/blog/post"},
		Tags: []string{"content:1", "type:blog_post"},
	})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}

	if calls.Load() != 3 { // 1 url + 2 tags
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestPurgeMissingCredentials(t *testing.T) {
	t.Parallel()

	// Provider set but credentials absent — must no-op, not error.
	if err := purgeCloudflare(context.Background(), Config{}, PurgeRequest{URLs: []string{"https://x/y"}}); err != nil {
		t.Fatalf("missing creds should no-op, got %v", err)
	}

	if err := purgeBunny(context.Background(), Config{}, PurgeRequest{URLs: []string{"https://x/y"}}); err != nil {
		t.Fatalf("missing creds should no-op, got %v", err)
	}
}

// TestContentRoutePrefixParity pins the route-prefix map so accidental edits
// are caught — the map mirrors apps/main/src/lib/utils/contentLink.ts (§14,
// ADR-058); a drifted entry purges the wrong URL.
func TestContentRoutePrefixParity(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"blog_post":       "/blog",
		"guide":           "/guide",
		"discussion":      "/forum",
		"forum_thread":    "/forum",
		"comparison":      "/comparisons",
		"story":           "/story",
		"achievement":     "/achievements",
		"page":            "/pages",
		"job_listing":     "/jobs",
		"service_listing": "/services",
		"video":           "/video",
		"list":            "/list",
	}
	for k, v := range want {
		if got := contentRoutePrefix[k]; got != v {
			t.Errorf("contentRoutePrefix[%q] = %q, want %q", k, got, v)
		}
	}
	if len(contentRoutePrefix) != len(want) {
		t.Errorf("contentRoutePrefix has %d entries, want %d", len(contentRoutePrefix), len(want))
	}
	if got := contentDetailURL("https://japa.to", "unknown_type", "x"); got != "https://japa.to/content/x" {
		t.Errorf("fallback = %q, want /content/x", got)
	}
}

// TestCFTagPurgeRecovery — after an Enterprise-rejection suppression, a zone
// change retries immediately and an elapsed cooldown also retries (plan
// upgrade picks up without a restart). Not parallel: it swaps cfAPIBase.
func TestCFTagPurgeRecovery(t *testing.T) {

	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"not entitled"}]}`))
	}))
	defer srv.Close()

	cfg := Config{Provider: ProviderCloudflare, CloudflareZoneID: "zone", CloudflareAPIToken: "tok"}

	old := cfAPIBase
	cfAPIBase = srv.URL
	defer func() { cfAPIBase = old }()
	defer func() {
		cfTagPurgeUnsupported.Store(false)
		cfTagRetryAt.Store(0)
		cfTagZone.Store("")
	}()

	req := PurgeRequest{Tags: []string{"content:1"}}

	// First rejection → suppressed.
	if err := purgeCloudflare(context.Background(), cfg, req); err == nil {
		t.Fatal("expected rejection error")
	}
	if calls.Load() != 1 || !cfTagPurgeUnsupported.Load() {
		t.Fatalf("expected 1 call + suppression, calls=%d suppressed=%v", calls.Load(), cfTagPurgeUnsupported.Load())
	}

	// Inside cooldown → skipped.
	if err := purgeCloudflare(context.Background(), cfg, req); err != nil {
		t.Fatalf("suppressed tag purge should not error, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("suppressed tag purge should not call API, calls=%d", calls.Load())
	}

	// Cooldown elapsed → retried (and re-rejected → suppressed again).
	cfTagRetryAt.Store(0)
	if err := purgeCloudflare(context.Background(), cfg, req); err == nil {
		t.Fatal("expected rejection error on retry")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected retry after cooldown, calls=%d", calls.Load())
	}

	// Zone change → immediate retry regardless of cooldown.
	cfg.CloudflareZoneID = "zone2"
	if err := purgeCloudflare(context.Background(), cfg, req); err == nil {
		t.Fatal("expected rejection error on zone change")
	}
	if calls.Load() != 3 {
		t.Fatalf("expected retry on zone change, calls=%d", calls.Load())
	}
}

func TestPurgeDisabledProvider(t *testing.T) {
	t.Parallel()

	// Purge on an unknown/empty provider no-ops even with URLs — needs the
	// resolver since dispatch happens there.
	SetConfigResolver(func() Config { return Config{Provider: ""} })

	defer SetConfigResolver(nil)

	if err := Purge(context.Background(), PurgeRequest{URLs: []string{"https://x/y"}}); err != nil {
		t.Fatalf("purge on disabled provider should no-op, got %v", err)
	}
}
