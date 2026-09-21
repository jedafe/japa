package probe

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// --- helpers --------------------------------------------------------------

// roundTripFunc adapts a function to http.RoundTripper — used to simulate
// the safetransport client (canned responses, SSRF rejections) without real
// network access.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func mustFixture(t *testing.T, name string) []byte {
	t.Helper()

	body, err := os.ReadFile(filepath.Clean(filepath.Join("testdata", name)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	return body
}

// serveFixture returns an httptest server whose / serves the named fixture
// as text/html; /robots.txt and everything else 404 unless extra is set.
func serveFixture(t *testing.T, fixture string, extra map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()

	body := mustFixture(t, fixture)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body)
	})

	for path, h := range extra {
		mux.HandleFunc(path, h)
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

// cannedClient simulates an outbound client that answers every GET with a
// 200 text/html body and 404s robots.txt / HEAD probes — the platform tests
// need URL-level detection without hitting real hosts.
func cannedClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodHead || req.URL.Path == "/robots.txt" {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{},
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
			Body: io.NopCloser(strings.NewReader(
				"<html><head><title>x</title></head><body><p>" +
					"A sufficiently long paragraph of real text so the page " +
					"does not look like a client-rendered shell.</p></body></html>")),
			Request: req,
		}, nil
	})}
}

func findMethod(report Report, method string) (DetectedMethod, bool) {
	for _, m := range report.Methods {
		if m.Method == method {
			return m, true
		}
	}

	return DetectedMethod{}, false
}

func findChannel(report Report, method string) (ChannelProposal, bool) {
	for _, c := range report.Channels {
		if c.CrawlMethod == method {
			return c, true
		}
	}

	return ChannelProposal{}, false
}

func hasWarning(report Report, substr string) bool {
	for _, w := range report.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}

	return false
}

// --- required brief tests --------------------------------------------------

// TestPreview_DetectsFeedAndJSONLD covers the brief's step-1 fixture:
// <link rel="alternate" application/rss+xml> + ld+json JobPosting must
// report the rss method, a feed channel proposal, jsonld_types containing
// JobPosting, and requires_js=false.
func TestPreview_DetectsFeedAndJSONLD(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "feed_jsonld.html", nil)

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if _, ok := findMethod(report, "rss"); !ok {
		t.Errorf("methods missing rss: %+v", report.Methods)
	}

	if _, ok := findMethod(report, "jsonld"); !ok {
		t.Errorf("methods missing jsonld: %+v", report.Methods)
	}

	if !slices.Contains(report.JSONLDTypes, "JobPosting") {
		t.Errorf("jsonld_types missing JobPosting: %v", report.JSONLDTypes)
	}

	if report.RequiresJS {
		t.Error("requires_js = true on a server-rendered page")
	}

	feedURL := srv.URL + "/feed"
	if !slices.Contains(report.FeedURLs, feedURL) {
		t.Errorf("feed_urls missing %s: %v", feedURL, report.FeedURLs)
	}

	prop, ok := findChannel(report, "rss")
	if !ok {
		t.Fatalf("channels missing rss proposal: %+v", report.Channels)
	}

	if prop.EntryURL != feedURL {
		t.Errorf("rss proposal entry_url = %q, want %q", prop.EntryURL, feedURL)
	}

	if prop.Pipeline != string(sources.PipelineCurate) {
		t.Errorf("rss proposal pipeline = %q, want curate", prop.Pipeline)
	}
}

// TestPreview_SSRFBlocked verifies that a safetransport-style client
// rejection (private/link-local IP) propagates out of Preview as an error —
// the probe never bypasses the injected transport.
func TestPreview_SSRFBlocked(t *testing.T) {
	t.Parallel()

	blocking := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("fetcher: redirect to private IP 169.254.169.254 rejected (SSRF)")
	})}

	_, err := NewProber(blocking, 5).Preview(t.Context(), "http://169.254.169.254/latest/meta-data", "", nil)
	if err == nil {
		t.Fatal("Preview: expected error for link-local target")
	}

	if !strings.Contains(err.Error(), "SSRF") {
		t.Errorf("expected propagated SSRF error, got: %v", err)
	}
}

// TestPreview_SPAMarkers covers the SPA fixture: a bare <div id="root">
// with negligible text must flag RequiresJS and propose the render method.
func TestPreview_SPAMarkers(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "spa_shell.html", nil)

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if !report.RequiresJS {
		t.Error("requires_js = false on an SPA shell")
	}

	if _, ok := findMethod(report, "render"); !ok {
		t.Errorf("methods missing render: %+v", report.Methods)
	}

	if _, ok := findChannel(report, "render"); !ok {
		t.Errorf("channels missing render proposal: %+v", report.Channels)
	}
}

// --- detectors -------------------------------------------------------------

// TestPreview_SitemapFromRobots verifies a robots.txt Sitemap: declaration
// produces a sitemap method + channel without spending a probe request.
func TestPreview_SitemapFromRobots(t *testing.T) {
	t.Parallel()

	var sitemapURL string

	srv := serveFixture(t, "feed_jsonld.html", map[string]http.HandlerFunc{
		"/robots.txt": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\nSitemap: " + sitemapURL + "\n"))
		},
	})
	sitemapURL = srv.URL + "/sitemap_index.xml"

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if !report.Robots.Fetched {
		t.Error("robots verdict not marked fetched")
	}

	if !slices.Contains(report.SitemapURLs, sitemapURL) {
		t.Errorf("sitemap_urls missing %s: %v", sitemapURL, report.SitemapURLs)
	}

	if !slices.Contains(report.Robots.DisallowedPaths, "/admin") {
		t.Errorf("robots disallowed paths missing /admin: %v", report.Robots.DisallowedPaths)
	}

	if _, ok := findChannel(report, "sitemap"); !ok {
		t.Errorf("channels missing sitemap proposal: %+v", report.Channels)
	}
}

// TestPreview_PathProbes verifies the HEAD convention probes for well-known
// endpoints when the page and robots.txt declare nothing: /sitemap.xml and
// the common feed paths.
func TestPreview_PathProbes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fixture  string
		path     string
		ctype    string
		wantURLs func(report Report) []string
	}{
		{
			name:    "sitemap.xml",
			fixture: "feed_jsonld.html",
			path:    "/sitemap.xml",
			ctype:   "application/xml",
			wantURLs: func(report Report) []string {
				return report.SitemapURLs
			},
		},
		{
			name:    "feed path",
			fixture: "docs_page.html",
			path:    "/feed",
			ctype:   "application/rss+xml",
			wantURLs: func(report Report) []string {
				return report.FeedURLs
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := serveFixture(t, tc.fixture, map[string]http.HandlerFunc{
				tc.path: func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", tc.ctype)
					w.WriteHeader(http.StatusOK)
				},
			})

			report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", nil)
			if err != nil {
				t.Fatalf("Preview: %v", err)
			}

			want := srv.URL + tc.path
			if !slices.Contains(tc.wantURLs(report), want) {
				t.Errorf("probed URL %s missing: %+v", want, report)
			}
		})
	}
}

// TestPreview_WebSubHeader verifies a Link: rel="hub" header flags HasWebSub
// and produces a webhook channel seeded with hub/topic provider_config.
func TestPreview_WebSubHeader(t *testing.T) {
	t.Parallel()

	hub := "https://pubsubhubbub.appspot.com/"

	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Add("Link", "<"+hub+">; rel=\"hub\"")
		w.Header().Add("Link", "<"+srvURL+"/feed>; rel=\"self\"")

		if r.URL.Path == "/" {
			_, _ = w.Write(mustFixture(t, "feed_jsonld.html"))

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if !report.HasWebSub {
		t.Error("has_websub = false despite Link rel=hub header")
	}

	prop, ok := findChannel(report, "webhook")
	if !ok {
		t.Fatalf("channels missing webhook proposal: %+v", report.Channels)
	}

	if got := prop.ProviderConfig["hub"]; got != hub {
		t.Errorf("webhook provider_config[hub] = %v, want %s", got, hub)
	}

	if got := prop.ProviderConfig["topic"]; got != srv.URL+"/feed" {
		t.Errorf("webhook provider_config[topic] = %v, want %s/feed", got, srv.URL)
	}
}

// TestPreview_PlatformSignatures covers the host allowlist: video providers
// route to channel_sync, Medium appends /feed, GitHub becomes git, Reddit
// and Telegram become social channels.
func TestPreview_PlatformSignatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		url          string
		wantKind     string
		wantMethod   string
		wantEntry    string
		wantPipeline string
		wantConfig   map[string]string
	}{
		{
			name:         "youtube channel",
			url:          "https://www.youtube.com/@JapaStories",
			wantKind:     "youtube",
			wantMethod:   MethodChannelSync,
			wantPipeline: "curate",
			wantConfig:   map[string]string{"platform": "youtube", "via": "channel_sync"},
		},
		{
			name:       "medium profile",
			url:        "https://medium.com/@japa.diaries",
			wantKind:   "medium",
			wantMethod: "rss",
			wantEntry:  "https://medium.com/feed/@japa.diaries",
		},
		{
			name:       "github repo",
			url:        "https://github.com/jedafe/japa/tree/main/docs",
			wantKind:   "github",
			wantMethod: "git",
			wantEntry:  "https://github.com/jedafe/japa",
			wantConfig: map[string]string{"provider": "github", "repo": "jedafe/japa"},
		},
		{
			name:       "reddit subreddit",
			url:        "https://www.reddit.com/r/nigeria",
			wantKind:   "reddit",
			wantMethod: "social",
			wantEntry:  "https://www.reddit.com/r/nigeria/.json",
			wantConfig: map[string]string{"provider": "reddit"},
		},
		{
			name:       "telegram channel",
			url:        "https://t.me/japaconnect",
			wantKind:   "telegram",
			wantMethod: "social",
			wantEntry:  "https://t.me/s/japaconnect",
			wantConfig: map[string]string{"provider": "telegram", "handle": "@japaconnect"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			report, err := NewProber(cannedClient(), 5).Preview(t.Context(), tc.url, "", nil)
			if err != nil {
				t.Fatalf("Preview(%s): %v", tc.url, err)
			}

			if !slices.Contains(report.DetectedKinds, tc.wantKind) {
				t.Errorf("detected_kinds missing %q: %v", tc.wantKind, report.DetectedKinds)
			}

			prop, ok := findChannel(report, tc.wantMethod)
			if !ok {
				t.Fatalf("channels missing %q proposal: %+v", tc.wantMethod, report.Channels)
			}

			if tc.wantEntry != "" && prop.EntryURL != tc.wantEntry {
				t.Errorf("entry_url = %q, want %q", prop.EntryURL, tc.wantEntry)
			}

			if tc.wantPipeline != "" && prop.Pipeline != tc.wantPipeline {
				t.Errorf("pipeline = %q, want %q", prop.Pipeline, tc.wantPipeline)
			}

			for k, v := range tc.wantConfig {
				if prop.ProviderConfig[k] != v {
					t.Errorf("provider_config[%s] = %v, want %q", k, prop.ProviderConfig[k], v)
				}
			}
		})
	}
}

// TestPreview_PlatformSurvivesCategoryIntersection verifies that
// platform-signature proposals (social/git) are ground truth: they bypass
// the category-method intersection just like channel_sync. A
// channel_profile URL with unrelated categories must not lose its only
// viable channel to the filter.
func TestPreview_PlatformSurvivesCategoryIntersection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		url        string
		categories []string
		wantMethod string
	}{
		{"reddit under visa", "https://www.reddit.com/r/nigeria", []string{"visa"}, "social"},
		{"telegram under job", "https://t.me/japaconnect", []string{"job"}, "social"},
		{"github under news", "https://github.com/jedafe/japa", []string{"news"}, "git"},
		{"youtube under job", "https://www.youtube.com/@JapaStories", []string{"job"}, MethodChannelSync},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			report, err := NewProber(cannedClient(), 5).Preview(t.Context(), tc.url, "", tc.categories)
			if err != nil {
				t.Fatalf("Preview(%s): %v", tc.url, err)
			}

			if _, ok := findChannel(report, tc.wantMethod); !ok {
				t.Errorf("%q proposal dropped under categories %v: %+v",
					tc.wantMethod, tc.categories, report.Channels)
			}
		})
	}
}

// TestPreview_PlatformOnHTTPError verifies host signatures still produce
// proposals when the entry fetch returns an HTTP error status.
func TestPreview_PlatformOnHTTPError(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{},
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})}

	report, err := NewProber(client, 5).Preview(t.Context(), "https://www.youtube.com/@Blocked", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if _, ok := findChannel(report, MethodChannelSync); !ok {
		t.Errorf("channels missing channel_sync proposal on 403: %+v", report.Channels)
	}

	if !hasWarning(report, "HTTP 403") {
		t.Errorf("warnings missing HTTP status note: %v", report.Warnings)
	}
}

// --- budget + input validation ----------------------------------------------

// TestPreview_RequestBudget verifies the shared request budget caps total
// outbound requests and records a warning when detector probes are skipped.
func TestPreview_RequestBudget(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(mustFixture(t, "docs_page.html"))

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	// Budget 2: entry GET + robots.txt consume it all — every detector
	// probe must be refused before it hits the wire.
	report, err := NewProber(srv.Client(), 2).Preview(t.Context(), srv.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if got := hits.Load(); got != 2 {
		t.Errorf("server saw %d requests, want exactly 2 (entry + robots)", got)
	}

	if !hasWarning(report, "budget exhausted") {
		t.Errorf("warnings missing budget note: %v", report.Warnings)
	}
}

// TestPreview_InvalidURL covers input validation: non-http(s) schemes are
// rejected before any request is made.
func TestPreview_InvalidURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr error
	}{
		{"empty", "", errpkg.ErrRequired},
		{"ftp scheme", "ftp://example.com/file", errpkg.ErrInvalid},
		{"file scheme", "file:///etc/passwd", errpkg.ErrInvalid},
		{"javascript", "javascript:alert(1)", errpkg.ErrInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewProber(cannedClient(), 5).Preview(t.Context(), tc.url, "", nil)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Preview(%q) err = %v, want %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

// TestPreview_NilClient fails fast rather than fetching through an
// unhardened transport.
func TestPreview_NilClient(t *testing.T) {
	t.Parallel()

	_, err := NewProber(nil, 5).Preview(t.Context(), "https://example.com", "", nil)
	if !errors.Is(err, errpkg.ErrRequired) {
		t.Errorf("err = %v, want ErrRequired", err)
	}
}

// TestPreview_NoScheme verifies scheme-less input is normalized to https.
func TestPreview_NoScheme(t *testing.T) {
	t.Parallel()

	report, err := NewProber(cannedClient(), 5).Preview(t.Context(), "example.com/path", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if !strings.HasPrefix(report.FinalURL, "https://example.com") {
		t.Errorf("final_url = %q, want https://example.com…", report.FinalURL)
	}
}

// --- ranking / intersection --------------------------------------------------

// TestPreview_DocAndEmailHints covers document links (pdf/tabular/ical) and
// the mailto + newsletter-form email hint.
func TestPreview_DocAndEmailHints(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "docs_page.html", nil)

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	for _, m := range []string{"pdf", "tabular", "ical", "email"} {
		if _, ok := findMethod(report, m); !ok {
			t.Errorf("methods missing %s: %+v", m, report.Methods)
		}
	}

	prop, ok := findChannel(report, "ical")
	if !ok {
		t.Fatalf("channels missing ical proposal: %+v", report.Channels)
	}

	if !strings.HasSuffix(prop.EntryURL, "/events/webinars.ics") {
		t.Errorf("ical entry_url = %q", prop.EntryURL)
	}

	email, ok := findChannel(report, "email")
	if !ok {
		t.Fatalf("channels missing email proposal: %+v", report.Channels)
	}

	if email.EntryURL != "mailto:visas@example.org" {
		t.Errorf("email entry_url = %q", email.EntryURL)
	}
}

// TestPreview_EntryIsFeed verifies a submitted feed URL detects directly.
func TestPreview_EntryIsFeed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed.xml" {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel/></rss>`))

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/feed.xml", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	prop, ok := findChannel(report, "rss")
	if !ok {
		t.Fatalf("channels missing rss proposal: %+v", report.Channels)
	}

	if prop.EntryURL != srv.URL+"/feed.xml" {
		t.Errorf("rss entry_url = %q", prop.EntryURL)
	}
}

// TestPreview_EntryIsAPI verifies a submitted JSON endpoint detects as an
// api channel, not as a feed.
func TestPreview_EntryIsAPI(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/rates" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"usd":1500,"gbp":1900}`))

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/api/v1/rates", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if _, ok := findChannel(report, "rss"); ok {
		t.Errorf("JSON endpoint misdetected as feed: %+v", report.Channels)
	}

	prop, ok := findChannel(report, "api")
	if !ok {
		t.Fatalf("channels missing api proposal: %+v", report.Channels)
	}

	if prop.EntryURL != srv.URL+"/api/v1/rates" {
		t.Errorf("api entry_url = %q", prop.EntryURL)
	}
}

// TestPreview_StaticFallback verifies a plain page with no signals still
// yields one low-confidence static proposal.
func TestPreview_StaticFallback(t *testing.T) {
	t.Parallel()

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>About</title></head><body>` +
			`<p>A static about page with enough visible text to clear the SPA ` +
			`floor — nothing here suggests a feed, sitemap, or documents.</p>` +
			`</body></html>`))
	}))
	t.Cleanup(plain.Close)

	report, err := NewProber(plain.Client(), 5).Preview(t.Context(), plain.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if len(report.Channels) != 1 {
		t.Fatalf("channels = %+v, want exactly the static fallback", report.Channels)
	}

	if report.Channels[0].CrawlMethod != "static" || report.Channels[0].Confidence != 0.3 {
		t.Errorf("unexpected fallback proposal: %+v", report.Channels[0])
	}
}

// TestPreview_CategoryIntersection verifies proposals are filtered by the
// resolved category methods: a feed on a visa-categorized source does not
// produce an rss channel (rss ∉ visa methods), and the surviving static
// fallback carries the resolved entity types and pipeline.
func TestPreview_CategoryIntersection(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "feed_jsonld.html", nil)

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", []string{"visa"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if _, ok := findChannel(report, "rss"); ok {
		t.Errorf("rss proposal survived visa-category intersection: %+v", report.Channels)
	}

	if _, ok := findChannel(report, "jsonld"); ok {
		t.Errorf("jsonld proposal survived visa-category intersection: %+v", report.Channels)
	}

	if len(report.Channels) != 1 || report.Channels[0].CrawlMethod != "static" {
		t.Fatalf("channels = %+v, want only static fallback", report.Channels)
	}

	if !slices.Equal(report.Channels[0].EntityTypes, []string{"visa"}) {
		t.Errorf("entity_types = %v, want [visa]", report.Channels[0].EntityTypes)
	}

	if report.Channels[0].Pipeline != string(sources.PipelineExtract) {
		t.Errorf("pipeline = %q, want extract", report.Channels[0].Pipeline)
	}

	if len(report.Channels[0].RelevanceTopics) == 0 {
		t.Error("relevance_topics not seeded from resolved map")
	}
}

// TestPreview_CategoryEntityTypes verifies a matching category seeds
// entity_types + relevance topics on the detected channel.
func TestPreview_CategoryEntityTypes(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "feed_jsonld.html", nil)

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", []string{"job"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	prop, ok := findChannel(report, "jsonld")
	if !ok {
		t.Fatalf("channels missing jsonld proposal: %+v", report.Channels)
	}

	if !slices.Equal(prop.EntityTypes, []string{"job"}) {
		t.Errorf("entity_types = %v, want [job]", prop.EntityTypes)
	}

	if prop.Pipeline != string(sources.PipelineExtract) {
		t.Errorf("pipeline = %q, want extract (job category)", prop.Pipeline)
	}

	if !slices.Contains(prop.RelevanceTopics, "hiring") {
		t.Errorf("relevance_topics missing job topics: %v", prop.RelevanceTopics)
	}
}

// TestPreview_ChannelProfileKindWarning verifies the advisory warning when a
// channel_profile submission matches no platform signature.
func TestPreview_ChannelProfileKindWarning(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "feed_jsonld.html", nil)

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "channel_profile", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if !hasWarning(report, "channel_profile") {
		t.Errorf("warnings missing channel_profile advisory: %v", report.Warnings)
	}
}
