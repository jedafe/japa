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

	defer cfTagPurgeUnsupported.Store(false)

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
