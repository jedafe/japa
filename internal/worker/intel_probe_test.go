package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/probe"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// --- pure helpers -------------------------------------------------------

// TestVerifySkipSource covers the verify-exempt source rules: §106
// video_channel rows and registry-only manual rows skip cleanly, while
// proposed 'auto' sources (crawl_method='manual' by design) still verify.
func TestVerifySkipSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  gen.IntelSource
		want bool
	}{
		{"video_channel", gen.IntelSource{SourceType: "video_channel", CrawlMethod: "manual"}, true},
		{"manual registry row", gen.IntelSource{SourceType: "static", CrawlMethod: "manual"}, true},
		{"proposed auto source", gen.IntelSource{SourceType: "auto", CrawlMethod: "manual"}, false},
		{"rss source", gen.IntelSource{SourceType: "auto", CrawlMethod: "rss"}, false},
		{"comparison api", gen.IntelSource{SourceType: "comparison_api", CrawlMethod: "api"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := verifySkipSource(tt.src); got != tt.want {
				t.Errorf("verifySkipSource(%+v) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

// TestProposalInputs covers the probe→sources adapter: fields carry over
// and provider_config maps marshal to JSON.
func TestProposalInputs(t *testing.T) {
	t.Parallel()

	got := proposalInputs([]probe.ChannelProposal{
		{
			Label:           "News feed",
			EntryURL:        "https://example.com/feed",
			CrawlMethod:     "rss",
			EntityTypes:     []string{"news"},
			Pipeline:        "curate",
			Confidence:      0.95,
			RelevanceTopics: []string{"migration"},
			ProviderConfig:  map[string]any{"hub": "https://hub.example"},
		},
		{Label: "Bare", EntryURL: "https://example.com", CrawlMethod: "static"},
	})

	if len(got) != 2 {
		t.Fatalf("inputs = %d, want 2", len(got))
	}

	in := got[0]
	if in.Label != "News feed" || in.CrawlMethod != "rss" || in.Confidence != 0.95 {
		t.Errorf("input fields = %+v", in)
	}
	if !slices.Equal(in.EntityTypes, []string{"news"}) || !slices.Equal(in.RelevanceTopics, []string{"migration"}) {
		t.Errorf("slices not carried: %+v", in)
	}

	var pc map[string]any
	if err := json.Unmarshal(in.ProviderConfig, &pc); err != nil || pc["hub"] != "https://hub.example" {
		t.Errorf("provider_config = %s, want marshalled hub", in.ProviderConfig)
	}

	if len(got[1].ProviderConfig) != 0 {
		t.Errorf("empty provider_config marshalled to %s", got[1].ProviderConfig)
	}
}

// TestDetectedMethodNames checks the report.Methods → detected_methods
// flattening.
func TestDetectedMethodNames(t *testing.T) {
	t.Parallel()

	got := detectedMethodNames([]probe.DetectedMethod{
		{Method: "rss", Confidence: 0.95},
		{Method: "jsonld", Confidence: 0.9},
	})

	if !slices.Equal(got, []string{"rss", "jsonld"}) {
		t.Errorf("methods = %v", got)
	}
}

// --- handler ------------------------------------------------------------

// TestHandleIntelProbeVerify_NotWired verifies the handler degrades to a
// logged skip when dependencies were never registered.
func TestHandleIntelProbeVerify_NotWired(t *testing.T) {
	oldProber, oldStore := intelProber, intelSourcesStore
	intelProber, intelSourcesStore = nil, nil
	t.Cleanup(func() { intelProber, intelSourcesStore = oldProber, oldStore })

	task := asynq.NewTask(TypeIntelProbeVerify, []byte(`{"version":"v1","source_id":"11111111-1111-4111-8111-111111111111"}`))
	if err := HandleIntelProbeVerify(context.Background(), task); err != nil {
		t.Fatalf("unwired handler returned error: %v", err)
	}
}

// verifyFixture serves a site: HTML entry with a feed link, live /feed,
// robots.txt, and a sitemap listing a /jobs page with JobPosting ld+json.
func verifyFixture(t *testing.T) *httptest.Server {
	t.Helper()

	var srvURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head>
			<link rel="alternate" type="application/rss+xml" href="/feed">
			</head><body><p>` + "Verified relocation guidance for Nigerians abroad, reviewed by our editorial team before every publication cycle." + `</p></body></html>`))
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
		_, _ = w.Write([]byte(`<html><head><script type="application/ld+json">
			{"@context":"https://schema.org","@type":"JobPosting","title":"Care Worker"}
			</script></head><body><p>` + "Visa-sponsored care roles verified weekly by the editorial desk for applicants planning relocation." + `</p></body></html>`))
	})
	mux.HandleFunc("/feed", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>News</title></channel></rss>`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	return srv
}

// TestHandleIntelProbeVerify_LiveDB exercises the happy path end to end:
// the handler verifies the fixture site, writes probe_report +
// detected_methods on the source row, and materializes channels — and a
// second run is idempotent. Gated on JC_TEST_DATABASE_URL.
func TestHandleIntelProbeVerify_LiveDB(t *testing.T) {
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping live Postgres test")
	}

	ctx := context.Background()

	database, err := db.New(ctx, dsn)
	if err != nil {
		t.Skipf("test database unreachable: %v", err)
	}
	// t.Cleanup so the pool outlives the cleanup-registered row deletes
	// (registered later → run first); a plain defer closes the pool before
	// any t.Cleanup fires.
	t.Cleanup(func() { database.Close() })

	srv := verifyFixture(t)

	srcStore := sources.NewStore(database)

	src, err := srcStore.Create(ctx, gen.CreateIntelSourceParams{
		Name:               "Verify Test Source",
		Domain:             "example.com",
		SourceUrl:          srv.URL + "/",
		Category:           "news",
		Categories:         []string{"news", "job"},
		SourceType:         "auto",
		SourceKind:         "publisher",
		TrustLevel:         3,
		CrawlMethod:        "manual",
		CrawlFrequency:     "manual",
		CrawlFrequencyMins: 0,
		RobotsPolicy:       []byte("{}"),
		ProviderConfig:     []byte("{}"),
		Enabled:            true,
		Status:             "active",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	defer func() { _ = srcStore.Delete(ctx, src.ID) }()

	oldProber, oldStore := intelProber, intelSourcesStore
	intelProber = probe.NewProber(srv.Client(), probe.DefaultVerifyMaxRequests)
	intelSourcesStore = srcStore
	t.Cleanup(func() { intelProber, intelSourcesStore = oldProber, oldStore })

	payload, _ := json.Marshal(intelSourcePayload{Version: intelPayloadVersion, SourceID: src.ID.String()})
	task := asynq.NewTask(TypeIntelProbeVerify, payload)

	if err := HandleIntelProbeVerify(ctx, task); err != nil {
		t.Fatalf("HandleIntelProbeVerify: %v", err)
	}

	got, err := srcStore.Get(ctx, src.ID)
	if err != nil {
		t.Fatalf("reload source: %v", err)
	}
	if len(got.DetectedMethods) == 0 {
		t.Error("detected_methods empty after verify")
	}
	if !got.LastProbedAt.Valid {
		t.Error("last_probed_at not stamped")
	}
	if len(got.ProbeReport) == 0 || string(got.ProbeReport) == "{}" {
		t.Error("probe_report not written")
	}

	channels, err := srcStore.ListChannels(ctx, src.ID)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) == 0 {
		t.Fatal("verify materialized no channels")
	}
	for _, ch := range channels {
		if ch.Origin != "probe" {
			t.Errorf("channel %s origin = %q, want probe", ch.Label, ch.Origin)
		}
	}

	var feedFound bool
	for _, ch := range channels {
		if ch.CrawlMethod == "rss" {
			feedFound = true
		}
	}
	if !feedFound {
		t.Error("no rss channel materialized")
	}

	// Idempotent re-run: all proposals dedupe against the UNIQUE
	// constraint — channel count is unchanged.
	if err := HandleIntelProbeVerify(ctx, task); err != nil {
		t.Fatalf("HandleIntelProbeVerify re-run: %v", err)
	}
	after, err := srcStore.ListChannels(ctx, src.ID)
	if err != nil {
		t.Fatalf("list channels after re-run: %v", err)
	}
	if len(after) != len(channels) {
		t.Errorf("re-run changed channel count %d → %d, want idempotent", len(channels), len(after))
	}
}

// TestHandleIntelProbeVerify_SkipPaths covers the live-DB skip rules:
// disabled and video_channel sources return nil without probing.
func TestHandleIntelProbeVerify_SkipPaths(t *testing.T) {
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping live Postgres test")
	}

	ctx := context.Background()

	database, err := db.New(ctx, dsn)
	if err != nil {
		t.Skipf("test database unreachable: %v", err)
	}
	// t.Cleanup so the pool outlives the cleanup-registered row deletes
	// inside mk (registered later → run first).
	t.Cleanup(func() { database.Close() })

	srcStore := sources.NewStore(database)

	mk := func(name string, mutate func(*gen.CreateIntelSourceParams)) gen.IntelSource {
		arg := gen.CreateIntelSourceParams{
			Name: name, Domain: "example.com", SourceUrl: "https://example.com/" + name,
			Category: "news", SourceType: "auto", SourceKind: "publisher",
			TrustLevel: 3, CrawlMethod: "manual", CrawlFrequency: "manual",
			RobotsPolicy: []byte("{}"), ProviderConfig: []byte("{}"),
			Enabled: true, Status: "active",
		}
		mutate(&arg)

		src, err := srcStore.Create(ctx, arg)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		t.Cleanup(func() { _ = srcStore.Delete(ctx, src.ID) })

		return src
	}

	disabled := mk("disabled", func(a *gen.CreateIntelSourceParams) { a.Enabled = false })
	proposed := mk("proposed", func(a *gen.CreateIntelSourceParams) { a.Status = "proposed"; a.Enabled = false })
	video := mk("video", func(a *gen.CreateIntelSourceParams) { a.SourceType = "video_channel" })

	oldProber, oldStore := intelProber, intelSourcesStore
	intelProber = probe.NewProber(http.DefaultClient, probe.DefaultVerifyMaxRequests)
	intelSourcesStore = srcStore
	t.Cleanup(func() { intelProber, intelSourcesStore = oldProber, oldStore })

	for _, src := range []gen.IntelSource{disabled, proposed, video} {
		payload, _ := json.Marshal(intelSourcePayload{Version: intelPayloadVersion, SourceID: src.ID.String()})
		if err := HandleIntelProbeVerify(ctx, asynq.NewTask(TypeIntelProbeVerify, payload)); err != nil {
			t.Errorf("skip path %s returned error: %v", src.Name, err)
		}

		channels, err := srcStore.ListChannels(ctx, src.ID)
		if err != nil {
			t.Fatalf("list channels %s: %v", src.Name, err)
		}
		if len(channels) != 0 {
			t.Errorf("skip path %s materialized %d channels", src.Name, len(channels))
		}
	}

	// Missing source: a well-formed payload for a UUID that does not exist
	// must skip cleanly, not error.
	var missing pgtype.UUID
	if err := missing.Scan("99999999-9999-4999-8999-999999999999"); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(intelSourcePayload{Version: intelPayloadVersion, SourceID: missing.String()})
	if err := HandleIntelProbeVerify(ctx, asynq.NewTask(TypeIntelProbeVerify, payload)); err != nil {
		t.Errorf("missing source returned error: %v", err)
	}
}
