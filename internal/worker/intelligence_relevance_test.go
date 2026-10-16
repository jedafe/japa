package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/relevance"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// relevanceTestSettings is a full settingsReader fake — the curate test's
// fakeSettingsReader only maps ints, while the relevance gate also reads
// float thresholds, the classifier toggle, and the category map JSON.
type relevanceTestSettings struct {
	ints    map[string]int
	floats  map[string]float64
	strings map[string]string
	bools   map[string]bool
}

func (f relevanceTestSettings) GetIntOr(_ context.Context, key string, fallback int) int {
	if v, ok := f.ints[key]; ok {
		return v
	}

	return fallback
}

func (f relevanceTestSettings) GetFloatOr(_ context.Context, key string, fallback float64) float64 {
	if v, ok := f.floats[key]; ok {
		return v
	}

	return fallback
}

func (f relevanceTestSettings) GetStringOr(_ context.Context, key, fallback string) string {
	if v, ok := f.strings[key]; ok {
		return v
	}

	return fallback
}

func (f relevanceTestSettings) GetBoolOr(_ context.Context, key string, fallback bool) bool {
	if v, ok := f.bools[key]; ok {
		return v
	}

	return fallback
}

// fakeRelevanceClassifier satisfies relevance.Classifier for gate wiring
// tests.
type fakeRelevanceClassifier struct {
	calls   int
	onTopic bool
	err     error
}

func (f *fakeRelevanceClassifier) Classify(_ context.Context, _ relevance.ItemInput, _ []string) (bool, error) {
	f.calls++

	return f.onTopic, f.err
}

// fakeIntelExtractor satisfies intelExtractorPort for extract-path tests.
type fakeIntelExtractor struct {
	results []IntelExtractResult
	err     error
	calls   int
}

func (f *fakeIntelExtractor) Extract(_ context.Context, _, _, _, _ string) ([]IntelExtractResult, error) {
	f.calls++

	return f.results, f.err
}

// setRelevanceGate installs a gate func (or nil) and restores the prior
// one on cleanup.
func setRelevanceGate(t *testing.T, fn func(ctx context.Context, item relevance.ItemInput, ch sources.Channel, mapTopics []string) (relevance.Verdict, error)) {
	t.Helper()

	old := intelRelevanceGate
	intelRelevanceGate = fn

	t.Cleanup(func() { intelRelevanceGate = old })
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

// TestIntelItemFromExtraction covers the extraction → ItemInput field
// mapping: preferred field names, fallbacks, non-string values skipped,
// malformed JSON → empty item.
func TestIntelItemFromExtraction(t *testing.T) {
	t.Parallel()

	enc := func(v any) []byte {
		b, _ := json.Marshal(v)
		return b
	}

	tests := []struct {
		name               string
		raw                []byte
		wantTitle, wantSum string
		wantURL            string
	}{
		{
			name: "primary fields",
			raw: enc(map[string]any{
				"title":       map[string]any{"value": "Visa bulletin"},
				"summary":     map[string]any{"value": "New rules"},
				"article_url": map[string]any{"value": "https://x.example/1"},
			}),
			wantTitle: "Visa bulletin", wantSum: "New rules", wantURL: "https://x.example/1",
		},
		{
			name: "fallback names",
			raw: enc(map[string]any{
				"name":        map[string]any{"value": "Grant fair"},
				"description": map[string]any{"value": "desc"},
				"url":         map[string]any{"value": "https://x.example/2"},
			}),
			wantTitle: "Grant fair", wantSum: "desc", wantURL: "https://x.example/2",
		},
		{
			name: "empty and non-string values skipped",
			raw: enc(map[string]any{
				"title":   map[string]any{"value": "  "},
				"summary": map[string]any{"value": 42},
				"name":    map[string]any{"value": "Fallback name"},
			}),
			wantTitle: "Fallback name",
		},
		{name: "malformed JSON", raw: []byte(`{nope`), wantTitle: ""},
		{name: "empty object", raw: []byte(`{}`), wantTitle: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intelItemFromExtraction(tt.raw)
			if got.Title != tt.wantTitle || got.Summary != tt.wantSum || got.URL != tt.wantURL {
				t.Errorf("intelItemFromExtraction = %+v, want title=%q summary=%q url=%q", got, tt.wantTitle, tt.wantSum, tt.wantURL)
			}
		})
	}
}

// TestIntelRelevanceMapTopics covers category-map topic resolution:
// custom mode ignores the map (nil without a settings read), filtered
// resolves the source's categories, legacy scalar category falls back,
// and the live system_settings JSON wins over the embedded default.
func TestIntelRelevanceMapTopics(t *testing.T) {
	ctx := t.Context()

	oldSettings := appSettings

	t.Cleanup(func() { appSettings = oldSettings })

	appSettings = relevanceTestSettings{}

	t.Run("custom mode returns nil", func(t *testing.T) {
		ch := sources.Channel{RelevanceMode: string(sources.RelevanceCustom)}
		src := gen.IntelSource{Category: "visa"}

		if got := intelRelevanceMapTopics(ctx, ch, src); got != nil {
			t.Errorf("custom mode topics = %v, want nil", got)
		}
	})

	t.Run("filtered resolves embedded category map", func(t *testing.T) {
		ch := sources.Channel{RelevanceMode: string(sources.RelevanceFiltered)}
		src := gen.IntelSource{Category: "visa"}

		got := intelRelevanceMapTopics(ctx, ch, src)
		if len(got) == 0 {
			t.Fatal("filtered mode returned no topics")
		}
		// Embedded map seeds visa → includes "visa".
		found := false

		for _, tp := range got {
			if tp == "visa" {
				found = true
			}
		}

		if !found {
			t.Errorf("visa category topics = %v, want to contain \"visa\"", got)
		}
	})

	t.Run("categories array preferred over scalar", func(t *testing.T) {
		ch := sources.Channel{RelevanceMode: string(sources.RelevanceFiltered)}
		src := gen.IntelSource{Category: "visa", Categories: []string{"scholarship"}}
		got := intelRelevanceMapTopics(ctx, ch, src)
		found := false

		for _, tp := range got {
			if tp == "scholarship" {
				found = true
			}
		}

		if !found {
			t.Errorf("categories=[scholarship] topics = %v, want to contain \"scholarship\"", got)
		}
	})

	t.Run("live setting JSON wins", func(t *testing.T) {
		appSettings = relevanceTestSettings{strings: map[string]string{
			"intel.category_method_map": `{"custom_cat":{"entity_types":[],"methods":[],"pipeline":"curate","topics":["bespoke-topic"]}}`,
		}}
		defer func() { appSettings = relevanceTestSettings{} }()

		ch := sources.Channel{RelevanceMode: string(sources.RelevanceFiltered)}
		src := gen.IntelSource{Category: "custom_cat"}

		got := intelRelevanceMapTopics(ctx, ch, src)
		if len(got) != 1 || got[0] != "bespoke-topic" {
			t.Errorf("live map topics = %v, want [bespoke-topic]", got)
		}
	})

	t.Run("no categories → nil", func(t *testing.T) {
		ch := sources.Channel{RelevanceMode: string(sources.RelevanceFiltered)}
		if got := intelRelevanceMapTopics(ctx, ch, gen.IntelSource{}); got != nil {
			t.Errorf("topics = %v, want nil", got)
		}
	})
}

// TestIntelEvaluateRelevance exercises the production gate function with
// stubbed package vars: settings tune the thresholds, the registered
// classifier port decides the borderline band, and a nil Valkey client
// disables the cache/counter (classifier skipped — cap unenforceable).
func TestIntelEvaluateRelevance(t *testing.T) {
	ctx := t.Context()

	oldSettings, oldCls, oldRdb := appSettings, intelRelevanceClassifier, rdb

	t.Cleanup(func() { appSettings, intelRelevanceClassifier, rdb = oldSettings, oldCls, oldRdb })

	appSettings = relevanceTestSettings{}
	rdb = nil

	ch := sources.Channel{RelevanceMode: string(sources.RelevanceFiltered), RelevanceTopics: []string{"visa"}}

	t.Run("on_topic passes", func(t *testing.T) {
		v, err := intelEvaluateRelevance(ctx, relevance.ItemInput{Title: "New visa rules"}, ch, nil)
		if err != nil || v != relevance.VerdictOnTopic {
			t.Fatalf("verdict = %v, %v; want on_topic", v, err)
		}
	})

	t.Run("off_topic drops", func(t *testing.T) {
		v, err := intelEvaluateRelevance(ctx, relevance.ItemInput{Title: "Local football scores"}, ch, nil)
		if err != nil || v != relevance.VerdictOffTopic {
			t.Fatalf("verdict = %v, %v; want off_topic", v, err)
		}
	})

	t.Run("classifier_enabled=false skips the borderline band", func(t *testing.T) {
		appSettings = relevanceTestSettings{bools: map[string]bool{"intel.relevance.classifier_enabled": false}}
		defer func() { appSettings = relevanceTestSettings{} }()

		cls := &fakeRelevanceClassifier{onTopic: true}
		intelRelevanceClassifier = cls

		defer func() { intelRelevanceClassifier = nil }()

		// Borderline-scoring item: one summary hit in a longer body.
		item := relevance.ItemInput{
			Title:   "Weekly ministry roundup",
			Summary: "New guidance covers permits and the visa bulletin plus routine notices for residents.",
		}

		v, err := intelEvaluateRelevance(ctx, item, ch, nil)
		if err != nil || v != relevance.VerdictBorderline {
			t.Fatalf("verdict = %v, %v; want borderline", v, err)
		}

		if cls.calls != 0 {
			t.Fatalf("classifier calls = %d, want 0 (disabled)", cls.calls)
		}
	})

	t.Run("thresholds honoured from settings", func(t *testing.T) {
		appSettings = relevanceTestSettings{floats: map[string]float64{
			"intel.relevance.on_topic_threshold": 0.9,
		}}
		defer func() { appSettings = relevanceTestSettings{} }()

		// 0.25-density hit no longer clears a 0.9 on-threshold → not on_topic.
		v, err := intelEvaluateRelevance(ctx, relevance.ItemInput{Title: "Visa requirements for Nigerians"}, ch, nil)
		if err != nil || v == relevance.VerdictOnTopic {
			t.Fatalf("verdict = %v, %v; want non-on_topic under raised threshold", v, err)
		}
	})
}

// TestIntelRelevanceCallFor covers the extract-path channel resolution:
// nil gate → nil context; wired gate with no channel row → synthetic
// filtered channel scoped to the source id.
func TestIntelRelevanceCallFor(t *testing.T) {
	ctx := t.Context()

	oldGate, oldSrcStore := intelRelevanceGate, intelSourcesStore

	t.Cleanup(func() { intelRelevanceGate, intelSourcesStore = oldGate, oldSrcStore })

	t.Run("nil gate → nil", func(t *testing.T) {
		intelRelevanceGate = nil

		if got := intelRelevanceCallFor(ctx, "", gen.IntelRawDocument{}, gen.IntelSource{}); got != nil {
			t.Fatalf("intelRelevanceCallFor = %+v, want nil", got)
		}
	})

	t.Run("no channel → synthetic filtered scoped to source", func(t *testing.T) {
		intelRelevanceGate = intelEvaluateRelevance
		intelSourcesStore = nil // resolveIntelPipelineChannel fails → no channel

		var srcID pgtype.UUID
		if err := srcID.Scan("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"); err != nil {
			t.Fatal(err)
		}

		src := gen.IntelSource{ID: srcID, Category: "visa"}

		got := intelRelevanceCallFor(ctx, "", gen.IntelRawDocument{}, src)
		if got == nil {
			t.Fatal("intelRelevanceCallFor = nil, want synthetic call")
		}

		if got.ch.RelevanceMode != string(sources.RelevanceFiltered) {
			t.Errorf("synthetic channel mode = %q, want filtered", got.ch.RelevanceMode)
		}

		if got.ch.ID != srcID {
			t.Errorf("synthetic channel ID = %v, want source id %v", got.ch.ID, srcID)
		}
		// Category "visa" resolves map topics (embedded default).
		if len(got.mapTopics) == 0 {
			t.Error("mapTopics empty for visa category")
		}
	})
}

// TestRegisterIntelDependencies_InstallsRelevanceGate verifies the
// canonical wiring installs the production gate so the curate/extract
// handlers gate items by default.
func TestRegisterIntelDependencies_InstallsRelevanceGate(t *testing.T) {
	old := intelRelevanceGate

	t.Cleanup(func() { intelRelevanceGate = old })

	intelRelevanceGate = nil
	RegisterIntelDependencies(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	if intelRelevanceGate == nil {
		t.Fatal("RegisterIntelDependencies left intelRelevanceGate nil")
	}
}

// ---------------------------------------------------------------------------
// Live-DB tests (gated on JC_TEST_DATABASE_URL via wireIntelChannelDeps)
// ---------------------------------------------------------------------------

// TestHandleIntelCurateChannel_RelevanceGate exercises the production gate
// end to end on the curate path: a topical curated record persists as
// 'extracted' + enqueues dedup; an off-topic one persists as 'off_topic'
// with its external_key (re-crawl dedup) and enqueues nothing.
func TestHandleIntelCurateChannel_RelevanceGate(t *testing.T) {
	database, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	// Install the production gate — wireIntelChannelDeps sets package vars
	// directly so the seam is still nil here.
	setRelevanceGate(t, intelEvaluateRelevance)

	// Pin the trust gate off so relevance routing is under test in
	// isolation — the default source fixture is trust_level=1, which would
	// otherwise auto-publish under the spec-default ceiling (2). The fake
	// returns reader fallbacks for the relevance knobs, matching the
	// nil-settings defaults intelEvaluateRelevance uses.
	oldSettings := appSettings
	appSettings = fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 0}}

	t.Cleanup(func() { appSettings = oldSettings })

	// Category "news" seeds map topics (migration, visa, diaspora, …).
	src := mkIntelTestSource(t, srcStore, "rel-gate-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "rss"
		a.Pipeline = "curate"
		a.EntityTypes = []string{"news"}
		a.RelevanceMode = "filtered"
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		Body:        []byte("<rss/>relgate"),
		ContentHash: "relgatehash",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	setIntelCurator(t, &fakeIntelCurator{recs: []IntelCuratedRecord{
		{
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":       "New student visa rules for diaspora applicants",
				"article_url": "https://news.example.com/visa-1",
			}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "https://news.example.com/visa-1",
			EntityType:        "news",
		},
		{
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":       "Local football league results announced",
				"article_url": "https://news.example.com/sport-9",
			}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "https://news.example.com/sport-9",
			EntityType:        "news",
		},
	}})

	payload, _ := json.Marshal(intelCurateChannelPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
	})

	if err := HandleIntelCurateChannel(t.Context(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel: %v", err)
	}

	rows, err := database.Writer.Query(t.Context(),
		`SELECT external_key, status FROM intel_records WHERE raw_document_id = $1 ORDER BY external_key`, rawDoc.ID)
	if err != nil {
		t.Fatalf("query records: %v", err)
	}
	defer rows.Close()

	statuses := map[string]string{}

	for rows.Next() {
		var key, status string
		if err := rows.Scan(&key, &status); err != nil {
			t.Fatalf("scan: %v", err)
		}

		statuses[key] = status
	}

	if statuses["https://news.example.com/visa-1"] != "extracted" {
		t.Errorf("topical record status = %q, want extracted", statuses["https://news.example.com/visa-1"])
	}

	if statuses["https://news.example.com/sport-9"] != "off_topic" {
		t.Errorf("off-topic record status = %q, want off_topic", statuses["https://news.example.com/sport-9"])
	}

	// Only the on-topic record enqueues dedup.
	if n := len(pendingTasks(t, inspector, TypeIntelDedup)); n != 1 {
		t.Errorf("dedup tasks = %d, want 1 (off_topic must not enqueue)", n)
	}

	// Re-crawl dedup: a second run re-gates the off-topic item, but its
	// external_key row already exists → still exactly 2 rows, still 1 dedup.
	if err := HandleIntelCurateChannel(t.Context(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
		t.Fatalf("HandleIntelCurateChannel re-run: %v", err)
	}

	var n int
	if err := database.Writer.QueryRow(t.Context(),
		`SELECT count(*) FROM intel_records WHERE raw_document_id = $1`, rawDoc.ID).Scan(&n); err != nil {
		t.Fatalf("count records: %v", err)
	}

	if n != 2 {
		t.Errorf("records after re-crawl = %d, want 2 (external_key dedup)", n)
	}
}

// TestHandleIntelExtract_RelevanceGate exercises the post-extraction gate:
// extracted records are evaluated on their title/summary/article_url and
// off-topic ones land as 'off_topic' without a dedup enqueue. A stub gate
// keeps the test independent of topic scoring (covered by the package
// tests + the curate live test above).
func TestHandleIntelExtract_RelevanceGate(t *testing.T) {
	database, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	setRelevanceGate(t, func(_ context.Context, item relevance.ItemInput, _ sources.Channel, _ []string) (relevance.Verdict, error) {
		if item.URL == "https://news.example.com/off" {
			return relevance.VerdictOffTopic, nil
		}

		return relevance.VerdictOnTopic, nil
	})

	src := mkIntelTestSource(t, srcStore, "rel-ext-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.CrawlMethod = "static"
		a.Pipeline = "extract"
		a.EntityTypes = []string{"news"}
	})

	rawDoc, err := intelDocStore.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		// ≥20 visible chars — HasExtractableText skips thinner HTML as
		// JS-shell/blank pages before the extractor runs.
		Body:        []byte("<html><body><article><h1>News page</h1><p>Visa bulletin and sports coverage.</p></article></body></html>"),
		ContentHash: "relexthash",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	ext := &fakeIntelExtractor{results: []IntelExtractResult{
		{
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":       "Visa bulletin",
				"article_url": "https://news.example.com/on",
			}),
			ConfidenceOverall: 0.9,
			ExternalKey:       "on-1",
		},
		{
			RawExtraction: marshalCuratedExtraction(t, map[string]any{
				"title":       "Sports roundup",
				"article_url": "https://news.example.com/off",
			}),
			ConfidenceOverall: 0.9,
			ExternalKey:       "off-1",
		},
	}}

	oldExt := intelExtractor
	intelExtractor = ext

	t.Cleanup(func() { intelExtractor = oldExt })

	payload, _ := json.Marshal(intelExtractPayload{
		Version:   intelPayloadVersion,
		RawDocID:  rawDoc.ID.String(),
		ChannelID: ch.ID.String(),
	})

	if err := HandleIntelExtract(t.Context(), asynq.NewTask(TypeIntelExtract, payload)); err != nil {
		t.Fatalf("HandleIntelExtract: %v", err)
	}

	rows, err := database.Writer.Query(t.Context(),
		`SELECT external_key, status FROM intel_records WHERE raw_document_id = $1 ORDER BY external_key`, rawDoc.ID)
	if err != nil {
		t.Fatalf("query records: %v", err)
	}
	defer rows.Close()

	statuses := map[string]string{}

	for rows.Next() {
		var key, status string
		if err := rows.Scan(&key, &status); err != nil {
			t.Fatalf("scan: %v", err)
		}

		statuses[key] = status
	}

	if statuses["on-1"] != "extracted" {
		t.Errorf("on-topic record status = %q, want extracted", statuses["on-1"])
	}

	if statuses["off-1"] != "off_topic" {
		t.Errorf("off-topic record status = %q, want off_topic", statuses["off-1"])
	}

	if n := len(pendingTasks(t, inspector, TypeIntelDedup)); n != 1 {
		t.Errorf("dedup tasks = %d, want 1", n)
	}
}

// TestIntelRelevanceCall_VerdictFailOpen covers the worker-side fail-open:
// a gate error passes the item instead of dropping it.
func TestIntelRelevanceCall_VerdictFailOpen(t *testing.T) {
	setRelevanceGate(t, func(context.Context, relevance.ItemInput, sources.Channel, []string) (relevance.Verdict, error) {
		return relevance.VerdictBorderline, errors.New("gateway down")
	})

	rc := &intelRelevanceCall{ch: sources.Channel{}}
	if got := rc.verdict(t.Context(), relevance.ItemInput{Title: "x"}, "test"); got != relevance.VerdictOnTopic {
		t.Fatalf("verdict on error = %v, want on_topic (fail-open)", got)
	}
}
