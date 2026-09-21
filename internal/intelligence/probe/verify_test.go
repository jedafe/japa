package probe

import (
	"net/http"
	"net/http/httptest"
	"slices"
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
