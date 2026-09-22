package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/worker"
)

// --- Phase-3 hardening: websub denied visibility + admin re-drive -----------

// newLiveInboundHandlers wires a Handlers backed by live Postgres + miniredis
// Asynq/Valkey for the inbound hardening tests (newLiveChannelHandlers does
// not wire intelStores/rdb, which the websub paths need). Returns the *db.DB
// so fixtures can clean up FK-protected rows.
func newLiveInboundHandlers(t *testing.T) (*Handlers, *db.DB, *asynq.Inspector) {
	t.Helper()

	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping live Postgres test")
	}
	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Skipf("test database unreachable: %v", err)
	}
	// t.Cleanup (not defer) — fixture cleanup registered by mkInboundChannel
	// must run before the pool closes (LIFO).
	t.Cleanup(func() { database.Close() })

	client, inspector := newTestAsynqClient(t)
	h := &Handlers{
		intelStores:  stores.NewStore(database),
		intelSources: sources.NewService(sources.NewStore(database)),
		asynqClient:  client,
		rdb:          startProbeTestValkey(t),
	}

	return h, database, inspector
}

// mkInboundChannel inserts a channel with the given crawl method + provider
// config under a fresh test source. The source URL carries a per-test suffix
// — source_url is unique and other live tests share the fixed example.test
// fixture. Cleanup removes the whole chain: raw docs (channel_id FK is
// NO ACTION) → channel (inbound events cascade) → source.
func mkInboundChannel(t *testing.T, h *Handlers, database *db.DB, method string, providerConfig string) gen.IntelSourceChannel {
	t.Helper()

	ctx := context.Background()
	svc := h.intelSources
	src, err := svc.Create(ctx, gen.CreateIntelSourceParams{
		Name:               "Inbound Test Source " + t.Name(),
		Domain:             "example.test",
		SourceUrl:          "https://example.test/feed/" + t.Name(),
		Category:           "job",
		SourceType:         "static",
		TrustLevel:         3,
		CrawlMethod:        "rss",
		CrawlFrequency:     "daily",
		CrawlFrequencyMins: 1440,
		RobotsPolicy:       []byte("{}"),
		ProviderConfig:     []byte("{}"),
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("create test source: %v", err)
	}
	sourceID := src.ID

	ch, err := svc.CreateChannel(ctx, gen.CreateIntelSourceChannelParams{
		SourceID:             sourceID,
		Label:                "inbound test channel",
		EntryUrl:             "https://example.test/inbound/" + t.Name(),
		CrawlMethod:          method,
		EntityTypes:          []string{"news"},
		Pipeline:             "curate",
		ProviderConfig:       []byte(providerConfig),
		CrawlFrequencyMins:   1440,
		Enabled:              true,
		Origin:               "admin",
		RelevanceMode:        "filtered",
		RelevanceTopics:      []string{},
		RelevanceUrlPatterns: []string{},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Writer.Exec(ctx, "DELETE FROM intel_raw_documents WHERE channel_id=$1", ch.ID)
		_ = svc.DeleteChannel(ctx, ch.ID)
		_ = svc.Delete(ctx, sourceID)
	})

	return ch
}

// mkInboundEvent persists an event and drives it to the wanted status.
// hash is the content_hash — the websub doc URL derives from it, so tests
// that exercise the doc-reuse path need to know it upfront.
func mkInboundEvent(t *testing.T, h *Handlers, chID pgtype.UUID, status, hash string) gen.IntelInboundEvent {
	t.Helper()

	event, err := h.intelSources.CreateInboundEvent(context.Background(), gen.CreateIntelInboundEventParams{
		ChannelID:   chID,
		Payload:     []byte(`{"from":"a@b.example","to":"intel@inbound.japa.example","subject":"s","text":"body"}`),
		Headers:     []byte(`{"from":"a@b.example"}`),
		ContentType: strPtrInbound("application/x-intel-email"),
		ContentHash: hash,
	})
	if err != nil {
		t.Fatalf("create inbound event: %v", err)
	}
	if status != "received" {
		if err := h.intelSources.MarkInboundEvent(context.Background(), gen.MarkIntelInboundEventParams{
			ID: event.ID, Status: status,
		}); err != nil {
			t.Fatalf("mark event %s: %v", status, err)
		}
		event.Status = status
	}

	return event
}

// webSubVerifyRequest builds a GET with the chi channel_id param set.
func webSubVerifyRequest(t *testing.T, channelID, query string) *http.Request {
	t.Helper()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("channel_id", channelID)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/intel/inbound/websub/"+channelID+"?"+query, nil)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// TestWebSubVerify_Denied covers the hardening change: hub.mode=denied echoes
// the challenge (per spec), drops any stored lease, and surfaces hub.reason
// on the channel's last_error_message — without touching crawl counters.
func TestWebSubVerify_Denied(t *testing.T) {
	h, database, _ := newLiveInboundHandlers(t)

	ch := mkInboundChannel(t, h, database, "webhook",
		`{"topic":"https://hub.example/feed","hub":"https://hub.example/"}`)

	// Seed a stale lease — denied must clear it.
	leaseKey := "jc:intel:websub:lease:" + ch.ID.String()
	if err := h.rdb.Set(context.Background(), leaseKey, `{"hub":"x"}`, time.Hour).Err(); err != nil {
		t.Fatalf("seed lease: %v", err)
	}

	r := webSubVerifyRequest(t, ch.ID.String(),
		"hub.mode=denied&hub.topic=https%3A%2F%2Fhub.example%2Ffeed&hub.challenge=CH123&hub.reason=quota+exceeded")
	rr := httptest.NewRecorder()
	h.HandleWebSubVerify(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "CH123" {
		t.Errorf("challenge echo = %q, want CH123", rr.Body.String())
	}
	if n, _ := h.rdb.Exists(context.Background(), leaseKey).Result(); n != 0 {
		t.Error("lease key still present after denied")
	}

	got, err := h.intelSources.GetChannel(context.Background(), ch.ID)
	if err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if got.LastErrorMessage == nil ||
		!strings.Contains(*got.LastErrorMessage, "denied") ||
		!strings.Contains(*got.LastErrorMessage, "quota") {
		t.Errorf("last_error_message = %v, want denial + hub.reason", got.LastErrorMessage)
	}
	if got.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0 (denied is not a crawl failure)", got.ConsecutiveFailures)
	}
}

// TestAdminRedriveInboundEvent_Email re-drives a failed email delivery: CAS
// back to received, then a fresh intel:inbound_email task on the critical
// queue.
func TestAdminRedriveInboundEvent_Email(t *testing.T) {
	h, database, inspector := newLiveInboundHandlers(t)

	ch := mkInboundChannel(t, h, database, "email", `{"inbound_address":"intel@inbound.japa.example"}`)
	event := mkInboundEvent(t, h, ch.ID, "failed", "hash-email-1")

	rr := httptest.NewRecorder()
	r := authedChannelRequest(t, http.MethodPost, "/", event.ID.String(), "", true)
	h.HandleAdminRedriveInboundEvent(rr, r)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body=%s)", rr.Code, rr.Body.String())
	}

	got, err := h.intelSources.GetInboundEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got.Status != "received" {
		t.Errorf("event status = %q, want received", got.Status)
	}

	tasks, err := inspector.ListPendingTasks("critical", asynq.PageSize(10))
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	var found bool
	for _, task := range tasks {
		if task.Type == worker.TypeIntelInboundEmail {
			var p map[string]string
			if err := json.Unmarshal(task.Payload, &p); err == nil && p["event_id"] == event.ID.String() {
				found = true
			}
		}
	}
	if !found {
		t.Error("no intel:inbound_email task enqueued for the redriven event")
	}
}

// TestAdminRedriveInboundEvent_WebSub re-materializes a failed push delivery
// inline: raw doc created, detect_changes enqueued, event ends processed.
func TestAdminRedriveInboundEvent_WebSub(t *testing.T) {
	h, database, inspector := newLiveInboundHandlers(t)

	ch := mkInboundChannel(t, h, database, "webhook",
		`{"topic":"https://hub.example/feed","hub":"https://hub.example/"}`)
	hash := "deadbeefdeadbeefdeadbeefdeadbeef"
	event := mkInboundEvent(t, h, ch.ID, "failed", hash)

	// Pre-seed the doc the first (crashed) materialization persisted —
	// the deterministic websub:{ch}:{hash16} URL means the redrive must
	// reuse this row instead of inserting a duplicate.
	docURL := "websub:" + ch.ID.String() + ":" + hash[:16]
	var docID pgtype.UUID
	err := database.Writer.QueryRow(context.Background(),
		`INSERT INTO intel_raw_documents (source_id, channel_id, url, http_status, content_type, body, content_hash)
		 VALUES ((SELECT source_id FROM intel_source_channels WHERE id=$1), $1, $2, 200, 'application/atom+xml', '\x7b7d'::bytea, $3)
		 RETURNING id`, ch.ID, docURL, hash).Scan(&docID)
	if err != nil {
		t.Fatalf("seed existing raw doc: %v", err)
	}

	rr := httptest.NewRecorder()
	r := authedChannelRequest(t, http.MethodPost, "/", event.ID.String(), "", true)
	h.HandleAdminRedriveInboundEvent(rr, r)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body=%s)", rr.Code, rr.Body.String())
	}
	data := decodeChannelData(t, rr)
	if data["raw_doc_id"] != docID.String() {
		t.Errorf("raw_doc_id = %v, want reused doc %s — a duplicate was inserted", data["raw_doc_id"], docID.String())
	}

	got, err := h.intelSources.GetInboundEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got.Status != "processed" {
		t.Errorf("event status = %q, want processed", got.Status)
	}

	tasks, err := inspector.ListPendingTasks("default", asynq.PageSize(10))
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	var found bool
	for _, task := range tasks {
		if task.Type != worker.TypeIntelDetectChanges {
			continue
		}
		found = true
		// The seeded doc is the channel's latest — a self-referential
		// prior_hash would make detect_changes decide 'unchanged' and the
		// redriven delivery would produce no records. It must be empty so
		// the pipeline is forced to run (dedup keeps it idempotent).
		var p struct {
			PriorHash string `json:"prior_hash"`
		}
		if err := json.Unmarshal(task.Payload, &p); err == nil && p.PriorHash != "" {
			t.Errorf("detect_changes prior_hash = %q, want empty on re-materialization", p.PriorHash)
		}
	}
	if !found {
		t.Error("no intel:detect_changes task enqueued for the redriven delivery")
	}
}

// TestAdminRedriveInboundEvent_NotRedrivable covers the 409 contract —
// 'received' (waiting) and 'processed' (done) events are not re-drivable —
// plus the unauthenticated rejection.
func TestAdminRedriveInboundEvent_NotRedrivable(t *testing.T) {
	h, database, _ := newLiveInboundHandlers(t)

	ch := mkInboundChannel(t, h, database, "email", `{"inbound_address":"intel@inbound.japa.example"}`)

	for _, status := range []string{"received", "processed"} {
		event := mkInboundEvent(t, h, ch.ID, status, "hash-"+status+"-"+t.Name()[:8])

		rr := httptest.NewRecorder()
		r := authedChannelRequest(t, http.MethodPost, "/", event.ID.String(), "", true)
		h.HandleAdminRedriveInboundEvent(rr, r)

		if status == "processed" && rr.Code != http.StatusConflict {
			t.Errorf("status=%s: redrive = %d, want 409", status, rr.Code)
		}
		if status == "received" && rr.Code != http.StatusConflict {
			t.Errorf("status=%s: redrive = %d, want 409", status, rr.Code)
		}
	}

	// Unauthenticated → 401 before any state work.
	event := mkInboundEvent(t, h, ch.ID, "failed", "hash-unauthed")
	rr := httptest.NewRecorder()
	r := authedChannelRequest(t, http.MethodPost, "/", event.ID.String(), "", false)
	h.HandleAdminRedriveInboundEvent(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("unauthed redrive = %d, want 401", rr.Code)
	}
}
