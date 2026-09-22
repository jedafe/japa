package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jackc/pgx/v5/pgtype"

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

	// 5xx counts toward the breaker; it opens after renderCircuitThreshold
	// consecutive failures — a single transient failure must not blackout
	// every render channel.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	settings := mockAdapterSettings{"intel.render.service_url": srv.URL}
	f := NewRenderFetcher(srv.Client(), settings, rdb)

	for i := 0; i < renderCircuitThreshold; i++ {
		_, err := f.Fetch(t.Context(), sources.Source{SourceUrl: "https://example.com"})
		if err == nil || !strings.Contains(err.Error(), "502") {
			t.Fatalf("fetch %d err = %v, want status 502", i+1, err)
		}
	}

	// Circuit now open → ErrAdapterDisabled without hitting the service.
	if _, err := f.Fetch(t.Context(), sources.Source{SourceUrl: "https://example.com"}); !errors.Is(err, ErrAdapterDisabled) {
		t.Fatalf("open circuit: err = %v, want ErrAdapterDisabled", err)
	}
}

// TestRenderFetcher_CircuitReset: a successful render clears the
// consecutive-failure counter so sparse failures never trip the breaker.
func TestRenderFetcher_CircuitReset(t *testing.T) {
	t.Parallel()

	rdb := startTestValkey(t)

	var failNext atomic.Bool
	failNext.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failNext.Swap(false) {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"html":"<html>ok</html>","final_url":"https://example.com"}`))
	}))
	t.Cleanup(srv.Close)

	settings := mockAdapterSettings{"intel.render.service_url": srv.URL}
	f := NewRenderFetcher(srv.Client(), settings, rdb)
	src := sources.Source{SourceUrl: "https://example.com"}

	// Alternate failure/success — the counter peaks at 1 each round, never
	// reaching the threshold, so the breaker stays closed.
	for i := 0; i < 2*renderCircuitThreshold; i++ {
		failNext.Store(true)
		if _, err := f.Fetch(t.Context(), src); err == nil {
			t.Fatalf("round %d: want 502 failure", i)
		}
		if _, err := f.Fetch(t.Context(), src); err != nil {
			t.Fatalf("round %d: success fetch err = %v (breaker tripped early?)", i, err)
		}
	}
}

// --- WaybackFetcher ---------------------------------------------------------

func TestWaybackFetcher_Manifest(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("url"); got != "https://example.com/page" {
			t.Errorf("cdx url param = %q", got)
		}
		// max_snapshots must map to a NEGATIVE limit — CDX returns the
		// last N captures; a positive limit would pin the manifest to the
		// oldest captures and new snapshots would never appear.
		if got := r.URL.Query().Get("limit"); got != "-25" {
			t.Errorf("cdx limit param = %q, want -25 (last N captures)", got)
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

// TestCompositeFetch_WaybackSinceInjection: a wayback channel without an
// explicit provider_config.since gets the channel's last_success_at
// injected so the CDX manifest lists only captures newer than the last
// successful crawl (incremental "what changed" intel).
func TestCompositeFetch_WaybackSinceInjection(t *testing.T) {
	t.Parallel()

	var gotSince string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSince = r.URL.Query().Get("since")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[["timestamp","original","statuscode"],["20240102000000","https://example.com/p","200"]]`))
	}))
	t.Cleanup(srv.Close)

	lastOK := pgtype.Timestamptz{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	ch := sources.Channel{
		EntryUrl:       "https://example.com/p",
		CrawlMethod:    "wayback",
		ProviderConfig: []byte(`{"cdx_endpoint":"` + srv.URL + `"}`),
		LastSuccessAt:  lastOK,
	}

	if _, err := testComposite(srv.Client()).Fetch(t.Context(), sources.Source{}, ch); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotSince != "20240101000000" {
		t.Errorf("cdx since = %q, want 20240101000000 (channel last_success_at)", gotSince)
	}

	// An explicit provider_config.since wins over the channel timestamp.
	ch.ProviderConfig = []byte(`{"cdx_endpoint":"` + srv.URL + `","since":"20200101"}`)
	if _, err := testComposite(srv.Client()).Fetch(t.Context(), sources.Source{}, ch); err != nil {
		t.Fatalf("Fetch (explicit since): %v", err)
	}
	if gotSince != "20200101" {
		t.Errorf("cdx since = %q, want explicit 20200101", gotSince)
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

func TestSocialEndpoint_RedditHandles(t *testing.T) {
	t.Parallel()

	// user/<name> must hit /submitted.json — /user/{name}/new.json is not a
	// valid reddit listing and 404s every crawl.
	for _, tc := range []struct {
		handle string
		want   string
	}{
		{"r/nigeria", "https://www.reddit.com/r/nigeria/new.json?limit=50"},
		{"user/someone", "https://www.reddit.com/user/someone/submitted.json?sort=new&limit=50"},
	} {
		got, err := socialEndpoint("reddit", tc.handle)
		if err != nil {
			t.Fatalf("handle %q: %v", tc.handle, err)
		}
		if got != tc.want {
			t.Errorf("handle %q: endpoint = %q, want %q", tc.handle, got, tc.want)
		}
	}

	if _, err := socialEndpoint("reddit", "bare-handle"); err == nil {
		t.Error("bare reddit handle must be rejected (r/ or user/ prefix required)")
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

func TestSocialFetcher_TelegramNormalize(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
			<div class="tgme_widget_message">
				<a class="tgme_widget_message_date" href="https://t.me/testchan/42">
					<time datetime="2026-09-20T10:00:00+00:00"></time>
				</a>
				<div class="tgme_widget_message_text">First <b>post</b></div>
			</div>
			<div class="tgme_widget_message">
				<a class="tgme_widget_message_date" href="https://t.me/testchan/43">
					<time datetime="2026-09-20T11:00:00+00:00"></time>
				</a>
				<div class="tgme_widget_message_text">Second post</div>
				<span class="tgme_widget_message_author">Chan Admin</span>
			</div>
		</body></html>`))
	}))
	t.Cleanup(srv.Close)

	f := NewSocialFetcher(srv.Client(), mockAdapterSettings{
		"intel.social.enabled_providers": `["telegram"]`,
	})
	res, err := f.Fetch(t.Context(), sources.Source{
		ProviderConfig: []byte(`{"provider":"telegram","handle":"testchan","endpoint_override":"` + srv.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(res.Body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("telegram lines = %d, want 2: %q", len(lines), res.Body)
	}
	if !strings.Contains(lines[0], `"url":"https://t.me/testchan/42"`) ||
		!strings.Contains(lines[0], "First post") ||
		!strings.Contains(lines[0], "2026-09-20T10:00:00") {
		t.Errorf("line 0 = %q", lines[0])
	}
	if !strings.Contains(lines[1], `"author":"Chan Admin"`) {
		t.Errorf("line 1 missing author: %q", lines[1])
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

// TestGitFetcher_LocalRepo exercises the files and tags adapters against a
// real repository built with go-git in a temp dir. The fetcher methods are
// invoked directly with a file:// URL — the http(s)-only remote guard
// (validateGitRemote) stays production-only.
func TestGitFetcher_LocalRepo(t *testing.T) {
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init fixture repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := w.Add("data.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	commit, err := w.Commit("fixture commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := repo.CreateTag("v1.0.0", commit, nil); err != nil {
		t.Fatalf("tag: %v", err)
	}

	repoURL := "file://" + dir
	f := NewGitFetcher()

	// via=files → one JSON line per tracked file.
	body, err := f.fetchFiles(t.Context(), repoURL, map[string]any{})
	if err != nil {
		t.Fatalf("fetchFiles: %v", err)
	}
	if !strings.Contains(string(body), `"path":"data.txt"`) || !strings.Contains(string(body), `"content":"payload"`) {
		t.Errorf("files body = %q, want data.txt JSON line", body)
	}

	// paths filter excludes non-matching files.
	body, err = f.fetchFiles(t.Context(), repoURL, map[string]any{"paths": []any{"docs/"}})
	if err != nil {
		t.Fatalf("fetchFiles (paths filter): %v", err)
	}
	if strings.Contains(string(body), "data.txt") {
		t.Errorf("paths=docs/ should exclude data.txt, got %q", body)
	}

	// via=tags → tag<TAB>sha manifest.
	tags, err := f.fetchTags(t.Context(), repoURL)
	if err != nil {
		t.Fatalf("fetchTags: %v", err)
	}
	if !strings.HasPrefix(string(tags), "v1.0.0\t"+commit.String()) {
		t.Errorf("tags body = %q, want v1.0.0\\t%s", tags, commit.String())
	}
}
