package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/scheduler"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/intelligence/stores"
)

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

// TestResolveChannelEntityTypes covers the channel-first entity-type
// precedence: channel.entity_types wins over provider_config and category,
// unregistered values are dropped, and an empty channel array falls back to
// the source-level resolution (provider_config → category).
func TestResolveChannelEntityTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		channelTypes   []string
		category       string
		providerConfig string
		want           []string
	}{
		{
			name:           "channel types win over everything",
			channelTypes:   []string{"job", "visa"},
			category:       "scholarship",
			providerConfig: `{"entity_types":["news"]}`,
			want:           []string{"job", "visa"},
		},
		{
			name:           "unregistered channel values dropped",
			channelTypes:   []string{"bogus_domain", "job"},
			category:       "scholarship",
			providerConfig: `{}`,
			want:           []string{"job"},
		},
		{
			name:           "all-invalid channel types fall back to source",
			channelTypes:   []string{"bogus_domain"},
			category:       "news",
			providerConfig: `{"entity_types":["visa"]}`,
			want:           []string{"visa"},
		},
		{
			name:           "empty channel array falls back to provider_config",
			channelTypes:   []string{},
			category:       "job",
			providerConfig: `{"entity_types":["scholarship"]}`,
			want:           []string{"scholarship"},
		},
		{
			name:           "nil channel array falls back to category",
			channelTypes:   nil,
			category:       "remittance",
			providerConfig: `{}`,
			want:           []string{"remittance"},
		},
		{
			name:           "channel types deduped",
			channelTypes:   []string{"job", "job", "visa"},
			category:       "news",
			providerConfig: `{}`,
			want:           []string{"job", "visa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ch := gen.IntelSourceChannel{EntityTypes: tt.channelTypes}
			src := gen.IntelSource{Category: tt.category, ProviderConfig: []byte(tt.providerConfig)}

			got := resolveChannelEntityTypes(ch, src)

			if len(got) != len(tt.want) {
				t.Fatalf("resolveChannelEntityTypes = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("resolveChannelEntityTypes[%d] = %q, want %q (got %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

// TestHandleIntelFetchChannel_NotWired verifies the handler degrades to a
// logged skip when dependencies were never registered.
func TestHandleIntelFetchChannel_NotWired(t *testing.T) {
	oldFetcher, oldStores, oldSrcStore := intelFetcher, intelStores, intelSourcesStore
	intelFetcher, intelStores, intelSourcesStore = nil, nil, nil
	t.Cleanup(func() { intelFetcher, intelStores, intelSourcesStore = oldFetcher, oldStores, oldSrcStore })

	task := asynq.NewTask(TypeIntelFetchChannel, []byte(`{"version":"v1","channel_id":"11111111-1111-4111-8111-111111111111","source_id":"22222222-2222-4222-8222-222222222222"}`))
	if err := HandleIntelFetchChannel(context.Background(), task); err != nil {
		t.Fatalf("unwired handler returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Live-DB fixtures (gated on JC_TEST_DATABASE_URL, mirrors intel_probe_test)
// ---------------------------------------------------------------------------

// wireIntelChannelDeps opens the test database, spins up miniredis for the
// scheduler/preflight/asynq-client dependencies, and registers the full
// intel dependency set on the package vars (restored via t.Cleanup).
// Returns the database + stores + an asynq inspector pointed at the same
// miniredis so tests can assert on enqueued follow-up tasks.
func wireIntelChannelDeps(t *testing.T) (*db.DB, *sources.Store, *stores.Store, *asynq.Inspector) {
	t.Helper()

	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping live Postgres test")
	}

	ctx := context.Background()

	database, err := db.New(ctx, dsn)
	if err != nil {
		t.Skipf("test database unreachable: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(func() { vk.Close() })
	adapter := valkeycompat.NewAdapter(vk)

	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = asynqClient.Close() })
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = inspector.Close() })

	srcStore := sources.NewStore(database)
	store := stores.NewStore(database)

	oldFetcher, oldStores, oldSrcStore := intelFetcher, intelStores, intelSourcesStore
	oldSched, oldPreflight, oldClient := intelScheduler, intelPreflight, intelAsynqClient
	oldRdb := rdb

	// The composite fetcher runs against httptest fixtures on 127.0.0.1, so
	// it needs the AllowPrivate opt-out — production uses fetcher.NewComposite
	// (SSRF dial guard on).
	intelFetcher = fetcher.NewCompositeWithClient(
		fetcher.NewSafeClientOpts(30*time.Second, 0, fetcher.SafeClientOpts{AllowPrivate: true}),
		nil, nil,
	)
	intelStores = store
	intelSourcesStore = srcStore
	intelScheduler = scheduler.NewScheduler(srcStore, srcStore, adapter)
	intelPreflight = fetcher.NewPreflightChecker(adapter, nil, nil)
	intelAsynqClient = asynqClient
	rdb = adapter

	t.Cleanup(func() {
		intelFetcher, intelStores, intelSourcesStore = oldFetcher, oldStores, oldSrcStore
		intelScheduler, intelPreflight, intelAsynqClient = oldSched, oldPreflight, oldClient
		rdb = oldRdb
	})

	return database, srcStore, store, inspector
}

// mkIntelTestSource inserts a crawlable intel_sources row and registers its
// cleanup. mutate may tweak the defaults (disabled, proposed, …).
func mkIntelTestSource(t *testing.T, srcStore *sources.Store, name string, mutate func(*gen.CreateIntelSourceParams)) gen.IntelSource {
	t.Helper()

	arg := gen.CreateIntelSourceParams{
		Name: name, Domain: "example.com", SourceUrl: "https://example.com/" + name,
		Category: "news", SourceType: "auto", SourceKind: "publisher",
		TrustLevel: 1, CrawlMethod: "static", CrawlFrequency: "daily",
		CrawlFrequencyMins: 1440,
		RobotsPolicy:       []byte("{}"), ProviderConfig: []byte("{}"),
		Enabled: true, Status: "active",
	}
	if mutate != nil {
		mutate(&arg)
	}

	src, err := srcStore.Create(context.Background(), arg)
	if err != nil {
		t.Fatalf("create source %s: %v", name, err)
	}
	t.Cleanup(func() { _ = srcStore.Delete(context.Background(), src.ID) })

	return src
}

// mkIntelTestChannel inserts an intel_source_channels row under src.
func mkIntelTestChannel(t *testing.T, srcStore *sources.Store, src gen.IntelSource, mutate func(*gen.CreateIntelSourceChannelParams)) gen.IntelSourceChannel {
	t.Helper()

	arg := gen.CreateIntelSourceChannelParams{
		SourceID:             src.ID,
		Label:                "test channel",
		EntryUrl:             src.SourceUrl,
		CrawlMethod:          "static",
		EntityTypes:          []string{},
		Pipeline:             "extract",
		ProviderConfig:       []byte("{}"),
		CrawlFrequencyMins:   1440,
		Enabled:              true,
		Origin:               "admin",
		RelevanceMode:        "filtered",
		RelevanceTopics:      []string{},
		RelevanceUrlPatterns: []string{},
	}
	if mutate != nil {
		mutate(&arg)
	}

	ch, err := srcStore.CreateChannel(context.Background(), arg)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	return ch
}

// countChannelRawDocs returns the number of raw documents stamped with the
// given channel id.
func countChannelRawDocs(t *testing.T, database *db.DB, channelID pgtype.UUID) int {
	t.Helper()

	var n int
	if err := database.Writer.QueryRow(context.Background(),
		`SELECT count(*) FROM intel_raw_documents WHERE channel_id = $1`, channelID).Scan(&n); err != nil {
		t.Fatalf("count raw docs: %v", err)
	}

	return n
}

// pendingTasks returns all pending tasks of the given type across the
// default/critical/low queues. Queues that do not exist yet (no tasks ever
// enqueued there) are skipped — miniredis reports them as not found.
func pendingTasks(t *testing.T, inspector *asynq.Inspector, taskType string) []*asynq.TaskInfo {
	t.Helper()

	var out []*asynq.TaskInfo
	for _, q := range []string{QueueCritical, QueueDefault, QueueLow} {
		infos, err := inspector.ListPendingTasks(q, asynq.PageSize(100))
		if err != nil {
			if errors.Is(err, asynq.ErrQueueNotFound) {
				continue
			}
			t.Fatalf("inspect queue %s: %v", q, err)
		}
		for _, info := range infos {
			if info.Type == taskType {
				out = append(out, info)
			}
		}
	}

	return out
}

// scheduledTasks is pendingTasks' counterpart for the scheduled set —
// ProcessIn enqueues (e.g. WebSub renewals armed for a future lease
// window) land here, not on the pending list.
func scheduledTasks(t *testing.T, inspector *asynq.Inspector, taskType string) []*asynq.TaskInfo {
	t.Helper()

	var out []*asynq.TaskInfo
	for _, q := range []string{QueueCritical, QueueDefault, QueueLow} {
		infos, err := inspector.ListScheduledTasks(q, asynq.PageSize(100))
		if err != nil {
			if errors.Is(err, asynq.ErrQueueNotFound) {
				continue
			}
			t.Fatalf("inspect queue %s scheduled: %v", q, err)
		}
		for _, info := range infos {
			if info.Type == taskType {
				out = append(out, info)
			}
		}
	}

	return out
}

// staticFixture serves a small HTML page for fetch tests.
func staticFixture(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><p>channel fetch fixture</p></body></html>"))
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestHandleIntelFetchChannel_LiveDB exercises the happy path end to end:
// the handler fetches the channel's entry URL, persists a raw document with
// channel_id stamped, stamps the channel crawl status, and enqueues
// detect_changes carrying channel_id.
func TestHandleIntelFetchChannel_LiveDB(t *testing.T) {
	database, srcStore, _, inspector := wireIntelChannelDeps(t)
	srv := staticFixture(t)

	src := mkIntelTestSource(t, srcStore, "chan-happy", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.EntryUrl = srv.URL + "/page"
	})

	payload, _ := json.Marshal(intelChannelPayload{
		Version:   intelPayloadVersion,
		ChannelID: ch.ID.String(),
		SourceID:  src.ID.String(),
	})

	if err := HandleIntelFetchChannel(context.Background(), asynq.NewTask(TypeIntelFetchChannel, payload)); err != nil {
		t.Fatalf("HandleIntelFetchChannel: %v", err)
	}

	// Raw doc persisted with the channel stamp + channel entry URL.
	docID, docChannelID, docURL := latestChannelRawDoc(t, database, src, ch)
	if docChannelID != ch.ID.String() {
		t.Errorf("raw doc channel_id = %q, want %q", docChannelID, ch.ID.String())
	}
	if docURL != srv.URL+"/page" {
		t.Errorf("raw doc url = %q, want channel entry_url", docURL)
	}

	// Channel crawl status stamped.
	got, err := srcStore.GetChannel(context.Background(), ch.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if !got.LastSuccessAt.Valid {
		t.Error("channel last_success_at not stamped")
	}
	if got.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0", got.ConsecutiveFailures)
	}

	// detect_changes enqueued carrying channel_id + the new raw doc id.
	infos := pendingTasks(t, inspector, TypeIntelDetectChanges)
	if len(infos) == 0 {
		t.Fatal("no intel:detect_changes task enqueued")
	}
	var dc intelDetectChangesPayload
	if err := json.Unmarshal(infos[0].Payload, &dc); err != nil {
		t.Fatalf("detect_changes payload: %v", err)
	}
	if dc.ChannelID != ch.ID.String() {
		t.Errorf("detect_changes channel_id = %q, want %q", dc.ChannelID, ch.ID.String())
	}
	if dc.RawDocID != docID.String() {
		t.Errorf("detect_changes raw_doc_id = %q, want %q", dc.RawDocID, docID.String())
	}
}

// latestChannelRawDoc loads the newest raw doc row stamped with the given
// channel id — kept outside stores.Store since the channel_id column is
// only exposed through the raw query here.
func latestChannelRawDoc(t *testing.T, database *db.DB, src gen.IntelSource, ch gen.IntelSourceChannel) (docID pgtype.UUID, docChannelID, docURL string) {
	t.Helper()

	row := database.Writer.QueryRow(context.Background(),
		`SELECT id, channel_id::text, url FROM intel_raw_documents
		 WHERE source_id = $1 AND channel_id = $2
		 ORDER BY retrieved_at DESC LIMIT 1`, src.ID, ch.ID)

	if err := row.Scan(&docID, &docChannelID, &docURL); err != nil {
		t.Fatalf("raw doc lookup: %v", err)
	}

	return docID, docChannelID, docURL
}

// TestHandleIntelFetchChannel_SkipPaths covers the graceful-skip contract:
// disabled channels, inbound/manual methods, inactive or disabled parents,
// and a missing channel row all return nil without producing a raw doc or
// tripping the failure circuit-breaker.
func TestHandleIntelFetchChannel_SkipPaths(t *testing.T) {
	database, srcStore, _, _ := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "chan-skip", nil)
	disabledCh := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.Label = "disabled"
		a.EntryUrl = src.SourceUrl + "/disabled"
		a.Enabled = false
	})
	emailCh := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.Label = "email"
		a.CrawlMethod = "email"
		a.EntryUrl = "mailto:intake@example.com"
	})
	manualCh := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.Label = "manual"
		a.CrawlMethod = "manual"
		a.EntryUrl = src.SourceUrl + "/manual"
	})

	disabledParent := mkIntelTestSource(t, srcStore, "chan-parent-off", func(a *gen.CreateIntelSourceParams) {
		a.Enabled = false
	})
	orphanCh := mkIntelTestChannel(t, srcStore, disabledParent, nil)

	proposedParent := mkIntelTestSource(t, srcStore, "chan-parent-prop", func(a *gen.CreateIntelSourceParams) {
		a.Status = "proposed"
	})
	proposedCh := mkIntelTestChannel(t, srcStore, proposedParent, nil)

	var missing pgtype.UUID
	if err := missing.Scan("99999999-9999-4999-8999-999999999999"); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		channelID pgtype.UUID
	}{
		{"disabled channel", disabledCh.ID},
		{"email (inbound) channel", emailCh.ID},
		{"manual channel", manualCh.ID},
		{"disabled parent source", orphanCh.ID},
		{"non-active parent source", proposedCh.ID},
		{"missing channel row", missing},
	}

	for _, tc := range cases {
		payload, _ := json.Marshal(intelChannelPayload{
			Version:   intelPayloadVersion,
			ChannelID: tc.channelID.String(),
		})
		if err := HandleIntelFetchChannel(context.Background(), asynq.NewTask(TypeIntelFetchChannel, payload)); err != nil {
			t.Errorf("%s: returned error %v", tc.name, err)
		}
	}

	for _, ch := range []gen.IntelSourceChannel{disabledCh, emailCh, manualCh, orphanCh, proposedCh} {
		if n := countChannelRawDocs(t, database, ch.ID); n != 0 {
			t.Errorf("channel %s produced %d raw docs, want 0", ch.Label, n)
		}
		if got, err := srcStore.GetChannel(context.Background(), ch.ID); err == nil && got.ConsecutiveFailures != 0 {
			t.Errorf("channel %s consecutive_failures = %d, want 0 (skip paths must not trip the breaker)", ch.Label, got.ConsecutiveFailures)
		}
	}
}

// TestHandleIntelFetchSource_Delegates verifies the legacy fetch_source task
// resolves the source's default channel and runs the shared channel fetch
// path — the raw doc comes back stamped with channel_id.
func TestHandleIntelFetchSource_Delegates(t *testing.T) {
	database, srcStore, _, inspector := wireIntelChannelDeps(t)
	srv := staticFixture(t)

	src := mkIntelTestSource(t, srcStore, "legacy-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
		a.EntryUrl = srv.URL + "/legacy"
	})

	payload, _ := json.Marshal(intelSourcePayload{Version: intelPayloadVersion, SourceID: src.ID.String()})
	if err := HandleIntelFetchSource(context.Background(), asynq.NewTask(TypeIntelFetchSource, payload)); err != nil {
		t.Fatalf("HandleIntelFetchSource: %v", err)
	}

	_, docChannelID, _ := latestChannelRawDoc(t, database, src, ch)
	if docChannelID != ch.ID.String() {
		t.Errorf("delegated fetch stamped channel_id = %q, want %q", docChannelID, ch.ID.String())
	}

	// detect_changes carries the resolved channel id forward.
	infos := pendingTasks(t, inspector, TypeIntelDetectChanges)
	if len(infos) == 0 {
		t.Fatal("no detect_changes enqueued")
	}
	var dc intelDetectChangesPayload
	_ = json.Unmarshal(infos[0].Payload, &dc)
	if dc.ChannelID != ch.ID.String() {
		t.Errorf("detect_changes channel_id = %q, want %q", dc.ChannelID, ch.ID.String())
	}
}

// TestHandleIntelScheduleCrawls_EnqueuesChannels verifies the cron handler
// enqueues intel:fetch_channel per due channel — not fetch_source per
// source.
func TestHandleIntelScheduleCrawls_EnqueuesChannels(t *testing.T) {
	_, srcStore, _, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "sched-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, nil) // last_crawled_at NULL → due

	if err := HandleIntelScheduleCrawls(context.Background(), asynq.NewTask(TypeIntelScheduleCrawls, []byte("{}"))); err != nil {
		t.Fatalf("HandleIntelScheduleCrawls: %v", err)
	}

	infos := pendingTasks(t, inspector, TypeIntelFetchChannel)
	if len(infos) == 0 {
		t.Fatal("no intel:fetch_channel task enqueued")
	}

	var found bool
	for _, info := range infos {
		var p intelChannelPayload
		if err := json.Unmarshal(info.Payload, &p); err != nil {
			t.Fatalf("fetch_channel payload: %v", err)
		}
		if p.ChannelID == ch.ID.String() {
			found = true
			if p.SourceID != src.ID.String() {
				t.Errorf("payload source_id = %q, want %q", p.SourceID, src.ID.String())
			}
		}
	}
	if !found {
		t.Errorf("no fetch_channel task for channel %s", ch.ID.String())
	}

	if n := len(pendingTasks(t, inspector, TypeIntelFetchSource)); n != 0 {
		t.Errorf("schedule_crawls enqueued %d legacy fetch_source tasks, want 0", n)
	}
}

// TestHandleIntelDetectChanges_PropagatesChannel verifies detect_changes
// forwards channel_id into the extract enqueue (the Task 9 pipeline-routing
// hook).
func TestHandleIntelDetectChanges_PropagatesChannel(t *testing.T) {
	_, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "dc-src", nil)
	ch := mkIntelTestChannel(t, srcStore, src, nil)

	rawDoc, err := intelDocStore.CreateIntelRawDocument(context.Background(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		ChannelID:   ch.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: []byte(`{}`),
		Body:        []byte("<html><body>fresh content</body></html>"),
		ContentHash: "deadbeef",
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	payload, _ := json.Marshal(intelDetectChangesPayload{
		Version:   intelPayloadVersion,
		SourceID:  src.ID.String(),
		ChannelID: ch.ID.String(),
		RawDocID:  rawDoc.ID.String(),
		PriorHash: "", // empty → DecisionFirstCrawl → extract enqueued
	})

	if err := HandleIntelDetectChanges(context.Background(), asynq.NewTask(TypeIntelDetectChanges, payload)); err != nil {
		t.Fatalf("HandleIntelDetectChanges: %v", err)
	}

	infos := pendingTasks(t, inspector, TypeIntelExtract)
	if len(infos) == 0 {
		t.Fatal("no intel:extract task enqueued")
	}
	var ep intelExtractPayload
	if err := json.Unmarshal(infos[0].Payload, &ep); err != nil {
		t.Fatalf("extract payload: %v", err)
	}
	if ep.ChannelID != ch.ID.String() {
		t.Errorf("extract channel_id = %q, want %q", ep.ChannelID, ch.ID.String())
	}
}
