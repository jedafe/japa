package fetcher

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// testComposite builds a Composite whose adapters all use the fixture's
// HTTP client (bypassing the SSRF-safe transport so httptest's loopback
// listener is reachable), mirroring how rss_test.go injects srv.Client().
func testComposite(client *http.Client) *Composite {
	return &Composite{
		static: &StaticFetcher{client: client},
		rss:    &RSSFetcher{client: client},
		api:    &APIFetcher{client: client},
	}
}

func channelFixture(t *testing.T, handler http.Handler) (*httptest.Server, *Composite) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv, testComposite(srv.Client())
}

// TestCompositeFetch_ChannelEntryURL verifies the channel, not the source,
// drives the request: the composite must hit ch.EntryUrl even when
// src.SourceUrl points elsewhere.
func TestCompositeFetch_ChannelEntryURL(t *testing.T) {
	t.Parallel()

	var gotPath string
	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))

	src := sources.Source{SourceUrl: "https://source.example/ignored"}
	ch := sources.Channel{EntryUrl: srv.URL + "/channel/path", CrawlMethod: "static"}

	if _, err := c.Fetch(t.Context(), src, ch); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if gotPath != "/channel/path" {
		t.Errorf("fetched path = %q, want /channel/path (channel entry_url)", gotPath)
	}
}

// TestCompositeFetch_MethodDispatch covers the Phase-1 dispatch contract:
// pull methods route to an adapter, inbound/manual methods hard-error, and
// not-yet-implemented methods return ErrMethodNotImplemented.
func TestCompositeFetch_MethodDispatch(t *testing.T) {
	t.Parallel()

	src := sources.Source{SourceUrl: "https://example.com"}

	tests := []struct {
		method    string
		wantErr   bool
		sentinel  error
		errSubstr string
	}{
		{"manual", true, nil, "not fetchable"},
		{"email", true, nil, "not fetchable"},
		{"webhook", true, nil, "not fetchable"},
		{"render", true, ErrMethodNotImplemented, ""},
		{"git", true, ErrMethodNotImplemented, ""},
		{"social", true, ErrMethodNotImplemented, ""},
		{"search", true, ErrMethodNotImplemented, ""},
		{"wayback", true, ErrMethodNotImplemented, ""},
		{"xhr_sniff", true, ErrMethodNotImplemented, ""},
		{"oneshot", true, ErrMethodNotImplemented, ""},
		{"bogus_method", true, ErrMethodNotImplemented, ""},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			t.Parallel()

			ch := sources.Channel{EntryUrl: "https://example.com", CrawlMethod: tt.method}
			_, err := testComposite(http.DefaultClient).Fetch(t.Context(), src, ch)
			if err == nil {
				t.Fatalf("method %q: expected error, got nil", tt.method)
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Errorf("method %q: got %v, want %v", tt.method, err, tt.sentinel)
			}
			if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("method %q: error %q missing %q", tt.method, err, tt.errSubstr)
			}
		})
	}
}

// TestCompositeFetch_JSONLD verifies the thin jsonld adapter: page bytes are
// fetched via the static path, then the body is replaced with the raw
// ld+json blocks and the hash covers only the structured data.
func TestCompositeFetch_JSONLD(t *testing.T) {
	t.Parallel()

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head>
			<script type="application/ld+json">{"@type":"JobPosting","title":"Care Worker"}</script>
			<script type="application/ld+json">{"@type":"JobPosting","title":"Nurse"}</script>
		</head><body><p>page chrome that must not reach the body</p></body></html>`))
	}))

	src := sources.Source{}
	ch := sources.Channel{EntryUrl: srv.URL + "/jobs", CrawlMethod: "jsonld"}

	res, err := c.Fetch(t.Context(), src, ch)
	if err != nil {
		t.Fatalf("Fetch jsonld: %v", err)
	}

	body := string(res.Body)
	if !strings.Contains(body, "Care Worker") || !strings.Contains(body, "Nurse") {
		t.Errorf("jsonld body missing blocks: %q", body)
	}
	if strings.Contains(body, "page chrome") {
		t.Errorf("jsonld body should contain only ld+json blocks, got: %q", body)
	}
	if res.ContentType != "application/ld+json" {
		t.Errorf("content type = %q, want application/ld+json", res.ContentType)
	}
	if res.ContentHash == "" {
		t.Error("expected non-empty content hash")
	}
}

// TestCompositeFetch_JSONLDNoBlocks: a page with no ld+json is a fetch
// failure (surfaced to the channel's consecutive_failures bookkeeping), not
// an empty document.
func TestCompositeFetch_JSONLDNoBlocks(t *testing.T) {
	t.Parallel()

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>plain page</body></html>"))
	}))

	ch := sources.Channel{EntryUrl: srv.URL, CrawlMethod: "jsonld"}
	if _, err := c.Fetch(t.Context(), sources.Source{}, ch); err == nil {
		t.Fatal("expected error for page without ld+json blocks")
	}
}

// TestCompositeFetch_Sitemap verifies the urlset → URL+lastmod manifest
// transform.
func TestCompositeFetch_Sitemap(t *testing.T) {
	t.Parallel()

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
			<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
				<url><loc>https://example.com/jobs</loc><lastmod>2026-09-01</lastmod></url>
				<url><loc>https://example.com/visa</loc></url>
			</urlset>`))
	}))

	ch := sources.Channel{EntryUrl: srv.URL + "/sitemap.xml", CrawlMethod: "sitemap"}

	res, err := c.Fetch(t.Context(), sources.Source{}, ch)
	if err != nil {
		t.Fatalf("Fetch sitemap: %v", err)
	}

	want := "https://example.com/jobs\t2026-09-01\nhttps://example.com/visa\t\n"
	if string(res.Body) != want {
		t.Errorf("manifest = %q, want %q (missing lastmod yields an empty field)", res.Body, want)
	}
}

// TestCompositeFetch_SitemapIndex: a sitemapindex flattens member sitemap
// URLs into the same manifest shape.
func TestCompositeFetch_SitemapIndex(t *testing.T) {
	t.Parallel()

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
			<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
				<sitemap><loc>https://example.com/sitemap-jobs.xml</loc><lastmod>2026-09-02</lastmod></sitemap>
			</sitemapindex>`))
	}))

	ch := sources.Channel{EntryUrl: srv.URL, CrawlMethod: "sitemap"}
	res, err := c.Fetch(t.Context(), sources.Source{}, ch)
	if err != nil {
		t.Fatalf("Fetch sitemap index: %v", err)
	}

	if !strings.Contains(string(res.Body), "https://example.com/sitemap-jobs.xml\t2026-09-02") {
		t.Errorf("manifest = %q", res.Body)
	}
}

// TestCompositeFetch_SitemapEmpty: an empty/non-XML document is a fetch
// failure, not a silently empty manifest.
func TestCompositeFetch_SitemapEmpty(t *testing.T) {
	t.Parallel()

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>not a sitemap</body></html>"))
	}))

	ch := sources.Channel{EntryUrl: srv.URL, CrawlMethod: "sitemap"}
	if _, err := c.Fetch(t.Context(), sources.Source{}, ch); err == nil {
		t.Fatal("expected error for non-sitemap body")
	}
}

// TestCompositeFetch_BytePassthroughMethods: ical and tabular channels fetch
// raw bytes through the static path — parsing is the curator's job (Task 9).
func TestCompositeFetch_BytePassthroughMethods(t *testing.T) {
	t.Parallel()

	payload := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:Test\r\nEND:VEVENT\r\nEND:VCALENDAR"

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(payload))
	}))

	for _, method := range []string{"ical", "tabular"} {
		ch := sources.Channel{EntryUrl: srv.URL, CrawlMethod: method}
		res, err := c.Fetch(t.Context(), sources.Source{}, ch)
		if err != nil {
			t.Fatalf("Fetch %s: %v", method, err)
		}
		if string(res.Body) != payload {
			t.Errorf("%s body = %q, want raw bytes", method, res.Body)
		}
	}
}

// TestResolveChannelProviderConfig covers the channel-first provider_config
// precedence: a channel with a valid API config uses it; a channel with an
// empty/'{}' config falls back to the parent source's.
func TestResolveChannelProviderConfig(t *testing.T) {
	t.Parallel()

	srcCfg := []byte(`{"endpoint":"https://api.source.example/v1"}`)
	chCfg := []byte(`{"endpoint":"https://api.channel.example/v2"}`)

	tests := []struct {
		name     string
		ch, src  []byte
		wantJSON string
	}{
		{"channel config wins", chCfg, srcCfg, "api.channel.example"},
		{"empty channel falls back to source", nil, srcCfg, "api.source.example"},
		{"{} channel falls back to source", []byte(`{}`), srcCfg, "api.source.example"},
		{"probe-stamped channel falls back to source", []byte(`{"probe_confidence":0.9}`), srcCfg, "api.source.example"},
		{"both empty yields source raw", nil, nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveChannelProviderConfig(
				sources.Channel{ProviderConfig: tt.ch},
				sources.Source{ProviderConfig: tt.src},
			)

			if tt.wantJSON == "" {
				if len(got) != 0 {
					t.Errorf("got %q, want empty", got)
				}
				return
			}

			if !strings.Contains(string(got), tt.wantJSON) {
				t.Errorf("got %q, want config containing %q", got, tt.wantJSON)
			}
		})
	}
}

// TestPendingFetchMethod pins the Phase-2 stub set the probe_verify worker
// filters out of channel proposals — the same methods Composite.Fetch
// answers ErrMethodNotImplemented for. Drift between the two would let
// un-fetchable channels materialize (or over-filter implemented ones).
func TestPendingFetchMethod(t *testing.T) {
	t.Parallel()

	pending := []string{"render", "git", "social", "search", "wayback", "xhr_sniff", "oneshot"}
	for _, m := range pending {
		if !PendingFetchMethod(m) {
			t.Errorf("PendingFetchMethod(%q) = false, want true", m)
		}

		// The dispatch must agree — a pending method answers
		// ErrMethodNotImplemented from Fetch.
		ch := sources.Channel{CrawlMethod: m, EntryUrl: "https://example.com"}
		_, err := testComposite(nil).Fetch(t.Context(), sources.Source{}, ch)
		if !errors.Is(err, ErrMethodNotImplemented) {
			t.Errorf("Fetch(%q) err = %v, want ErrMethodNotImplemented", m, err)
		}
	}

	implemented := []string{"static", "rss", "api", "pdf", "ocr", "ical", "tabular", "jsonld", "sitemap", "manual", "email", "webhook"}
	for _, m := range implemented {
		if PendingFetchMethod(m) {
			t.Errorf("PendingFetchMethod(%q) = true, want false", m)
		}
	}
}
