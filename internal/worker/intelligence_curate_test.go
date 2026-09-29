package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeIntelCurator is a canned intelCuratorPort for handler tests — the
// worker package cannot import the real curator (import cycle), so tests
// stub the port exactly like production wiring does.
type fakeIntelCurator struct {
	recs     []IntelCuratedRecord
	err      error
	calls    int
	lastBody []byte
	lastCh   sources.Channel
}

func (f *fakeIntelCurator) Curate(_ context.Context, body []byte, _ string, ch sources.Channel) ([]IntelCuratedRecord, error) {
	f.calls++
	f.lastBody = body
	f.lastCh = ch

	return f.recs, f.err
}

// fakeSettingsReader satisfies the settingsReader interface with a static
// int map — enough for the intel.curate.* thresholds the curate handler reads.
type fakeSettingsReader struct {
	ints    map[string]int
	strings map[string]string
	bools   map[string]bool
}

func (f fakeSettingsReader) GetIntOr(_ context.Context, key string, fallback int) int {
	if v, ok := f.ints[key]; ok {
		return v
	}

	return fallback
}
func (fakeSettingsReader) GetFloatOr(_ context.Context, _ string, fallback float64) float64 {
	return fallback
}
func (f fakeSettingsReader) GetStringOr(_ context.Context, key, fallback string) string {
	if v, ok := f.strings[key]; ok {
		return v
	}

	return fallback
}
func (f fakeSettingsReader) GetBoolOr(_ context.Context, key string, fallback bool) bool {
	if v, ok := f.bools[key]; ok {
		return v
	}

	return fallback
}

// setIntelCurator registers a fake curator port and restores the previous
// one on cleanup.
func setIntelCurator(t *testing.T, c intelCuratorPort) {
	t.Helper()
	old := intelCurator
	intelCurator = c
	t.Cleanup(func() { intelCurator = old })
}

// marshalCuratedExtraction builds the marshalled extraction JSON a curator
// record carries: field → {value, confidence, evidence}.
func marshalCuratedExtraction(t *testing.T, fields map[string]any) []byte {
	t.Helper()

	ext := map[string]any{}
	for k, v := range fields {
		ext[k] = map[string]any{"value": v, "confidence": 1.0, "evidence": "fixture"}
	}

	b, err := json.Marshal(ext)
	if err != nil {
		t.Fatalf("marshal extraction: %v", err)
	}

	return b
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

// TestIntelMissingRequiredFields covers the hybrid-pipeline check: required
// schema fields absent/null/empty in the marshalled extraction report true;
// unregistered entity types and malformed JSON cannot be checked and report
// false (the record stands as curated).
func TestIntelMissingRequiredFields(t *testing.T) {
	t.Parallel()

	jobWithTitle := marshalCuratedExtraction(t, map[string]any{"title": "Nurse"})
	jobFull := marshalCuratedExtraction(t, map[string]any{"title": "Nurse", "country": []any{"GB"}})

	tests := []struct {
		name       string
		raw        []byte
		entityType string
		want       bool
	}{
		{"job missing required country", jobWithTitle, "job", true},
		{"job complete", jobFull, "job", false},
		{"article missing required title", marshalCuratedExtraction(t, map[string]any{"summary": "x"}), "article", true},
		{"unregistered type unchecked", []byte(`{}`), "bogus_type", false},
		{"malformed JSON unchecked", []byte(`{nope`), "job", false},
		{"empty-string required value", marshalCuratedExtraction(t, map[string]any{"title": "  ", "country": []any{"GB"}}), "job", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := intelMissingRequiredFields(tt.raw, tt.entityType); got != tt.want {
				t.Errorf("intelMissingRequiredFields(%s, %q) = %v, want %v", tt.raw, tt.entityType, got, tt.want)
			}
		})
	}
}

// TestIntelCurateAutoPublish covers the trust-gate routing predicate:
// trust_level <= intel.curate.auto_publish_trust_max with a spec default
// of 2 — applied even when the settings reader is nil (matching the
// relevance gate's nil→defaults convention) — while an admin-set 0
// disables the gate and a raised ceiling widens it.
func TestIntelCurateAutoPublish(t *testing.T) {
	// Not parallel: mutates the appSettings package var.
	ctx := context.Background()

	oldSettings := appSettings
	t.Cleanup(func() { appSettings = oldSettings })

	src := func(trust int16) gen.IntelSource { return gen.IntelSource{TrustLevel: trust} }

	tests := []struct {
		name     string
		settings settingsReader // nil → reader unwired
		trust    int16
		want     bool
	}{
		{"nil reader: trust=1 auto-publishes", nil, 1, true},
		{"nil reader: trust=2 auto-publishes", nil, 2, true},
		{"nil reader: trust=3 stays on review path", nil, 3, false},
		{"nil reader: trust=4 stays on review path", nil, 4, false},
		{"no override row: trust=2 auto-publishes", fakeSettingsReader{ints: map[string]int{}}, 2, true},
		{"no override row: trust=3 stays on review path", fakeSettingsReader{ints: map[string]int{}}, 3, false},
		{"override 0 disables gate for trust=1", fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 0}}, 1, false},
		{"override 1 keeps trust=1, drops trust=2", fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 1}}, 2, false},
		{"override 4 pulls trust=4 into direct publish", fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 4}}, 4, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appSettings = tt.settings
			defer func() { appSettings = oldSettings }()

			if got := intelCurateAutoPublish(ctx, src(tt.trust)); got != tt.want {
				t.Errorf("intelCurateAutoPublish(trust=%d) = %v, want %v", tt.trust, got, tt.want)
			}
		})
	}
}

// TestHandleIntelCurateChannel_NotWired verifies the handler degrades to a
// logged skip when the intel dependencies were never registered.
func TestHandleIntelCurateChannel_NotWired(t *testing.T) {
	oldStores, oldSrcStore := intelStores, intelSourcesStore
	intelStores, intelSourcesStore = nil, nil
	t.Cleanup(func() { intelStores, intelSourcesStore = oldStores, oldSrcStore })

	task := asynq.NewTask(TypeIntelCurateChannel, []byte(`{"version":"v1","source_id":"11111111-1111-4111-8111-111111111111","channel_id":"22222222-2222-4222-8222-222222222222","raw_doc_id":"33333333-3333-4333-8333-333333333333"}`))
	if err := HandleIntelCurateChannel(context.Background(), task); err != nil {
		t.Fatalf("unwired handler returned error: %v", err)
	}
}

// TestIntelNormalizeItemURL pins the canonicalization used for the hybrid
// fallback's claimed-item suppression: scheme/host case, fragments, and a
// trailing slash fold together; query params are preserved (they can carry
// identity); unparseable/empty input yields "" and can never match.
func TestIntelNormalizeItemURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"plain", "https://example.com/jobs/1", "https://example.com/jobs/1"},
		{"case folding", "HTTPS://Example.COM/Jobs/1", "https://example.com/Jobs/1"},
		{"fragment dropped", "https://example.com/jobs/1#details", "https://example.com/jobs/1"},
		{"trailing slash folded", "https://example.com/jobs/1/", "https://example.com/jobs/1"},
		{"query kept", "https://example.com/jobs?id=123&utm_source=x", "https://example.com/jobs?id=123&utm_source=x"},
		{"empty", "", ""},
		{"blank", "   ", ""},
		{"unparseable", "not a url", ""},
		{"scheme-relative", "//example.com/path", "//example.com/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := intelNormalizeItemURL(tt.raw); got != tt.want {
				t.Errorf("intelNormalizeItemURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}

	// The set builder normalizes members and drops unusable entries.
	set := intelNormalizeItemURLSet([]string{
		"HTTPS://Example.COM/a/", "not a url", "https://example.com/b",
	})
	if _, ok := set["https://example.com/a"]; !ok {
		t.Errorf("set missing normalized entry: %v", set)
	}
	if len(set) != 2 {
		t.Errorf("set = %v, want 2 usable entries", set)
	}
	if intelNormalizeItemURLSet(nil) != nil {
		t.Error("nil input must yield nil (skip check disabled)")
	}
}

// ---------------------------------------------------------------------------
// Live-DB tests (gated on JC_TEST_DATABASE_URL via wireIntelChannelDeps)
// ---------------------------------------------------------------------------

// TestHandleIntelDetectChanges_PipelineRouting covers the detect_changes →
// pipeline dispatch contract: extract-pipeline channels enqueue
// intel:extract, curate/hybrid channels enqueue intel:curate_channel, and a
// source with no channel falls back to extract (legacy behaviour).
func TestHandleIntelDetectChanges_PipelineRouting(t *testing.T) {
	cases := []struct {
		name       string
		pipeline   string // "" = create no channel at all
		wantTask   string
		wantAbsent string
	}{
		{"extract pipeline enqueues extract", "extract", TypeIntelExtract, TypeIntelCurateChannel},
		{"curate pipeline enqueues curate", "curate", TypeIntelCurateChannel, TypeIntelExtract},
		{"hybrid pipeline enqueues curate", "hybrid", TypeIntelCurateChannel, TypeIntelExtract},
		{"no channel falls back to extract", "", TypeIntelExtract, TypeIntelCurateChannel},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

			src := mkIntelTestSource(t, srcStore, "pipe-"+tc.pipeline+tc.name[:4], nil)

			var chID string
			var channelIDForDoc gen.IntelSourceChannel
			hasChannel := tc.pipeline != ""

			if hasChannel {
				pipeline := tc.pipeline
				ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
					a.Pipeline = pipeline
					if pipeline != "extract" {
						a.EntityTypes = []string{"news"}
					}
				})
				chID = ch.ID.String()
				channelIDForDoc = ch
			}

			rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
				SourceID:    src.ID,
				Url:         src.SourceUrl,
				HttpStatus:  200,
				HttpHeaders: []byte(`{}`),
				Body:        []byte("pipeline routing fixture"),
				ContentHash: "pipehash-" + tc.pipeline,
			})
			if err != nil {
				t.Fatalf("create raw doc: %v", err)
			}
			_ = channelIDForDoc

			payload, _ := json.Marshal(intelDetectChangesPayload{
				Version:   intelPayloadVersion,
				SourceID:  src.ID.String(),
				ChannelID: chID,
				RawDocID:  rawDoc.ID.String(),
				PriorHash: "", // first crawl → downstream enqueue
			})

			if err := HandleIntelDetectChanges(context.Background(), asynq.NewTask(TypeIntelDetectChanges, payload)); err != nil {
				t.Fatalf("HandleIntelDetectChanges: %v", err)
			}

			infos := pendingTasks(t, inspector, tc.wantTask)
			if len(infos) == 0 {
				t.Fatalf("no %s task enqueued", tc.wantTask)
			}
			if n := len(pendingTasks(t, inspector, tc.wantAbsent)); n != 0 {
				t.Errorf("%d %s tasks enqueued, want 0", n, tc.wantAbsent)
			}

			if tc.wantTask == TypeIntelCurateChannel {
				var cp intelCurateChannelPayload
				if err := json.Unmarshal(infos[0].Payload, &cp); err != nil {
					t.Fatalf("curate payload: %v", err)
				}
				if cp.ChannelID != chID || cp.RawDocID != rawDoc.ID.String() || cp.SourceID != src.ID.String() {
					t.Errorf("curate payload = %+v", cp)
				}
			} else {
				var ep intelExtractPayload
				if err := json.Unmarshal(infos[0].Payload, &ep); err != nil {
					t.Fatalf("extract payload: %v", err)
				}
				if ep.RawDocID != rawDoc.ID.String() {
					t.Errorf("extract raw_doc_id = %q, want %q", ep.RawDocID, rawDoc.ID.String())
				}
				// Legacy payload (no channel_id) resolves the default
				// channel and stamps it on the extract enqueue.
				if !hasChannel && ep.ChannelID != "" {
					t.Errorf("extract channel_id = %q, want empty (no channel)", ep.ChannelID)
				}
			}
		})
	}
}

// TestHandleIntelCurateChannel_LiveDB exercises the happy path: a fake
// curator returns two records; both land in intel_records as 'extracted'
// with curator-origin evidence and confidence 1.0, and each enqueues
// intel:dedup (trust 3 exceeds the spec-default auto-publish ceiling of 2).
func TestHandleIntelCurateChannel_LiveDB(t *testing.T) {
	database, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	// Settings wired with no override row — the trust gate resolves its
	// spec default (2), which trust_level=3 does not clear.
	oldSettings := appSettings
	appSettings = fakeSettingsReader{ints: map[string]int{}}
	t.Cleanup(func() { appSettings = oldSettings })

	src := mkIntelTestSource(t, srcStore, "curate-src", func(a *gen.CreateIntelSourceParams) {
		a.TrustLevel = 3 // aggregator — above the default auto-publish ceiling (2)
	})
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "rss"
		a.Pipeline = "curate"
		a.EntityTypes = []string{"news"}
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		Body:        []byte("<rss/>fixture"),
		ContentHash: "curatehash1",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	fake := &fakeIntelCurator{recs: []IntelCuratedRecord{
		{
			RawExtraction:     marshalCuratedExtraction(t, map[string]any{"title": "Item one", "article_url": "https://news.example.com/1"}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "https://news.example.com/1",
			EntityType:        "news",
		},
		{
			RawExtraction:     marshalCuratedExtraction(t, map[string]any{"title": "Item two", "article_url": "https://news.example.com/2"}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "https://news.example.com/2",
			EntityType:        "news",
		},
	}}
	setIntelCurator(t, fake)

	payload, _ := json.Marshal(intelCurateChannelPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
	})

	if err := HandleIntelCurateChannel(context.Background(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel: %v", err)
	}

	if fake.calls != 1 {
		t.Fatalf("curator calls = %d, want 1", fake.calls)
	}
	if string(fake.lastBody) != "<rss/>fixture" {
		t.Errorf("curator received body %q", fake.lastBody)
	}

	// Both records persisted as 'extracted' with curator evidence.
	rows, err := database.Writer.Query(context.Background(),
		`SELECT entity_type, external_key, status, confidence_overall, evidence::text
		 FROM intel_records WHERE raw_document_id = $1 ORDER BY external_key`, rawDoc.ID)
	if err != nil {
		t.Fatalf("query records: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var et, key, status, evidence string
		var conf float32
		if err := rows.Scan(&et, &key, &status, &conf, &evidence); err != nil {
			t.Fatalf("scan record: %v", err)
		}
		if et != "news" || status != "extracted" || conf != 1.0 {
			t.Errorf("record %s: entity_type=%q status=%q confidence=%v", key, et, status, conf)
		}
		if !json.Valid([]byte(evidence)) || !jsonContains([]byte(evidence), `"origin":"curator"`) {
			t.Errorf("record %s evidence missing curator origin: %s", key, evidence)
		}
		got = append(got, key)
	}
	if len(got) != 2 {
		t.Fatalf("persisted records = %v, want 2", got)
	}

	// Each record enqueued intel:dedup carrying record_id + channel_id.
	infos := pendingTasks(t, inspector, TypeIntelDedup)
	if len(infos) != 2 {
		t.Fatalf("dedup tasks = %d, want 2", len(infos))
	}
	for _, info := range infos {
		var rp intelRecordPayload
		if err := json.Unmarshal(info.Payload, &rp); err != nil {
			t.Fatalf("dedup payload: %v", err)
		}
		if rp.ChannelID != ch.ID.String() || rp.RecordID == "" {
			t.Errorf("dedup payload = %+v", rp)
		}
	}
	if n := len(pendingTasks(t, inspector, TypeIntelPromote)); n != 0 {
		t.Errorf("promote tasks = %d, want 0 (trust 3 exceeds default ceiling 2)", n)
	}
}

// jsonContains reports whether a JSON object document contains the given
// "key":"value" pair.
func jsonContains(doc []byte, pair string) bool {
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		return false
	}
	var kv map[string]string
	if err := json.Unmarshal([]byte("{"+pair+"}"), &kv); err != nil {
		return false
	}
	for k, v := range kv {
		if mv, ok := m[k].(string); !ok || mv != v {
			return false
		}
	}

	return true
}

// TestHandleIntelCurateChannel_NoCuratorFallsBack covers the extract
// fallback: a crawl method with no curator (ErrIntelNoCurator from the
// port) routes the raw doc to intel:extract instead of failing.
func TestHandleIntelCurateChannel_NoCuratorFallsBack(t *testing.T) {
	_, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "nocurator-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "api" // fetchable method with no registered curator
		a.Pipeline = "curate"
		a.EntityTypes = []string{"news"}
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		Body:        []byte(`{"items": []}`),
		ContentHash: "nocuratorhash",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	setIntelCurator(t, &fakeIntelCurator{err: fmt.Errorf("%w: %q", ErrIntelNoCurator, "api")})

	payload, _ := json.Marshal(intelCurateChannelPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
	})

	if err := HandleIntelCurateChannel(context.Background(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel: %v", err)
	}

	infos := pendingTasks(t, inspector, TypeIntelExtract)
	if len(infos) == 0 {
		t.Fatal("no intel:extract fallback task enqueued")
	}
	var ep intelExtractPayload
	if err := json.Unmarshal(infos[0].Payload, &ep); err != nil {
		t.Fatalf("extract payload: %v", err)
	}
	if ep.ChannelID != ch.ID.String() {
		t.Errorf("extract channel_id = %q, want %q", ep.ChannelID, ch.ID.String())
	}
}

// TestHandleIntelCurateChannel_SitemapManifestSkipsExtract covers the
// sitemap carve-out of the no-curator fallback: a sitemap channel's raw doc
// is a url<TAB>lastmod manifest (change-detection signal only — per-URL
// child fetches are Phase 2), so ErrIntelNoCurator must NOT route it to
// intel:extract where an LLM pass could only fabricate records from slugs.
func TestHandleIntelCurateChannel_SitemapManifestSkipsExtract(t *testing.T) {
	_, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "sitemap-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "sitemap"
		a.Pipeline = "curate"
		a.EntityTypes = []string{"news"}
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		ContentType: strPtr(fetcher.ManifestContentType),
		Body:        []byte("https://example.com/a\t2025-01-01\n"),
		ContentHash: "sitemaphash",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	setIntelCurator(t, &fakeIntelCurator{err: fmt.Errorf("%w: %q", ErrIntelNoCurator, "sitemap")})

	payload, _ := json.Marshal(intelCurateChannelPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
	})

	if err := HandleIntelCurateChannel(context.Background(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel: %v", err)
	}

	if n := len(pendingTasks(t, inspector, TypeIntelExtract)); n != 0 {
		t.Errorf("sitemap manifest enqueued %d intel:extract tasks, want 0", n)
	}
}

// TestHandleIntelCurateChannel_DerivedDocDispatchesEffectiveMethod covers
// the manifest-child re-dispatch: a doc fetched as a sitemap child was
// retrieved via the static adapter, so the curator port must be invoked
// with crawl_method 'static' — not the channel's 'sitemap', which has no
// curator and would otherwise dead-hop into the extract fallback.
func TestHandleIntelCurateChannel_DerivedDocDispatchesEffectiveMethod(t *testing.T) {
	_, srcStore, intelDocStore, _ := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "derived-curate-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "sitemap"
		a.Pipeline = "curate"
		a.EntityTypes = []string{"job"}
	})

	parent, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		ContentType: strPtr(fetcher.ManifestContentType),
		Body:        []byte("https://example.com/jobs/1\t2025-01-01\n"),
		ContentHash: "derived-parent-hash",
	})
	if err != nil {
		t.Fatalf("create parent doc: %v", err)
	}

	derived, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:         src.ID,
		ChannelID:        ch.ID,
		Url:              "https://example.com/jobs/1",
		HttpStatus:       200,
		HttpHeaders:      []byte(`{}`),
		ContentType:      strPtr("text/html"),
		Body:             []byte(`<html><script type="application/ld+json">{"@type":"JobPosting"}</script></html>`),
		ContentHash:      "derived-child-hash",
		ParentDocumentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("create derived doc: %v", err)
	}

	fake := &fakeIntelCurator{recs: []IntelCuratedRecord{
		{
			RawExtraction:     marshalCuratedExtraction(t, map[string]any{"title": "Baker", "article_url": "https://example.com/jobs/1"}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "https://example.com/jobs/1",
			EntityType:        "job",
		},
	}}
	setIntelCurator(t, fake)

	payload, _ := json.Marshal(intelCurateChannelPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  derived.ID.String(),
	})

	if err := HandleIntelCurateChannel(context.Background(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel: %v", err)
	}

	if fake.calls != 1 {
		t.Fatalf("curator calls = %d, want 1", fake.calls)
	}
	if fake.lastCh.CrawlMethod != "static" {
		t.Errorf("curator dispatched on crawl_method %q, want %q (effective fetch method)", fake.lastCh.CrawlMethod, "static")
	}
	// entry_url must be the fetched child page, not the channel's manifest
	// URL — the curator's page-level key falls back to entry_url.
	if fake.lastCh.EntryUrl != "https://example.com/jobs/1" {
		t.Errorf("curator dispatched with entry_url %q, want the derived doc URL", fake.lastCh.EntryUrl)
	}
	if fake.lastCh.ID != ch.ID {
		t.Errorf("curator dispatched with channel id %q, want %q (clone must keep identity)", fake.lastCh.ID, ch.ID)
	}
}

// TestHandleIntelDetectChanges_DerivedDocKeepsChannelPipeline covers the
// detect_changes side: a derived child of a sitemap channel now follows
// the channel's declared pipeline — 'curate' enqueues intel:curate_channel
// (the handler re-dispatches on the effective fetch method) instead of
// being force-routed to intel:extract.
func TestHandleIntelDetectChanges_DerivedDocKeepsChannelPipeline(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pipeline   string
		wantTask   string
		wantAbsent string
	}{
		{"curate pipeline curates derived doc", "curate", TypeIntelCurateChannel, TypeIntelExtract},
		{"extract pipeline still extracts derived doc", "extract", TypeIntelExtract, TypeIntelCurateChannel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

			src := mkIntelTestSource(t, srcStore, "derived-dc-"+tc.pipeline, nil)
			ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
				a.CrawlMethod = "sitemap"
				a.Pipeline = tc.pipeline
				a.EntityTypes = []string{"job"}
			})

			parent, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
				SourceID:    src.ID,
				ChannelID:   ch.ID,
				Url:         ch.EntryUrl,
				HttpStatus:  200,
				HttpHeaders: []byte(`{}`),
				ContentType: strPtr(fetcher.ManifestContentType),
				Body:        []byte("https://example.com/jobs/1\t2025-01-01\n"),
				ContentHash: "parent-hash-" + tc.pipeline,
			})
			if err != nil {
				t.Fatalf("create parent doc: %v", err)
			}

			derived, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
				SourceID:         src.ID,
				ChannelID:        ch.ID,
				Url:              "https://example.com/jobs/1",
				HttpStatus:       200,
				HttpHeaders:      []byte(`{}`),
				ContentType:      strPtr("text/html"),
				Body:             []byte("<html><body>job page</body></html>"),
				ContentHash:      "derived-hash-" + tc.pipeline,
				ParentDocumentID: parent.ID,
			})
			if err != nil {
				t.Fatalf("create derived doc: %v", err)
			}

			payload, _ := json.Marshal(intelDetectChangesPayload{
				Version:   intelPayloadVersion,
				SourceID:  src.ID.String(),
				ChannelID: ch.ID.String(),
				RawDocID:  derived.ID.String(),
				PriorHash: "", // first capture of this URL → downstream fires
			})

			if err := HandleIntelDetectChanges(context.Background(), asynq.NewTask(TypeIntelDetectChanges, payload)); err != nil {
				t.Fatalf("HandleIntelDetectChanges: %v", err)
			}

			if n := len(pendingTasks(t, inspector, tc.wantTask)); n == 0 {
				t.Fatalf("no %s task enqueued for derived doc", tc.wantTask)
			}
			if n := len(pendingTasks(t, inspector, tc.wantAbsent)); n != 0 {
				t.Errorf("%d %s tasks enqueued, want 0", n, tc.wantAbsent)
			}
		})
	}
}

// TestHandleIntelDetectChanges_SitemapStopsChain covers the dispatch-side
// half of the same rule: a changed sitemap manifest records the crawl
// outcome (volatility tuner + last_changed_at) but enqueues neither
// intel:curate_channel nor intel:extract.
func TestHandleIntelDetectChanges_SitemapStopsChain(t *testing.T) {
	_, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "sitemap-dc", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "sitemap"
		a.Pipeline = "curate"
		a.EntityTypes = []string{"news"}
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		ContentType: strPtr(fetcher.ManifestContentType),
		Body:        []byte("https://example.com/a\t2025-01-01\n"),
		ContentHash: "sitemaphash2",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	payload, _ := json.Marshal(intelDetectChangesPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
		PriorHash: "", // first crawl → downstream would fire without the guard
	})

	if err := HandleIntelDetectChanges(context.Background(), asynq.NewTask(TypeIntelDetectChanges, payload)); err != nil {
		t.Fatalf("HandleIntelDetectChanges: %v", err)
	}

	if n := len(pendingTasks(t, inspector, TypeIntelCurateChannel)); n != 0 {
		t.Errorf("sitemap manifest enqueued %d intel:curate_channel tasks, want 0", n)
	}
	if n := len(pendingTasks(t, inspector, TypeIntelExtract)); n != 0 {
		t.Errorf("sitemap manifest enqueued %d intel:extract tasks, want 0", n)
	}
}

// TestHandleIntelCurateChannel_TrustAutoPublish covers the trust gate: a
// source whose trust_level clears intel.curate.auto_publish_trust_max has
// its curated records marked auto_published and routed straight to
// intel:promote, skipping dedup/publish_gate.
func TestHandleIntelCurateChannel_TrustAutoPublish(t *testing.T) {
	database, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	oldSettings := appSettings
	appSettings = fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 2}}
	t.Cleanup(func() { appSettings = oldSettings })

	src := mkIntelTestSource(t, srcStore, "trusted-src", func(a *gen.CreateIntelSourceParams) {
		a.TrustLevel = 2
	})
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "rss"
		a.Pipeline = "curate"
		a.EntityTypes = []string{"news"}
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		Body:        []byte("<rss/>trusted"),
		ContentHash: "trustedhash",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	setIntelCurator(t, &fakeIntelCurator{recs: []IntelCuratedRecord{{
		RawExtraction:     marshalCuratedExtraction(t, map[string]any{"title": "Trusted item"}),
		ConfidenceOverall: 1.0,
		ExternalKey:       "https://news.example.com/trusted-1",
		EntityType:        "news",
	}}})

	payload, _ := json.Marshal(intelCurateChannelPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
	})

	if err := HandleIntelCurateChannel(context.Background(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel: %v", err)
	}

	var status string
	if err := database.Writer.QueryRow(context.Background(),
		`SELECT status FROM intel_records WHERE raw_document_id = $1`, rawDoc.ID).Scan(&status); err != nil {
		t.Fatalf("load record status: %v", err)
	}
	if status != "auto_published" {
		t.Errorf("record status = %q, want auto_published", status)
	}

	// The promote enqueue carries everything the promoter needs: the
	// persisted record id (loaded by HandleIntelPromote → GetIntelRecord)
	// plus the channel stamp.
	infos := pendingTasks(t, inspector, TypeIntelPromote)
	if len(infos) != 1 {
		t.Fatalf("promote tasks = %d, want 1", len(infos))
	}
	var rp intelRecordPayload
	if err := json.Unmarshal(infos[0].Payload, &rp); err != nil {
		t.Fatalf("promote payload: %v", err)
	}
	var recordID string
	if err := database.Writer.QueryRow(context.Background(),
		`SELECT id::text FROM intel_records WHERE raw_document_id = $1`, rawDoc.ID).Scan(&recordID); err != nil {
		t.Fatalf("load record id: %v", err)
	}
	if rp.RecordID != recordID {
		t.Errorf("promote payload record_id = %q, want %q", rp.RecordID, recordID)
	}
	if rp.ChannelID != ch.ID.String() {
		t.Errorf("promote payload channel_id = %q, want %q", rp.ChannelID, ch.ID.String())
	}
	if n := len(pendingTasks(t, inspector, TypeIntelDedup)); n != 0 {
		t.Errorf("dedup tasks = %d, want 0 (auto-publish skips dedup)", n)
	}
}

// TestHandleIntelCurateChannel_TrustGateRouting is the Task 11 routing
// contract end to end: with the spec-default trust ceiling (2 — no
// override row) a trust=1 source's curated records land 'auto_published'
// with intel:promote enqueued; trust=4 stays on the dedup → publish_gate
// review path; an admin override of 0 disables the gate entirely; and a
// raised ceiling pulls higher-trust-level sources into direct publish.
func TestHandleIntelCurateChannel_TrustGateRouting(t *testing.T) {
	cases := []struct {
		name        string
		trustLevel  int16
		trustMax    int  // setting value when hasOverride
		hasOverride bool // false → no settings row → spec default (2)
		wantStatus  string
		wantTask    string
		wantAbsent  string
	}{
		{"default ceiling publishes trust=1", 1, 0, false, "auto_published", TypeIntelPromote, TypeIntelDedup},
		{"default ceiling routes trust=4 to dedup", 4, 0, false, "extracted", TypeIntelDedup, TypeIntelPromote},
		{"override 0 disables the gate", 1, 0, true, "extracted", TypeIntelDedup, TypeIntelPromote},
		{"override 4 publishes trust=4", 4, 4, true, "auto_published", TypeIntelPromote, TypeIntelDedup},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

			ints := map[string]int{}
			if tc.hasOverride {
				ints["intel.curate.auto_publish_trust_max"] = tc.trustMax
			}
			oldSettings := appSettings
			appSettings = fakeSettingsReader{ints: ints}
			t.Cleanup(func() { appSettings = oldSettings })

			src := mkIntelTestSource(t, srcStore, fmt.Sprintf("trust-gate-%d", i), func(a *gen.CreateIntelSourceParams) {
				a.TrustLevel = tc.trustLevel
			})
			ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
				a.CrawlMethod = "rss"
				a.Pipeline = "curate"
				a.EntityTypes = []string{"news"}
			})

			rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
				SourceID:    src.ID,
				ChannelID:   ch.ID,
				Url:         ch.EntryUrl,
				HttpStatus:  200,
				HttpHeaders: []byte(`{}`),
				Body:        []byte("<rss/>trust-gate"),
				ContentHash: fmt.Sprintf("trustgatehash-%d", i),
			})
			if err != nil {
				t.Fatalf("create raw doc: %v", err)
			}

			setIntelCurator(t, &fakeIntelCurator{recs: []IntelCuratedRecord{{
				RawExtraction:     marshalCuratedExtraction(t, map[string]any{"title": "Gate item", "article_url": "https://news.example.com/gate"}),
				ConfidenceOverall: 1.0,
				ExternalKey:       fmt.Sprintf("https://news.example.com/gate-%d", i),
				EntityType:        "news",
			}}})

			payload, _ := json.Marshal(intelCurateChannelPayload{
				Version:   intelPayloadVersion,
				SourceID:  src.ID.String(),
				ChannelID: ch.ID.String(),
				RawDocID:  rawDoc.ID.String(),
			})

			if err := HandleIntelCurateChannel(context.Background(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
				t.Fatalf("HandleIntelCurateChannel: %v", err)
			}

			var status, recordID string
			if err := database.Writer.QueryRow(context.Background(),
				`SELECT status, id::text FROM intel_records WHERE raw_document_id = $1`, rawDoc.ID).Scan(&status, &recordID); err != nil {
				t.Fatalf("load record: %v", err)
			}
			if status != tc.wantStatus {
				t.Errorf("record status = %q, want %q", status, tc.wantStatus)
			}

			infos := pendingTasks(t, inspector, tc.wantTask)
			if len(infos) != 1 {
				t.Fatalf("%s tasks = %d, want 1", tc.wantTask, len(infos))
			}
			var rp intelRecordPayload
			if err := json.Unmarshal(infos[0].Payload, &rp); err != nil {
				t.Fatalf("%s payload: %v", tc.wantTask, err)
			}
			if rp.RecordID != recordID || rp.ChannelID != ch.ID.String() {
				t.Errorf("%s payload = %+v, want record_id=%q channel_id=%q", tc.wantTask, rp, recordID, ch.ID.String())
			}
			if n := len(pendingTasks(t, inspector, tc.wantAbsent)); n != 0 {
				t.Errorf("%s tasks = %d, want 0", tc.wantAbsent, n)
			}
		})
	}
}

// TestHandleIntelCurateChannel_Hybrid covers the hybrid pipeline split: a
// curated record missing required schema fields on an extractable entity
// type routes the doc to intel:extract once, while a curate-only entity
// type (which can never be LLM-extracted) persists with review_required.
func TestHandleIntelCurateChannel_Hybrid(t *testing.T) {
	database, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	// Pin the trust gate off so hybrid routing is under test in isolation —
	// the default source fixture is trust_level=1, which would otherwise
	// auto-publish under the spec-default ceiling (2).
	oldSettings := appSettings
	appSettings = fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 0}}
	t.Cleanup(func() { appSettings = oldSettings })

	src := mkIntelTestSource(t, srcStore, "hybrid-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "jsonld"
		a.Pipeline = "hybrid"
		a.EntityTypes = []string{"job", "article"}
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		Body:        []byte("{}"),
		ContentHash: "hybridhash",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	setIntelCurator(t, &fakeIntelCurator{recs: []IntelCuratedRecord{
		{
			// job requires title + country — missing country → extract fallback.
			RawExtraction:     marshalCuratedExtraction(t, map[string]any{"title": "Incomplete job"}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "job:incomplete-1",
			EntityType:        "job",
		},
		{
			// article requires title — missing → curate-only, review_required.
			RawExtraction:     marshalCuratedExtraction(t, map[string]any{"summary": "no title"}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "article:no-title-1",
			EntityType:        "article",
		},
		{
			// complete job record → normal dedup path.
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":   "Complete job",
				"country": []any{"GB"},
			}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "job:complete-1",
			EntityType:        "job",
		},
	}})

	payload, _ := json.Marshal(intelCurateChannelPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
	})

	if err := HandleIntelCurateChannel(context.Background(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel: %v", err)
	}

	// The incomplete job routed the doc to extract exactly once.
	if n := len(pendingTasks(t, inspector, TypeIntelExtract)); n != 1 {
		t.Errorf("extract tasks = %d, want 1", n)
	}

	// The curate-only article record persisted with review_required; the
	// complete job record persisted with extracted + dedup.
	rows, err := database.Writer.Query(context.Background(),
		`SELECT entity_type, status FROM intel_records WHERE raw_document_id = $1 ORDER BY external_key`, rawDoc.ID)
	if err != nil {
		t.Fatalf("query records: %v", err)
	}
	defer rows.Close()

	statuses := map[string]string{}
	for rows.Next() {
		var et, st string
		if err := rows.Scan(&et, &st); err != nil {
			t.Fatalf("scan: %v", err)
		}
		statuses[et] = st
	}
	if statuses["article"] != "review_required" {
		t.Errorf("article record status = %q, want review_required", statuses["article"])
	}
	if statuses["job"] != "extracted" {
		t.Errorf("job record status = %q, want extracted", statuses["job"])
	}
	if len(statuses) != 2 {
		t.Errorf("persisted entity types = %v, want exactly article+job", statuses)
	}

	// Only the complete record continues to dedup.
	if n := len(pendingTasks(t, inspector, TypeIntelDedup)); n != 1 {
		t.Errorf("dedup tasks = %d, want 1", n)
	}
}

// TestHandleIntelCurateChannel_HybridExtractSkipSet covers the
// cross-path duplicate guard: when a hybrid doc is routed to extract
// because one record is missing required fields, the extract task payload
// carries the canonical item URLs the curate pass already persisted —
// and the extract handler suppresses a re-derived result matching one of
// those URLs, even though its extractor-style external_key never collides
// with the curator key. Without the suppression the well-formed item
// would land in intel_records twice (once curated, once extracted).
func TestHandleIntelCurateChannel_HybridExtractSkipSet(t *testing.T) {
	database, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	// Isolate from the trust gate (source fixture is trust=1) and the
	// relevance gate — the skip-set contract is under test alone.
	oldSettings := appSettings
	appSettings = fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 0}}
	t.Cleanup(func() { appSettings = oldSettings })
	setRelevanceGate(t, nil)

	src := mkIntelTestSource(t, srcStore, "hybrid-skip-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "jsonld"
		a.Pipeline = "hybrid"
		a.EntityTypes = []string{"job"}
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		Body:        []byte("{}"),
		ContentHash: "hybridskiphash",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	setIntelCurator(t, &fakeIntelCurator{recs: []IntelCuratedRecord{
		{
			// Missing required 'country' → routes the doc to extract; the
			// record itself is NOT persisted or claimed.
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":           "Incomplete job",
				"application_url": "https://jobs.example.com/incomplete",
			}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "job:incomplete-1",
			EntityType:        "job",
		},
		{
			// Complete → persisted under the curator key and claimed.
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":           "Complete job",
				"country":         []any{"GB"},
				"application_url": "https://jobs.example.com/complete",
			}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "job:complete-1",
			EntityType:        "job",
		},
	}})

	curatePayload, _ := json.Marshal(intelCurateChannelPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
	})
	if err := HandleIntelCurateChannel(context.Background(), asynq.NewTask(TypeIntelCurateChannel, curatePayload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel: %v", err)
	}

	// The extract fallback payload must claim the persisted item's
	// canonical URL — and NOT the deficient item's (extract owns it).
	extractInfos := pendingTasks(t, inspector, TypeIntelExtract)
	if len(extractInfos) != 1 {
		t.Fatalf("extract tasks = %d, want 1", len(extractInfos))
	}
	var ep intelExtractPayload
	if err := json.Unmarshal(extractInfos[0].Payload, &ep); err != nil {
		t.Fatalf("extract payload: %v", err)
	}
	if !slices.Contains(ep.SkipItemURLs, "https://jobs.example.com/complete") {
		t.Errorf("skip_item_urls = %v, want the claimed item URL", ep.SkipItemURLs)
	}
	if slices.Contains(ep.SkipItemURLs, "https://jobs.example.com/incomplete") {
		t.Errorf("skip_item_urls must not contain the extract-routed item: %v", ep.SkipItemURLs)
	}

	// Phase 2 — run the enqueued extract task against a fake extractor
	// that re-derives the claimed item under an extractor-style key (plus
	// a trailing-slash variant to prove normalization) and recovers the
	// deficient item under a different key.
	ext := &fakeIntelExtractor{results: []IntelExtractResult{
		{
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":           "Complete job",
				"country":         []any{"GB"},
				"application_url": "https://jobs.example.com/complete/", // trailing-slash variant
			}),
			ConfidenceOverall: 0.9,
			ExternalKey:       "https://example.com/hybrid-skip-src|dup-1",
		},
		{
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":           "Incomplete job",
				"country":         []any{"GB"},
				"application_url": "https://jobs.example.com/incomplete",
			}),
			ConfidenceOverall: 0.9,
			ExternalKey:       "https://example.com/hybrid-skip-src|recovered-1",
		},
	}}
	oldExt := intelExtractor
	intelExtractor = ext
	t.Cleanup(func() { intelExtractor = oldExt })

	if err := HandleIntelExtract(context.Background(), asynq.NewTask(TypeIntelExtract, extractInfos[0].Payload)); err != nil {
		t.Fatalf("HandleIntelExtract: %v", err)
	}

	// Exactly two records exist: the curated 'complete' row and the
	// extracted 'recovered' row — the claimed item was NOT re-persisted
	// under the extractor key.
	var n int
	if err := database.Writer.QueryRow(context.Background(),
		`SELECT count(*) FROM intel_records WHERE raw_document_id = $1`, rawDoc.ID).Scan(&n); err != nil {
		t.Fatalf("count records: %v", err)
	}
	if n != 2 {
		t.Errorf("records = %d, want 2 (claimed item suppressed)", n)
	}

	var dup int
	if err := database.Writer.QueryRow(context.Background(),
		`SELECT count(*) FROM intel_records WHERE raw_document_id = $1 AND external_key = $2`,
		rawDoc.ID, "https://example.com/hybrid-skip-src|dup-1").Scan(&dup); err != nil {
		t.Fatalf("count dup: %v", err)
	}
	if dup != 0 {
		t.Error("claimed item double-persisted under the extractor key")
	}

	// One dedup per persisted record: the curated complete item + the
	// recovered item — nothing for the suppressed duplicate.
	if n := len(pendingTasks(t, inspector, TypeIntelDedup)); n != 2 {
		t.Errorf("dedup tasks = %d, want 2", n)
	}
}
