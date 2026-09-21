package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// mockAdapterSettings is a map-backed AdapterSettings for the config-gated
// Phase-2 adapters (render service URL, social allowlist).
type mockAdapterSettings map[string]string

func (m mockAdapterSettings) GetStringOr(_ context.Context, key, fallback string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return fallback
}

func (m mockAdapterSettings) GetIntOr(_ context.Context, key string, fallback int) int {
	if v, ok := m[key]; ok {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

// --- DiffManifest -----------------------------------------------------------

func TestDiffManifest(t *testing.T) {
	t.Parallel()

	prev := []byte("https://a.example/1\t2024-01-01\nhttps://a.example/2\t2024-01-02\nhttps://a.example/3\t2024-01-03\n")
	cur := []byte("https://a.example/1\t2024-01-01\nhttps://a.example/2\t2024-02-01\nhttps://a.example/4\t2024-01-04\n")

	got := DiffManifest(cur, prev)
	want := []string{"https://a.example/2", "https://a.example/4"}
	if len(got) != len(want) {
		t.Fatalf("DiffManifest = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DiffManifest = %v, want %v", got, want)
		}
	}

	// First crawl (empty prev) — everything is new.
	got = DiffManifest(cur, nil)
	if len(got) != 3 {
		t.Errorf("first-crawl diff = %v, want all 3", got)
	}

	// Unchanged manifest — nothing derived.
	if got := DiffManifest(prev, prev); len(got) != 0 {
		t.Errorf("unchanged diff = %v, want empty", got)
	}
}

// --- RenderFetcher ----------------------------------------------------------

func TestRenderFetcher_Disabled(t *testing.T) {
	t.Parallel()

	f := NewRenderFetcher(nil, nil, nil)
	_, err := f.Fetch(t.Context(), sources.Source{SourceUrl: "https://example.com"})
	if !errors.Is(err, ErrAdapterDisabled) {
		t.Fatalf("unset service_url: err = %v, want ErrAdapterDisabled", err)
	}

	f = NewRenderFetcher(http.DefaultClient, mockAdapterSettings{}, nil)
	if _, err := f.Fetch(t.Context(), sources.Source{SourceUrl: "https://example.com"}); !errors.Is(err, ErrAdapterDisabled) {
		t.Fatalf("empty service_url: err = %v, want ErrAdapterDisabled", err)
	}
}

func TestRenderFetcher_Fetch(t *testing.T) {
	t.Parallel()

	var gotReq renderRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/render" {
			t.Errorf("path = %q, want /render", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"html":"<html><body>rendered</body></html>","final_url":"https://example.com/final"}`))
	}))
	t.Cleanup(srv.Close)

	f := NewRenderFetcher(srv.Client(), mockAdapterSettings{
		"intel.render.service_url": srv.URL,
	}, nil)

	res, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      "https://example.com/spa",
		ProviderConfig: []byte(`{"wait_selector":"#main"}`),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotReq.URL != "https://example.com/spa" || gotReq.WaitSelector != "#main" {
		t.Errorf("request = %+v, want url+wait_selector propagated", gotReq)
	}
	if !strings.Contains(string(res.Body), "rendered") {
		t.Errorf("body = %q", res.Body)
	}
	if res.Headers["X-Render-Final-URL"] != "https://example.com/final" {
		t.Errorf("final_url header = %q", res.Headers["X-Render-Final-URL"])
	}
}

func TestRenderFetcher_Circuit(t *testing.T) {
	t.Parallel()

	rdb := startTestValkey(t)

	// 5xx trips the shared breaker.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	settings := mockAdapterSettings{"intel.render.service_url": srv.URL}
	f := NewRenderFetcher(srv.Client(), settings, rdb)

	_, err := f.Fetch(t.Context(), sources.Source{SourceUrl: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("first fetch err = %v, want status 502", err)
	}

	// Circuit now open → ErrAdapterDisabled without hitting the service.
	if _, err := f.Fetch(t.Context(), sources.Source{SourceUrl: "https://example.com"}); !errors.Is(err, ErrAdapterDisabled) {
		t.Fatalf("open circuit: err = %v, want ErrAdapterDisabled", err)
	}
}

// --- WaybackFetcher ---------------------------------------------------------

func TestWaybackFetcher_Manifest(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("url"); got != "https://example.com/page" {
			t.Errorf("cdx url param = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			["timestamp","original","statuscode"],
			["20240101000000","https://example.com/page","200"],
			["20240201000000","https://example.com/page","200"],
			["20240301000000","https://example.com/page","404"]
		]`))
	}))
	t.Cleanup(srv.Close)

	f := &WaybackFetcher{client: srv.Client()}
	res, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      "https://example.com/page",
		ProviderConfig: []byte(`{"cdx_endpoint":"` + srv.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(res.Body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("manifest lines = %d, want 2 (non-200 filtered): %q", len(lines), res.Body)
	}
	if !strings.HasPrefix(lines[0], "https://web.archive.org/web/20240101000000id_/https://example.com/page") {
		t.Errorf("manifest line = %q, want web.archive.org snapshot URL", lines[0])
	}
}

// --- SocialFetcher ----------------------------------------------------------

func TestSocialFetcher_ProviderGate(t *testing.T) {
	t.Parallel()

	src := sources.Source{
		ProviderConfig: []byte(`{"provider":"reddit","handle":"r/nigeria"}`),
	}

	// No settings → disabled.
	f := NewSocialFetcher(http.DefaultClient, nil)
	if _, err := f.Fetch(t.Context(), src); !errors.Is(err, ErrAdapterDisabled) {
		t.Fatalf("nil settings: err = %v, want ErrAdapterDisabled", err)
	}

	// Provider not in allowlist → disabled.
	f = NewSocialFetcher(http.DefaultClient, mockAdapterSettings{
		"intel.social.enabled_providers": `["telegram"]`,
	})
	if _, err := f.Fetch(t.Context(), src); !errors.Is(err, ErrAdapterDisabled) {
		t.Fatalf("provider not allowlisted: err = %v, want ErrAdapterDisabled", err)
	}
}

func TestSocialFetcher_RedditNormalize(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"children":[{"data":{
			"id":"abc","title":"Japa story","url":"https://x.example/1",
			"permalink":"/r/nigeria/comments/abc/x/","author":"u1",
			"created_utc":1700000000,"score":42,"selftext":"hello"}}]}}`))
	}))
	t.Cleanup(srv.Close)

	f := NewSocialFetcher(srv.Client(), mockAdapterSettings{
		"intel.social.enabled_providers": `["reddit"]`,
	})
	res, err := f.Fetch(t.Context(), sources.Source{
		ProviderConfig: []byte(`{"provider":"reddit","handle":"r/nigeria","endpoint_override":"` + srv.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.ContentType != "application/x-ndjson" {
		t.Errorf("content_type = %q", res.ContentType)
	}
	if !strings.Contains(string(res.Body), `"id":"abc"`) || !strings.Contains(string(res.Body), `"score":42`) {
		t.Errorf("normalized body = %q", res.Body)
	}
}

// --- GitFetcher -------------------------------------------------------------

func TestGitFetcher_RemoteGuard(t *testing.T) {
	t.Parallel()

	f := NewGitFetcher()

	// Private-literal repo host is rejected before any dial.
	_, err := f.Fetch(t.Context(), sources.Source{
		ProviderConfig: []byte(`{"repo":"http://127.0.0.1:1/repo.git"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "private IP") {
		t.Fatalf("private repo host: err = %v, want private IP rejection", err)
	}

	// Non-http scheme rejected.
	_, err = f.Fetch(t.Context(), sources.Source{
		ProviderConfig: []byte(`{"repo":"ssh://git@example.com/x.git"}`),
	})
	if err == nil {
		t.Fatal("ssh scheme: expected error")
	}
}
