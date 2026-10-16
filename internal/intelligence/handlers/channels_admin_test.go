package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/rbac"
)

// --- channel admin test helpers ---------------------------------------------

// authedRequest builds a request with the chi URL param injected (as the
// router would) and, when authed, an AuthenticatedUser in the context (as
// auth.RequireAuth would).
func authedChannelRequest(t *testing.T, method, target, paramValue, body string, authed bool) *http.Request {
	t.Helper()

	rctx := chi.NewRouteContext()
	if paramValue != "" {
		rctx.URLParams.Add("id", paramValue)
	}

	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	if authed {
		u := &auth.AuthenticatedUser{UserID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")}
		r = r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, u))
	}

	return r
}

// newTestAsynqClient starts an in-process Redis (miniredis) and returns an
// Asynq client + inspector so enqueueing handlers can be exercised without a
// real broker and the queued task can be asserted.
func newTestAsynqClient(t *testing.T) (*asynq.Client, *asynq.Inspector) {
	t.Helper()

	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)

	t.Cleanup(func() {
		_ = client.Close()
		_ = inspector.Close()
	})

	return client, inspector
}

// assertErrorCode decodes the {error:{code}} envelope and checks status+code.
func assertErrorCode(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body=%s)", rr.Code, wantStatus, rr.Body.String())
	}

	var env errorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v (body=%s)", err, rr.Body.String())
	}

	if env.Error.Code != wantCode {
		t.Errorf("error.code = %q, want %q", env.Error.Code, wantCode)
	}
}

const testUUID = "550e8400-e29b-41d4-a716-446655440000"

// --- nil-dependency graceful degradation ------------------------------------

// TestAdminChannelHandlers_NotConfigured covers the nil-dependency contract:
// every channel admin handler returns 503 intel_unavailable before touching
// auth or the body.
func TestAdminChannelHandlers_NotConfigured(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"list", (&Handlers{}).HandleAdminListSourceChannels},
		{"create", (&Handlers{}).HandleAdminCreateChannel},
		{"get", (&Handlers{}).HandleAdminGetChannel},
		{"update", (&Handlers{}).HandleAdminUpdateChannel},
		{"delete", (&Handlers{}).HandleAdminDeleteChannel},
		{"crawl", (&Handlers{}).HandleAdminCrawlChannel},
		{"reprobe", (&Handlers{}).HandleAdminReprobeSource},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			r := authedChannelRequest(t, http.MethodPost, "/", testUUID, `{}`, true)
			tc.handler(rr, r)
			assertIntelUnavailable(t, rr)
		})
	}
}

// TestAdminChannelHandlers_Unauthenticated verifies every channel admin
// handler rejects requests without an authenticated user (401). The RBAC
// middleware runs before the handler in production; requireUser inside the
// handler is the defense-in-depth check.
func TestAdminChannelHandlers_Unauthenticated(t *testing.T) {
	t.Parallel()

	client, _ := newTestAsynqClient(t)

	for _, tc := range []struct {
		name    string
		handler func(h *Handlers) http.HandlerFunc
	}{
		{"list", func(h *Handlers) http.HandlerFunc { return h.HandleAdminListSourceChannels }},
		{"create", func(h *Handlers) http.HandlerFunc { return h.HandleAdminCreateChannel }},
		{"get", func(h *Handlers) http.HandlerFunc { return h.HandleAdminGetChannel }},
		{"update", func(h *Handlers) http.HandlerFunc { return h.HandleAdminUpdateChannel }},
		{"delete", func(h *Handlers) http.HandlerFunc { return h.HandleAdminDeleteChannel }},
		{"crawl", func(h *Handlers) http.HandlerFunc { return h.HandleAdminCrawlChannel }},
		{"reprobe", func(h *Handlers) http.HandlerFunc { return h.HandleAdminReprobeSource }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := &Handlers{intelSources: sources.NewService(nil), asynqClient: client}
			rr := httptest.NewRecorder()
			r := authedChannelRequest(t, http.MethodPost, "/", testUUID, `{}`, false)
			tc.handler(h)(rr, r)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d (body=%s)", rr.Code, http.StatusUnauthorized, rr.Body.String())
			}
		})
	}
}

// --- PATCH presence/validation (no store needed — fails before lookups) -----

// TestHandleAdminUpdateChannel_RequestValidation covers the PATCH request-
// level checks the handler enforces before any service call: JSON shape,
// entity-type registry membership, provider_config object-ness, and the
// no-editable-fields guard.
func TestHandleAdminUpdateChannel_RequestValidation(t *testing.T) {
	t.Parallel()

	h := &Handlers{intelSources: sources.NewService(nil)}

	for _, tc := range []struct {
		name     string
		body     string
		wantCode string
	}{
		{"invalid_json", `{bad`, "invalid_json"},
		{"empty_object", `{}`, "empty_patch"},
		{"unknown_keys_only", `{"frobnicate": true}`, "empty_patch"},
		{"unregistered_entity_type", `{"entity_types": ["not_a_schema"]}`, "invalid_entity_type"},
		{"provider_config_string", `{"provider_config": "oops"}`, "invalid_provider_config"},
		{"provider_config_array", `{"provider_config": [1,2]}`, "invalid_provider_config"},
		{"nonpositive_frequency", `{"crawl_frequency_mins": 0}`, "invalid_input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			r := authedChannelRequest(t, http.MethodPatch, "/", testUUID, tc.body, true)
			h.HandleAdminUpdateChannel(rr, r)
			assertErrorCode(t, rr, http.StatusBadRequest, tc.wantCode)
		})
	}
}

// TestValidateEntityTypes_ChannelPatch covers the registry check the PATCH
// handler applies when the entity_types key is present: an empty array is
// an explicit clear (valid), unregistered types are rejected, and a
// registered type passes. The end-to-end clear is asserted in the live-DB
// test.
func TestValidateEntityTypes_ChannelPatch(t *testing.T) {
	t.Parallel()

	if got := validateEntityTypes([]string{}); got != "" {
		t.Errorf("validateEntityTypes([]) = %q, want \"\" (explicit clear is valid)", got)
	}

	if got := validateEntityTypes(nil); got != "" {
		t.Errorf("validateEntityTypes(nil) = %q, want \"\" (null → clear)", got)
	}

	if got := validateEntityTypes([]string{"not_a_schema"}); got == "" {
		t.Error("validateEntityTypes([not_a_schema]) = \"\", want the offending value")
	}

	if got := validateEntityTypes([]string{"job"}); got != "" {
		t.Errorf("validateEntityTypes([job]) = %q, want \"\" (registered type)", got)
	}
}

// --- live-DB happy path + RBAC denial ---------------------------------------

// newLiveChannelHandlers wires a Handlers backed by a real Postgres
// (JC_TEST_DATABASE_URL) plus a miniredis-backed Asynq client. Returns nil
// handlers when the DSN is unset/unreachable so callers can t.Skip.
func newLiveChannelHandlers(t *testing.T) (*Handlers, *sources.Service, *asynq.Inspector, func()) {
	t.Helper()

	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping live Postgres test")
	}

	ctx := t.Context()

	database, err := db.New(ctx, dsn)
	if err != nil {
		t.Skipf("test database unreachable: %v", err)
	}

	svc := sources.NewService(sources.NewStore(database))
	client, inspector := newTestAsynqClient(t)

	h := &Handlers{intelSources: svc, asynqClient: client}

	cleanup := func() {
		database.Close()
	}

	return h, svc, inspector, cleanup
}

// createChannelTestSource inserts a minimal intel_sources row the channel
// tests can hang channels off. Returns the source ID.
func createChannelTestSource(t *testing.T, svc *sources.Service) pgtype.UUID {
	t.Helper()

	src, err := svc.Create(t.Context(), gen.CreateIntelSourceParams{
		Name:               "Channel Admin Test Source",
		Domain:             "example.test",
		SourceUrl:          "https://example.test/feed",
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

	return src.ID
}

// decodeChannelData decodes the {data:{...}} envelope into a map.
func decodeChannelData(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var env struct {
		Data map[string]any `json:"data"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode data envelope: %v (body=%s)", err, rr.Body.String())
	}

	return env.Data
}

// TestAdminChannelCRUD_LiveDB exercises the happy-path CRUD surface end to
// end against a live Postgres: create → list → get → patch (scalar +
// entity_types + relevance composition) → delete. Also covers the enqueue
// endpoints (crawl + reprobe) via a miniredis-backed Asynq client and the
// not-crawlable/disabled guards.
func TestAdminChannelCRUD_LiveDB(t *testing.T) {
	h, svc, inspector, cleanup := newLiveChannelHandlers(t)
	defer cleanup()

	ctx := t.Context()
	sourceID := createChannelTestSource(t, svc)

	defer func() { _ = svc.Delete(ctx, sourceID) }()

	// --- create ---
	createBody := `{
		"label": "Jobs RSS",
		"entry_url": "https://example.test/jobs.xml",
		"crawl_method": "rss",
		"entity_types": ["job"],
		"pipeline": "curate",
		"crawl_frequency_mins": 60,
		"relevance_mode": "filtered"
	}`
	rr := httptest.NewRecorder()
	r := authedChannelRequest(t, http.MethodPost, "/", sourceID.String(), createBody, true)
	h.HandleAdminCreateChannel(rr, r)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201 (body=%s)", rr.Code, rr.Body.String())
	}

	created := decodeChannelData(t, rr)

	channelID, _ := created["id"].(string)
	if channelID == "" {
		t.Fatalf("create: no id in response (body=%s)", rr.Body.String())
	}

	defer func() { _ = svc.DeleteChannel(ctx, mustUUID(t, channelID)) }()

	if created["pipeline"] != "curate" {
		t.Errorf("pipeline = %v, want curate", created["pipeline"])
	}

	if created["origin"] != "admin" {
		t.Errorf("origin = %v, want admin", created["origin"])
	}

	if created["enabled"] != true {
		t.Errorf("enabled = %v, want true", created["enabled"])
	}

	// --- list ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodGet, "/", sourceID.String(), "", true)
	h.HandleAdminListSourceChannels(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("list: status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	var listEnv struct {
		Data []map[string]any `json:"data"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&listEnv); err != nil {
		t.Fatalf("list: decode: %v", err)
	}

	found := false

	for _, c := range listEnv.Data {
		if c["id"] == channelID {
			found = true
		}
	}

	if !found {
		t.Errorf("list: channel %s not in listing (body=%s)", channelID, rr.Body.String())
	}

	// --- get ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodGet, "/", channelID, "", true)
	h.HandleAdminGetChannel(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	got := decodeChannelData(t, rr)
	if got["label"] != "Jobs RSS" {
		t.Errorf("get: label = %v, want Jobs RSS", got["label"])
	}

	if got["source_id"] != sourceID.String() {
		t.Errorf("get: source_id = %v, want %s", got["source_id"], sourceID.String())
	}

	// --- patch: scalar + entity_types + relevance in one request ---
	patchBody := `{
		"label": "Jobs RSS (renamed)",
		"entity_types": [],
		"relevance_mode": "custom",
		"relevance_topics": ["japa", "visa"],
		"relevance_url_patterns": ["/jobs/*"]
	}`
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPatch, "/", channelID, patchBody, true)
	h.HandleAdminUpdateChannel(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("patch: status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	patched := decodeChannelData(t, rr)
	if patched["label"] != "Jobs RSS (renamed)" {
		t.Errorf("patch: label = %v", patched["label"])
	}

	if ets, ok := patched["entity_types"].([]any); !ok || len(ets) != 0 {
		t.Errorf("patch: entity_types = %v, want []", patched["entity_types"])
	}

	if patched["relevance_mode"] != "custom" {
		t.Errorf("patch: relevance_mode = %v, want custom", patched["relevance_mode"])
	}

	if topics, ok := patched["relevance_topics"].([]any); !ok || len(topics) != 2 {
		t.Errorf("patch: relevance_topics = %v", patched["relevance_topics"])
	}

	// entity_types cleared + pipeline still 'curate' is allowed by direct
	// assignment — the runtime falls back to source-level types (service
	// contract). Switching pipeline via scalar keeps entity_types check
	// satisfied only when types exist; curate→extract is also fine.

	// --- crawl enqueue: enabled rss channel → 202 + pending task ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPost, "/", channelID, "", true)
	h.HandleAdminCrawlChannel(rr, r)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("crawl: status = %d, want 202 (body=%s)", rr.Code, rr.Body.String())
	}

	tasks, err := inspector.ListPendingTasks("default", asynq.PageSize(10))
	if err != nil {
		t.Fatalf("crawl: list pending tasks: %v", err)
	}

	var crawlTask *asynq.TaskInfo

	for _, task := range tasks {
		if task.Type == TaskIntelFetchChannel {
			crawlTask = task
		}
	}

	if crawlTask == nil {
		t.Fatalf("crawl: no %s task pending", TaskIntelFetchChannel)
	}

	var crawlPayload map[string]string
	if err := json.Unmarshal(crawlTask.Payload, &crawlPayload); err != nil {
		t.Fatalf("crawl: unmarshal payload: %v", err)
	}

	if crawlPayload["channel_id"] != channelID {
		t.Errorf("crawl payload channel_id = %v, want %s", crawlPayload["channel_id"], channelID)
	}

	if crawlPayload["source_id"] != sourceID.String() {
		t.Errorf("crawl payload source_id = %v, want %s", crawlPayload["source_id"], sourceID.String())
	}

	// --- crawl guards: disabled channel → 409 ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPatch, "/", channelID, `{"enabled": false}`, true)
	h.HandleAdminUpdateChannel(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("disable patch: status = %d (body=%s)", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPost, "/", channelID, "", true)
	h.HandleAdminCrawlChannel(rr, r)
	assertErrorCode(t, rr, http.StatusConflict, "channel_disabled")

	// --- crawl guards: manual channel → 409 not_crawlable ---
	manualBody := `{"label":"Manual","entry_url":"https://example.test/m","crawl_method":"manual","pipeline":"curate","entity_types":["job"]}`
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPost, "/", sourceID.String(), manualBody, true)
	h.HandleAdminCreateChannel(rr, r)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create manual: status = %d (body=%s)", rr.Code, rr.Body.String())
	}

	manualID, _ := decodeChannelData(t, rr)["id"].(string)

	defer func() { _ = svc.DeleteChannel(ctx, mustUUID(t, manualID)) }()

	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPost, "/", manualID, "", true)
	h.HandleAdminCrawlChannel(rr, r)
	assertErrorCode(t, rr, http.StatusConflict, "channel_not_crawlable")

	// --- reprobe enqueue → 202 + intel:probe_verify pending ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPost, "/", sourceID.String(), "", true)
	h.HandleAdminReprobeSource(rr, r)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("reprobe: status = %d, want 202 (body=%s)", rr.Code, rr.Body.String())
	}

	tasks, err = inspector.ListPendingTasks("default", asynq.PageSize(10))
	if err != nil {
		t.Fatalf("reprobe: list pending tasks: %v", err)
	}

	var probeTask *asynq.TaskInfo

	for _, task := range tasks {
		if task.Type == TaskIntelProbeVerify {
			probeTask = task
		}
	}

	if probeTask == nil {
		t.Fatalf("reprobe: no %s task pending", TaskIntelProbeVerify)
	}

	var probePayload map[string]string
	if err := json.Unmarshal(probeTask.Payload, &probePayload); err != nil {
		t.Fatalf("reprobe: unmarshal payload: %v", err)
	}

	if probePayload["source_id"] != sourceID.String() {
		t.Errorf("reprobe payload source_id = %v, want %s", probePayload["source_id"], sourceID.String())
	}

	// --- delete ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodDelete, "/", channelID, "", true)
	h.HandleAdminDeleteChannel(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("delete: status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodGet, "/", channelID, "", true)
	h.HandleAdminGetChannel(rr, r)
	assertErrorCode(t, rr, http.StatusNotFound, "not_found")
}

// TestAdminChannelHandlers_RBACDenied verifies each new endpoint's RBAC
// node denies an authenticated user holding no permissions. RBAC is enforced
// by route middleware, so the test wraps each handler in
// rbac.RequirePermission with the node the route registration uses — a live
// resolver (backed by the test DB) fails closed for an unknown user.
func TestAdminChannelHandlers_RBACDenied(t *testing.T) {
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping live Postgres test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Skipf("test database unreachable: %v", err)
	}
	defer database.Close()

	resolver := rbac.New(database, nil)

	client, _ := newTestAsynqClient(t)
	h := &Handlers{intelSources: sources.NewService(nil), asynqClient: client}

	for _, tc := range []struct {
		name    string
		node    string
		handler http.HandlerFunc
	}{
		{"list", "intel.channels.list", h.HandleAdminListSourceChannels},
		{"create", "intel.channels.create", h.HandleAdminCreateChannel},
		{"view", "intel.channels.view", h.HandleAdminGetChannel},
		{"update", "intel.channels.update", h.HandleAdminUpdateChannel},
		{"delete", "intel.channels.delete", h.HandleAdminDeleteChannel},
		{"crawl", "intel.channels.crawl", h.HandleAdminCrawlChannel},
		{"reprobe", "intel.sources.probe", h.HandleAdminReprobeSource},
	} {
		t.Run(tc.node, func(t *testing.T) {
			wrapped := rbac.RequirePermission(resolver, tc.node)(tc.handler)

			rr := httptest.NewRecorder()
			r := authedChannelRequest(t, http.MethodGet, "/", testUUID, "", true)
			wrapped.ServeHTTP(rr, r)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("%s: status = %d, want 403 (body=%s)", tc.node, rr.Code, rr.Body.String())
			}
		})
	}
}
