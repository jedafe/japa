package probe

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// longHTML returns a minimal server-rendered page with enough visible text
// to stay under the SPA detector's radar, plus an optional head block.
func longHTML(head string) string {
	return `<!DOCTYPE html><html><head>` + head + `</head><body><main>
	<p>Nigerians abroad rely on this site for verified guidance on jobs,
	visas, scholarships, and relocation planning. Every page is reviewed by
	an editorial team before publication so readers can act with confidence
	and without fear of misinformation or recruitment scams.</p>
	</main></body></html>`
}

const jobPostingLD = `<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Care Worker — UK Visa Sponsorship"}
</script>`

// TestVerify_SitemapWalkAndCategoryPaths covers the verify deep pass: a
// site that hides everything behind its sitemap and category paths must
// yield a jsonld channel for the /jobs listing and a feed channel for
// /feed — none of which the shallow preview discovers.
func TestVerify_SitemapWalkAndCategoryPaths(t *testing.T) {
	t.Parallel()

	var srvURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML("")))
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\nSitemap: " + srvURL + "/sitemap.xml\n"))
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + srvURL + `/jobs</loc></url>
  <url><loc>` + srvURL + `/about</loc></url>
</urlset>`))
	})
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML(jobPostingLD)))
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML("")))
	})
	mux.HandleFunc("/feed", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Jobs</title></channel></rss>`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	// categories job+news: job lists jsonld, news lists rss — both channel
	// families survive the category-method intersection.
	report, err := NewProber(srv.Client(), DefaultVerifyMaxRequests).
		Verify(t.Context(), srv.URL+"/", "publisher", []string{"job", "news"}, VerifyOpts{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	for _, m := range []string{"rss", "jsonld", "sitemap"} {
		if _, ok := findMethod(report, m); !ok {
			t.Errorf("methods missing %s: %+v", m, report.Methods)
		}
	}

	// The sitemap/category-path-fetched /jobs page must produce a typed
	// channel pointing at /jobs — the verify pass's core deliverable —
	// not inflate the entry page's jsonld proposal.
	var jobsProp *ChannelProposal

	for i := range report.Channels {
		if report.Channels[i].EntryURL == srvURL+"/jobs" {
			jobsProp = &report.Channels[i]
		}
	}

	if jobsProp == nil {
		t.Fatalf("channels missing /jobs proposal from sitemap walk: %+v", report.Channels)
	}

	if jobsProp.CrawlMethod != "jsonld" {
		t.Errorf("/jobs proposal method = %q, want jsonld", jobsProp.CrawlMethod)
	}

	if !slices.Contains(jobsProp.EntityTypes, "job") {
		t.Errorf("/jobs proposal entity_types = %v, want job", jobsProp.EntityTypes)
	}

	// /feed was re-checked by GET and verified live.
	if !slices.Contains(report.FeedURLs, srvURL+"/feed") {
		t.Errorf("feed_urls missing %s/feed: %v", srvURL, report.FeedURLs)
	}

	feed, ok := findChannel(report, "rss")
	if !ok || feed.EntryURL != srvURL+"/feed" {
		t.Errorf("rss channel = %+v, want entry_url %s/feed", feed, srvURL)
	}
}

// TestVerify_RechecksDeadFeed verifies the feed re-check: a <link
// rel="alternate"> target that 404s loses both its feed_urls entry and its
// rss channel proposal, with a warning recorded.
func TestVerify_RechecksDeadFeed(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(longHTML(
				`<link rel="alternate" type="application/rss+xml" href="/feed-dead">`)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	report, err := NewProber(srv.Client(), DefaultVerifyMaxRequests).
		Verify(t.Context(), srv.URL+"/", "", nil, VerifyOpts{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if slices.Contains(report.FeedURLs, srv.URL+"/feed-dead") {
		t.Errorf("dead feed survived re-check: %v", report.FeedURLs)
	}

	if _, ok := findChannel(report, "rss"); ok {
		t.Errorf("dead feed kept an rss channel proposal: %+v", report.Channels)
	}

	if !hasWarning(report, "did not re-check") {
		t.Errorf("warnings missing dead-feed note: %v", report.Warnings)
	}
}

// TestVerify_RecheckTransportErrorKeepsFeed verifies that a transport
// failure during the feed re-check (connection refused, DNS blip,
// timeout) is NOT treated as a dead feed — only a definitive non-2xx or
// non-feed response is. The feed survives in feed_urls and keeps its rss
// channel proposal, with a warning noting the failed re-check.
func TestVerify_RecheckTransportErrorKeepsFeed(t *testing.T) {
	t.Parallel()

	// Stand up a real feed server so discovery is honest, then kill it so
	// the verify re-check hits a connection-refused transport error.
	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel/></rss>`))
	}))
	feedURL := feedSrv.URL + "/feed"
	feedSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(longHTML(
				`<link rel="alternate" type="application/rss+xml" href="` + feedURL + `">`)))

			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	report, err := NewProber(srv.Client(), DefaultVerifyMaxRequests).
		Verify(t.Context(), srv.URL+"/", "", nil, VerifyOpts{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !slices.Contains(report.FeedURLs, feedURL) {
		t.Errorf("transport-failed feed dropped from feed_urls: %v", report.FeedURLs)
	}

	feed, ok := findChannel(report, "rss")
	if !ok || feed.EntryURL != feedURL {
		t.Errorf("transport-failed feed lost its rss channel: %+v", report.Channels)
	}

	if !hasWarning(report, "re-check failed") {
		t.Errorf("warnings missing failed re-check note: %v", report.Warnings)
	}
}

// TestVerify_SubPageJSONLDEntityTypes verifies a sub-page JSON-LD
// detection stamps its own entity types on the channel proposal — with
// no categories and no entry-page ld+json, a /jobs JobPosting must still
// yield entity_types=[job] so the extract/curate materialize gate does
// not silently drop the channel.
func TestVerify_SubPageJSONLDEntityTypes(t *testing.T) {
	t.Parallel()

	var srvURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML(""))) // no ld+json on the entry page
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\nSitemap: " + srvURL + "/sitemap.xml\n"))
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>` + srvURL + `/jobs</loc></url></urlset>`))
	})
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML(jobPostingLD)))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	report, err := NewProber(srv.Client(), DefaultVerifyMaxRequests).
		Verify(t.Context(), srv.URL+"/", "", nil, VerifyOpts{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	var jobsProp *ChannelProposal

	for i := range report.Channels {
		if report.Channels[i].EntryURL == srvURL+"/jobs" {
			jobsProp = &report.Channels[i]
		}
	}

	if jobsProp == nil {
		t.Fatalf("channels missing /jobs proposal: %+v", report.Channels)
	}

	if !slices.Contains(jobsProp.EntityTypes, "job") {
		t.Errorf("/jobs proposal entity_types = %v, want [job] from sub-page ld+json", jobsProp.EntityTypes)
	}

	// The entry page must NOT absorb the sub-page's types.
	if slices.Contains(report.JSONLDTypes, "JobPosting") {
		t.Errorf("entry-page jsonld_types absorbed sub-page type: %v", report.JSONLDTypes)
	}
}

// TestVerify_EntryErrorSkipsDeepPass verifies an HTTP-error entry page
// runs only the host-level detectors — no path probes or sitemap fetches.
func TestVerify_EntryErrorSkipsDeepPass(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	report, err := NewProber(srv.Client(), DefaultVerifyMaxRequests).
		Verify(t.Context(), srv.URL+"/", "", nil, VerifyOpts{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if len(report.Channels) != 0 {
		t.Errorf("channels = %+v, want none on entry error", report.Channels)
	}

	// Entry GET + robots.txt only — the deep pass must not fire.
	if got := hits.Load(); got > 2 {
		t.Errorf("server saw %d requests, want <=2 (entry + robots)", got)
	}
}

// TestVerify_SitemapIndex verifies a sitemapindex expands one level of
// child manifests during the walk.
func TestVerify_SitemapIndex(t *testing.T) {
	t.Parallel()

	var srvURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML("")))
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>` + srvURL + `/sitemap-jobs.xml</loc></sitemap>
</sitemapindex>`))
	})
	mux.HandleFunc("/sitemap-jobs.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + srvURL + `/jobs/1</loc></url>
</urlset>`))
	})
	mux.HandleFunc("/jobs/1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML(jobPostingLD)))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	report, err := NewProber(srv.Client(), DefaultVerifyMaxRequests).
		Verify(t.Context(), srv.URL+"/", "", []string{"job"}, VerifyOpts{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	found := false

	for _, ch := range report.Channels {
		if ch.EntryURL == srvURL+"/jobs/1" && ch.CrawlMethod == "jsonld" {
			found = true
		}
	}

	if !found {
		t.Errorf("channels missing sitemapindex-expanded /jobs/1 jsonld proposal: %+v", report.Channels)
	}
}

// TestVerify_SkipsPrivateIPLiteralDiscoveredURLs covers the SSRF guard on
// content-discovered URLs (final review C1d): a sitemap <loc> or feed link
// pointing at a private/reserved IP literal on a FOREIGN host must be
// skipped without spending a request — the report records warnings instead.
// Same-host literals (like this fixture's own 127.0.0.1 locs) stay
// fetchable; names are left to the fetcher dial guard.
func TestVerify_SkipsPrivateIPLiteralDiscoveredURLs(t *testing.T) {
	t.Parallel()

	var srvURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML(
			`<link rel="alternate" type="application/rss+xml" href="http://169.254.169.254/feed">`)))
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + srvURL + `/jobs</loc></url>
  <url><loc>http://169.254.169.254/latest/meta-data</loc></url>
  <url><loc>http://10.0.0.5/internal/secret</loc></url>
  <url><loc>http://[::1]:8500/v1/status</loc></url>
</urlset>`))
	})
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(longHTML(jobPostingLD)))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	report, err := NewProber(srv.Client(), DefaultVerifyMaxRequests).
		Verify(t.Context(), srv.URL+"/", "", []string{"job"}, VerifyOpts{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// The same-host loc was fetched and produced its channel.
	var jobsProp *ChannelProposal
	for i := range report.Channels {
		if report.Channels[i].EntryURL == srvURL+"/jobs" {
			jobsProp = &report.Channels[i]
		}
	}
	if jobsProp == nil {
		t.Fatalf("channels missing same-host /jobs proposal: %+v", report.Channels)
	}

	// No channel may carry a private-literal entry URL.
	for _, ch := range report.Channels {
		if strings.Contains(ch.EntryURL, "169.254.169.254") ||
			strings.Contains(ch.EntryURL, "10.0.0.5") ||
			strings.Contains(ch.EntryURL, "[::1]") {
			t.Errorf("private-literal channel proposal leaked through: %+v", ch)
		}
	}

	// The private feed was dropped from feed_urls and its rss proposal.
	for _, f := range report.FeedURLs {
		if strings.Contains(f, "169.254.169.254") {
			t.Errorf("private feed survived recheck: %v", report.FeedURLs)
		}
	}
	for _, ch := range report.Channels {
		if ch.CrawlMethod == "rss" {
			t.Errorf("private feed kept an rss channel proposal: %+v", ch)
		}
	}

	// The skips are surfaced as warnings, not silent.
	if !hasWarning(report, "private/reserved") {
		t.Errorf("warnings missing SSRF skip note: %v", report.Warnings)
	}
}

// TestVerify_BudgetCoversDeepPass verifies the verify budget exceeds the
// preview budget — a prober configured with the preview's
// DefaultMaxRequests still spends past it during the deep pass.
func TestVerify_BudgetCoversDeepPass(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(longHTML("")))

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	// A preview-budgeted prober (5) still gets the verify floor (25):
	// detectors + always/category path probes all land.
	report, err := NewProber(srv.Client(), DefaultMaxRequests).
		Verify(t.Context(), srv.URL+"/", "", []string{"job"}, VerifyOpts{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if got := hits.Load(); got <= int64(DefaultMaxRequests) {
		t.Errorf("server saw %d requests, want > preview budget %d (verify floor applies)", got, DefaultMaxRequests)
	}

	// No feed/sitemap existed — the run still ends with the static fallback
	// proposal rather than an empty channel set.
	if _, ok := findChannel(report, "static"); !ok {
		t.Errorf("channels missing static fallback: %+v", report.Channels)
	}
}
