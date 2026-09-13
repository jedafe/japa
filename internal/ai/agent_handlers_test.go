// Package ai — agent_handlers_test.go covers the Phase 6 HTTP handlers
// (Section 103, ADR-111).
//
// Tests verify:
//   - SSE streaming: headers, event format, channel drain.
//   - Capabilities: RBAC-filtered catalog + geo context.
//   - Session history: pagination, tenant/user scoping.
//   - Session detail: ownership enforcement, tool call inclusion.
//   - Session cancellation: ownership, already-completed conflict, Valkey flag.
//   - Authorship reassignment: provenance merge, ownership, UUID validation.
//   - AI privacy GET/PUT: entity types, ownership enforcement.
//   - Admin routes: session list/detail, tool stats, cost report.
//   - Error format: {error:{code,message}} per Rule 5.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (API Routes)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6, tasks 6.01-6.10)
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// ---------------------------------------------------------------------------
// Test stubs — AgentSessionQueries + ContentAuthorQueries implementations
// ---------------------------------------------------------------------------

// stubAgentSessions implements AgentSessionQueries for handler tests.
type stubAgentSessions struct {
	// Session storage keyed by ID.
	sessions  map[string]gen.AiAgentSession
	toolCalls map[string][]gen.AiAgentToolCall
	// Privacy flags.
	userExempt    bool
	contentExempt bool
	cfvExempt     bool
	// CFV entity ownership lookup (keyed by CFV ID).
	cfvEntities map[string]gen.GetCustomFieldValueEntityRow
	// Admin aggregates.
	toolStats  []gen.GetAgentToolStatsRow
	costReport []gen.GetAgentCostReportRow
	// Tracking.
	cancelledID *pgtype.UUID
	cancelRace  bool // if true, CancelAgentSession returns ErrNoRows even for running sessions
}

func newStubAgentSessions() *stubAgentSessions {
	return &stubAgentSessions{
		sessions:    make(map[string]gen.AiAgentSession),
		toolCalls:   make(map[string][]gen.AiAgentToolCall),
		cfvEntities: make(map[string]gen.GetCustomFieldValueEntityRow),
	}
}

func (s *stubAgentSessions) GetAgentSessionForUser(_ context.Context, arg gen.GetAgentSessionForUserParams) (gen.AiAgentSession, error) {
	sess, ok := s.sessions[arg.ID.String()]
	if !ok || sess.UserID != arg.UserID {
		return gen.AiAgentSession{}, pgx.ErrNoRows
	}

	return sess, nil
}

func (s *stubAgentSessions) ListAgentSessionsByUser(_ context.Context, arg gen.ListAgentSessionsByUserParams) ([]gen.AiAgentSession, error) {
	var out []gen.AiAgentSession

	for _, sess := range s.sessions {
		if sess.UserID == arg.UserID {
			out = append(out, sess)
		}
	}

	return out, nil
}

func (s *stubAgentSessions) CountAgentSessionsByUser(_ context.Context, userID pgtype.UUID) (int64, error) {
	rows, _ := s.ListAgentSessionsByUser(context.Background(), gen.ListAgentSessionsByUserParams{

		UserID: userID,
	})

	return int64(len(rows)), nil
}

func (s *stubAgentSessions) ListAgentToolCallsBySession(_ context.Context, sessionID pgtype.UUID) ([]gen.AiAgentToolCall, error) {
	return s.toolCalls[sessionID.String()], nil
}

func (s *stubAgentSessions) CancelAgentSession(_ context.Context, id pgtype.UUID) (gen.AiAgentSession, error) {
	sess, ok := s.sessions[id.String()]
	if !ok {
		return gen.AiAgentSession{}, pgx.ErrNoRows
	}

	if s.cancelRace {
		return gen.AiAgentSession{}, pgx.ErrNoRows
	}

	if sess.Status != "running" {
		return gen.AiAgentSession{}, pgx.ErrNoRows
	}

	sess.Status = "cancelled"
	s.sessions[id.String()] = sess
	s.cancelledID = &id

	return sess, nil
}

func (s *stubAgentSessions) GetAgentSession(_ context.Context, id pgtype.UUID) (gen.AiAgentSession, error) {
	sess, ok := s.sessions[id.String()]
	if !ok {
		return gen.AiAgentSession{}, pgx.ErrNoRows
	}

	return sess, nil
}

func (s *stubAgentSessions) ListAgentSessionsForAdmin(_ context.Context, arg gen.ListAgentSessionsForAdminParams) ([]gen.AiAgentSession, error) {
	var out []gen.AiAgentSession

	for _, sess := range s.sessions {
				if arg.StatusFilter != "" && sess.Status != arg.StatusFilter {
			continue
		}

		out = append(out, sess)
	}

	return out, nil
}

func (s *stubAgentSessions) CountAgentSessionsForAdmin(_ context.Context, statusFilter string) (int64, error) {
	rows, _ := s.ListAgentSessionsForAdmin(context.Background(), gen.ListAgentSessionsForAdminParams{

		StatusFilter: statusFilter,
	})

	return int64(len(rows)), nil
}

func (s *stubAgentSessions) GetAgentToolStats(_ context.Context, _ gen.GetAgentToolStatsParams) ([]gen.GetAgentToolStatsRow, error) {
	return s.toolStats, nil
}

func (s *stubAgentSessions) GetAgentCostReport(_ context.Context, _ gen.GetAgentCostReportParams) ([]gen.GetAgentCostReportRow, error) {
	return s.costReport, nil
}

func (s *stubAgentSessions) GetContentAIExempt(_ context.Context, id pgtype.UUID) (bool, error) {
	_ = id
	return s.contentExempt, nil
}

func (s *stubAgentSessions) GetCustomFieldValueAIExempt(_ context.Context, id pgtype.UUID) (bool, error) {
	_ = id
	return s.cfvExempt, nil
}

func (s *stubAgentSessions) GetCustomFieldValueEntity(_ context.Context, id pgtype.UUID) (gen.GetCustomFieldValueEntityRow, error) {
	if row, ok := s.cfvEntities[id.String()]; ok {
		return row, nil
	}

	return gen.GetCustomFieldValueEntityRow{}, pgx.ErrNoRows
}

func (s *stubAgentSessions) GetUserAIExempt(_ context.Context, id pgtype.UUID) (bool, error) {
	_ = id
	return s.userExempt, nil
}

func (s *stubAgentSessions) SetContentAIExempt(_ context.Context, arg gen.SetContentAIExemptParams) (gen.SetContentAIExemptRow, error) {
	return gen.SetContentAIExemptRow{ID: arg.ID, AiExempt: arg.AiExempt}, nil
}

func (s *stubAgentSessions) SetCustomFieldValueAIExempt(_ context.Context, arg gen.SetCustomFieldValueAIExemptParams) (gen.SetCustomFieldValueAIExemptRow, error) {
	return gen.SetCustomFieldValueAIExemptRow{ID: arg.ID, FieldID: pgtype.UUID{}, AiExempt: arg.AiExempt}, nil
}

func (s *stubAgentSessions) SetUserAIExempt(_ context.Context, arg gen.SetUserAIExemptParams) (gen.SetUserAIExemptRow, error) {
	return gen.SetUserAIExemptRow{ID: arg.ID, AiExempt: arg.AiExempt}, nil
}

// stubContentStore implements ContentAuthorQueries for handler tests.
type stubContentStore struct {
	contents map[string]gen.GetContentByIDRow
	users    map[string]gen.GetUserByIDRow // keyed by user ID string
}

func newStubContentStore() *stubContentStore {
	return &stubContentStore{
		contents: make(map[string]gen.GetContentByIDRow),
		users:    make(map[string]gen.GetUserByIDRow),
	}
}

func (s *stubContentStore) GetContentByID(_ context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error) {
	row, ok := s.contents[id.String()]
	if !ok {
		return gen.GetContentByIDRow{}, pgx.ErrNoRows
	}

	return row, nil
}

func (s *stubContentStore) GetUserByID(_ context.Context, userID pgtype.UUID) (gen.GetUserByIDRow, error) {
	row, ok := s.users[userID.String()]
	if !ok {
		return gen.GetUserByIDRow{}, pgx.ErrNoRows
	}

	return row, nil
}

func (s *stubContentStore) ReassignSingleContentAuthor(_ context.Context, arg gen.ReassignSingleContentAuthorParams) (gen.ReassignSingleContentAuthorRow, error) {
	row, ok := s.contents[arg.ID.String()]
	if !ok {
		return gen.ReassignSingleContentAuthorRow{}, pgx.ErrNoRows
	}

	row.AuthorID = arg.AuthorID
	row.FieldValues = arg.FieldValues
	s.contents[arg.ID.String()] = row

	return gen.ReassignSingleContentAuthorRow{
		ID: row.ID,

		AuthorID: row.AuthorID,
	}, nil
}

// stubCache implements CacheClient for handler tests.
type stubCache struct {
	values map[string]string
}

func newStubCache() *stubCache {
	return &stubCache{values: make(map[string]string)}
}

func (c *stubCache) Get(_ context.Context, key string) (string, error) {
	v, ok := c.values[key]
	if !ok {
		return "", pgx.ErrNoRows
	}

	return v, nil
}

func (c *stubCache) Set(_ context.Context, key, value string, _ int) error {
	c.values[key] = value
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testUserAndTenant creates a test user + tenant and returns them.
func testUserAndTenant() (*auth.AuthenticatedUser, *tenant.Tenant) {
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	tid := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	return &auth.AuthenticatedUser{UserID: uid, SessionID: "test-jti"},
		&tenant.Tenant{ID: tid, Name: "Test", Slug: "test"}
}

// withAuthContext returns a request with the user + tenant injected into
// the context, simulating what RequireAuth + tenant middleware do.
func withAuthContext(r *http.Request, user *auth.AuthenticatedUser, t *tenant.Tenant) *http.Request {
	ctx := context.WithValue(r.Context(), auth.UserContextKey, user)
	ctx = context.WithValue(ctx, tenant.TenantContextKey, t)

	return r.WithContext(ctx)
}

// newTestAgentHTTPHandlers builds a handler set with stub deps for testing.
func newTestAgentHTTPHandlers(t *testing.T) (*AgentHTTPHandlers, *stubAgentSessions, *stubContentStore, *stubCache) {
	t.Helper()

	provider := &mockAgentProvider{name: "mock"}
	agent := newTestAgentHandler(t, provider)
	sessions := newStubAgentSessions()
	content := newStubContentStore()
	cache := newStubCache()
	h := NewAgentHTTPHandlers(
		agent, sessions,
		WithAgentContentStore(content),
		WithAgentHTTPCache(cache),
	)

	return h, sessions, content, cache
}

// decodeError extracts the error object from a JSON response body.
func decodeError(t *testing.T, body io.Reader) map[string]string {
	t.Helper()

	var wrapper struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(body).Decode(&wrapper); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}

	return map[string]string{"code": wrapper.Error.Code, "message": wrapper.Error.Message}
}

// ---------------------------------------------------------------------------
// Task 6.01 — SSE streaming handler
// ---------------------------------------------------------------------------

func TestHandleAgent_NoAuth(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)

	req := httptest.NewRequest(http.MethodPost, "/ai/agent", bytes.NewBufferString(`{"prompt":"hi"}`))
	w := httptest.NewRecorder()
	h.HandleAgent(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "unauthorized" {
		t.Errorf("error code = %q, want %q", err["code"], "unauthorized")
	}
}

func TestHandleAgent_EmptyPrompt(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	req := httptest.NewRequest(http.MethodPost, "/ai/agent", bytes.NewBufferString(`{"prompt":""}`))
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAgent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "validation_error" {
		t.Errorf("error code = %q, want %q", err["code"], "validation_error")
	}
}

func TestHandleAgent_InvalidJSON(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	req := httptest.NewRequest(http.MethodPost, "/ai/agent", bytes.NewBufferString(`{bad`))
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAgent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAgent_SSEHeaders(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	req := httptest.NewRequest(http.MethodPost, "/ai/agent", bytes.NewBufferString(`{"prompt":"hello"}`))
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAgent(w, req)

	// The agent loop runs in a goroutine and emits events. The SSE headers
	// must be set regardless of whether events were produced.
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", cc, "no-cache")
	}

	if ka := w.Header().Get("Connection"); ka != "keep-alive" {
		t.Errorf("Connection = %q, want %q", ka, "keep-alive")
	}
}

func TestHandleAgent_SSEEventFormat(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	req := httptest.NewRequest(http.MethodPost, "/ai/agent", bytes.NewBufferString(`{"prompt":"hello"}`))
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAgent(w, req)

	body := w.Body.String()
	if body == "" {
		t.Fatal("expected SSE output, got empty body")
	}
	// Each SSE line must start with "data: " and end with "\n\n".
	for _, line := range strings.Split(strings.TrimSpace(body), "\n\n") {
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			t.Errorf("SSE line does not start with 'data: ': %q", line)
		}
		// The JSON payload after "data: " must be valid JSON.
		payload := strings.TrimPrefix(line, "data: ")

		var ev AgentEvent

		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			t.Errorf("SSE payload is not valid JSON: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 6.03 — Session history pagination
// ---------------------------------------------------------------------------

func TestHandleListSessions_Pagination(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	// Seed 3 sessions for the user.
	for range 3 {
		id := randomUUID()
		sessions.sessions[id.String()] = gen.AiAgentSession{
			ID: id,

			UserID: user.UserID,
			Prompt: "test",
			Status: "completed",
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/ai/agent/sessions?page_size=2&page_offset=0", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}

	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Meta.Total != 3 {
		t.Errorf("total = %d, want 3", resp.Meta.Total)
	}
}

func TestHandleListSessions_NoAuth(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/ai/agent/sessions", nil)
	w := httptest.NewRecorder()
	h.HandleListSessions(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListSessions_UserScoping(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()
	otherUser := randomUUID()

	// Seed a session for a different user.
	otherID := randomUUID()
	sessions.sessions[otherID.String()] = gen.AiAgentSession{
		ID: otherID,

		UserID: otherUser,
		Prompt: "other",
	}

	// Seed a session for the current user.
	myID := randomUUID()
	sessions.sessions[myID.String()] = gen.AiAgentSession{
		ID: myID,

		UserID: user.UserID,
		Prompt: "mine",
	}

	req := httptest.NewRequest(http.MethodGet, "/ai/agent/sessions", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleListSessions(w, req)

	var resp struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}

	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Meta.Total != 1 {
		t.Errorf("total = %d, want 1 (user scoping failed)", resp.Meta.Total)
	}
}

// ---------------------------------------------------------------------------
// Task 6.04 — Session detail + tool calls
// ---------------------------------------------------------------------------

func TestHandleGetSession_NotFound(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	// Use chi router to set URL params.
	r := chi.NewRouter()
	r.Get("/ai/agent/sessions/{id}", h.HandleGetSession)

	req := httptest.NewRequest(http.MethodGet, "/ai/agent/sessions/"+randomUUID().String(), nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "session_not_found" {
		t.Errorf("error code = %q, want %q", err["code"], "session_not_found")
	}
}

func TestHandleGetSession_WithToolCalls(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	sessID := randomUUID()
	sessions.sessions[sessID.String()] = gen.AiAgentSession{
		ID: sessID,

		UserID: user.UserID,
		Prompt: "test",
		Status: "completed",
	}
	sessions.toolCalls[sessID.String()] = []gen.AiAgentToolCall{
		{ID: randomUUID(), SessionID: sessID, ToolName: "content.search", Status: "success", Arguments: []byte(`{"query":"go"}`), Result: []byte(`{"results":[]}`)},
	}

	r := chi.NewRouter()
	r.Get("/ai/agent/sessions/{id}", h.HandleGetSession)

	req := httptest.NewRequest(http.MethodGet, "/ai/agent/sessions/"+sessID.String(), nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data struct {
			Session   map[string]any   `json:"session"`
			ToolCalls []map[string]any `json:"tool_calls"`
		} `json:"data"`
	}

	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Data.ToolCalls) != 1 {
		t.Errorf("tool_calls count = %d, want 1", len(resp.Data.ToolCalls))
	}

	if resp.Data.ToolCalls[0]["tool_name"] != "content.search" {
		t.Errorf("tool_name = %v, want content.search", resp.Data.ToolCalls[0]["tool_name"])
	}
}

func TestHandleGetSession_OwnershipEnforced(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	// A session owned by a different user.
	otherUID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	sessID := randomUUID()
	sessions.sessions[sessID.String()] = gen.AiAgentSession{
		ID: sessID,

		UserID: otherUID,
		Prompt: "not yours",
	}

	r := chi.NewRouter()
	r.Get("/ai/agent/sessions/{id}", h.HandleGetSession)

	req := httptest.NewRequest(http.MethodGet, "/ai/agent/sessions/"+sessID.String(), nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (ownership not enforced)", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetSession_InvalidUUID(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	r := chi.NewRouter()
	r.Get("/ai/agent/sessions/{id}", h.HandleGetSession)

	req := httptest.NewRequest(http.MethodGet, "/ai/agent/sessions/not-a-uuid", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "validation_error" {
		t.Errorf("error code = %q, want %q", err["code"], "validation_error")
	}
}

// ---------------------------------------------------------------------------
// Task 6.05 — Session cancellation
// ---------------------------------------------------------------------------

func TestHandleCancelSession_Success(t *testing.T) {
	t.Parallel()
	h, sessions, _, cache := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	sessID := randomUUID()
	sessions.sessions[sessID.String()] = gen.AiAgentSession{
		ID: sessID,

		UserID: user.UserID,
		Status: "running",
	}

	r := chi.NewRouter()
	r.Post("/ai/agent/sessions/{id}/cancel", h.HandleCancelSession)

	req := httptest.NewRequest(http.MethodPost, "/ai/agent/sessions/"+sessID.String()+"/cancel", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	// Verify the Valkey cancel flag was set.
	flagKey := "jc:agent:session:" + tn.ID.String() + ":" + sessID.String()
	if v, _ := cache.Get(t.Context(), flagKey); v != "cancelled" {
		t.Errorf("Valkey cancel flag = %q, want %q", v, "cancelled")
	}
}

func TestHandleCancelSession_AlreadyCompleted(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	sessID := randomUUID()
	sessions.sessions[sessID.String()] = gen.AiAgentSession{
		ID: sessID,

		UserID: user.UserID,
		Status: "completed",
	}

	r := chi.NewRouter()
	r.Post("/ai/agent/sessions/{id}/cancel", h.HandleCancelSession)

	req := httptest.NewRequest(http.MethodPost, "/ai/agent/sessions/"+sessID.String()+"/cancel", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "session_not_running" {
		t.Errorf("error code = %q, want %q", err["code"], "session_not_running")
	}
}

func TestHandleCancelSession_NotOwner(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	otherUID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	sessID := randomUUID()
	sessions.sessions[sessID.String()] = gen.AiAgentSession{
		ID: sessID,

		UserID: otherUID,
		Status: "running",
	}

	r := chi.NewRouter()
	r.Post("/ai/agent/sessions/{id}/cancel", h.HandleCancelSession)

	req := httptest.NewRequest(http.MethodPost, "/ai/agent/sessions/"+sessID.String()+"/cancel", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not owner)", w.Code, http.StatusNotFound)
	}
}

// TestHandleCancelSession_RaceCompleted simulates the race where the
// session completes between the ownership check and the CancelAgentSession
// DB call. The ownership check sees status="running" (passes), but
// CancelAgentSession returns ErrNoRows (the row's status changed between
// the two calls). The handler should return 409.
func TestHandleCancelSession_RaceCompleted(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	sessID := randomUUID()
	sessions.sessions[sessID.String()] = gen.AiAgentSession{
		ID: sessID,

		UserID: user.UserID,
		Status: "running",
	}
	// Set the race flag: CancelAgentSession will return ErrNoRows even
	// though the session appears "running" to GetAgentSessionForUser.
	sessions.cancelRace = true

	r := chi.NewRouter()
	r.Post("/ai/agent/sessions/{id}/cancel", h.HandleCancelSession)

	req := httptest.NewRequest(http.MethodPost, "/ai/agent/sessions/"+sessID.String()+"/cancel", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (race completed)", w.Code, http.StatusConflict)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "session_not_running" {
		t.Errorf("error code = %q, want %q", err["code"], "session_not_running")
	}
}

// ---------------------------------------------------------------------------
// Task 6.06 — Authorship reassignment
// ---------------------------------------------------------------------------

func TestHandleReassignAuthor_Success(t *testing.T) {
	t.Parallel()
	h, _, content, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	contentID := randomUUID()
	content.contents[contentID.String()] = gen.GetContentByIDRow{
		ID: contentID,

		AuthorID:    user.UserID,
		FieldValues: []byte(`{"title":"Test"}`),
	}

	newAuthorID := randomUUID()
	// Seed the target author in the user store so the validity check passes.
	content.users[newAuthorID.String()] = gen.GetUserByIDRow{
		ID:       newAuthorID,
		Username: "newauthor",
	}

	r := chi.NewRouter()
	r.Post("/content/{id}/reassign-author", h.HandleReassignAuthor)

	body := bytes.NewBufferString(`{"new_author_id":"` + newAuthorID.String() + `","reason":"AI reassignment"}`)
	req := httptest.NewRequest(http.MethodPost, "/content/"+contentID.String()+"/reassign-author", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify the author was updated.
	updated := content.contents[contentID.String()]
	if updated.AuthorID != newAuthorID {
		t.Errorf("author_id = %v, want %v", updated.AuthorID, newAuthorID)
	}

	// Verify provenance was merged into field_values.
	var fv map[string]any

	json.Unmarshal(updated.FieldValues, &fv)

	prov, ok := fv["_ai_provenance"].(map[string]any)
	if !ok {
		t.Fatal("missing _ai_provenance in field_values")
	}

	if prov["author_assigned_by"] != user.UserID.String() {
		t.Errorf("author_assigned_by = %v, want %v", prov["author_assigned_by"], user.UserID.String())
	}

	if prov["author_assignment_reason"] != "AI reassignment" {
		t.Errorf("author_assignment_reason = %v, want %q", prov["author_assignment_reason"], "AI reassignment")
	}
}

func TestHandleReassignAuthor_ContentNotFound(t *testing.T) {
	t.Parallel()
	h, _, content, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	newAuthorID := randomUUID()
	// Seed the author so the validity check passes; the content lookup
	// should then fail with 404.
	content.users[newAuthorID.String()] = gen.GetUserByIDRow{
		ID:       newAuthorID,
		Username: "newauthor",
	}

	r := chi.NewRouter()
	r.Post("/content/{id}/reassign-author", h.HandleReassignAuthor)

	body := bytes.NewBufferString(`{"new_author_id":"` + newAuthorID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/content/"+randomUUID().String()+"/reassign-author", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "content_not_found" {
		t.Errorf("error code = %q, want %q", err["code"], "content_not_found")
	}
}

func TestHandleReassignAuthor_InvalidAuthor(t *testing.T) {
	t.Parallel()
	h, _, content, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	contentID := randomUUID()
	content.contents[contentID.String()] = gen.GetContentByIDRow{
		ID: contentID,

		AuthorID: user.UserID,
	}
	// Do NOT seed the new author — the validity check should fail.
	newAuthorID := randomUUID()

	r := chi.NewRouter()
	r.Post("/content/{id}/reassign-author", h.HandleReassignAuthor)

	body := bytes.NewBufferString(`{"new_author_id":"` + newAuthorID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/content/"+contentID.String()+"/reassign-author", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "invalid_author" {
		t.Errorf("error code = %q, want %q", err["code"], "invalid_author")
	}
}

func TestHandleReassignAuthor_MissingNewAuthorID(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	r := chi.NewRouter()
	r.Post("/content/{id}/reassign-author", h.HandleReassignAuthor)

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/content/"+randomUUID().String()+"/reassign-author", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleReassignAuthor_InvalidUUID(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	r := chi.NewRouter()
	r.Post("/content/{id}/reassign-author", h.HandleReassignAuthor)

	body := bytes.NewBufferString(`{"new_author_id":"not-a-uuid"}`)
	req := httptest.NewRequest(http.MethodPost, "/content/"+randomUUID().String()+"/reassign-author", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMergeAuthorProvenance_EmptyFieldValues(t *testing.T) {
	t.Parallel()

	userID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	out, err := mergeAuthorProvenance(nil, userID, "test reason")
	if err != nil {
		t.Fatalf("mergeAuthorProvenance: %v", err)
	}

	var fv map[string]any
	if err := json.Unmarshal(out, &fv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	prov, ok := fv["_ai_provenance"].(map[string]any)
	if !ok {
		t.Fatal("missing _ai_provenance")
	}

	if prov["author_assigned_by"] != userID.String() {
		t.Errorf("author_assigned_by = %v, want %v", prov["author_assigned_by"], userID.String())
	}

	if prov["author_assignment_reason"] != "test reason" {
		t.Errorf("reason = %v, want %q", prov["author_assignment_reason"], "test reason")
	}
}

func TestMergeAuthorProvenance_PreservesExistingFields(t *testing.T) {
	t.Parallel()

	userID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	existing := []byte(`{"title":"My Post","_ai_provenance":{"ai_generated":true}}`)

	out, err := mergeAuthorProvenance(existing, userID, "")
	if err != nil {
		t.Fatalf("mergeAuthorProvenance: %v", err)
	}

	var fv map[string]any

	json.Unmarshal(out, &fv)

	if fv["title"] != "My Post" {
		t.Errorf("title = %v, want %q", fv["title"], "My Post")
	}

	prov := fv["_ai_provenance"].(map[string]any)
	if prov["ai_generated"] != true {
		t.Error("existing ai_generated flag was overwritten")
	}

	if prov["author_assigned_by"] != userID.String() {
		t.Errorf("author_assigned_by = %v, want %v", prov["author_assigned_by"], userID.String())
	}
}

// ---------------------------------------------------------------------------
// Task 6.07 — AI privacy GET/PUT
// ---------------------------------------------------------------------------

func TestHandleGetAIPrivacy_Success(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()
	sessions.userExempt = true

	req := httptest.NewRequest(http.MethodGet, "/me/ai-privacy", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleGetAIPrivacy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data struct {
			ProfileAIExempt bool `json:"profile_ai_exempt"`
		} `json:"data"`
	}

	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.Data.ProfileAIExempt {
		t.Error("profile_ai_exempt = false, want true")
	}
}

func TestHandleSetAIPrivacy_Profile(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	body := bytes.NewBufferString(`{"entity_type":"profile","entity_id":"` + user.UserID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleSetAIPrivacy_ProfileNotOwner(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()
	otherID := randomUUID()

	body := bytes.NewBufferString(`{"entity_type":"profile","entity_id":"` + otherID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "not_owner" {
		t.Errorf("error code = %q, want %q", err["code"], "not_owner")
	}
}

func TestHandleSetAIPrivacy_ContentNotOwner(t *testing.T) {
	t.Parallel()
	h, _, content, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()
	otherUID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}

	contentID := randomUUID()
	content.contents[contentID.String()] = gen.GetContentByIDRow{
		ID: contentID,

		AuthorID: otherUID, // owned by someone else
	}

	body := bytes.NewBufferString(`{"entity_type":"content","entity_id":"` + contentID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleSetAIPrivacy_CFV_UserEntityOwner(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	cfvID := randomUUID()
	sessions.cfvEntities[cfvID.String()] = gen.GetCustomFieldValueEntityRow{
		EntityType: "user",
		EntityID:   user.UserID, // owned by the user
	}

	body := bytes.NewBufferString(`{"entity_type":"custom_field_value","entity_id":"` + cfvID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleSetAIPrivacy_CFV_UserEntityNotOwner(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()
	otherUID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}

	cfvID := randomUUID()
	sessions.cfvEntities[cfvID.String()] = gen.GetCustomFieldValueEntityRow{
		EntityType: "user",
		EntityID:   otherUID, // owned by someone else
	}

	body := bytes.NewBufferString(`{"entity_type":"custom_field_value","entity_id":"` + cfvID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (not owner)", w.Code, http.StatusForbidden)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "not_owner" {
		t.Errorf("error code = %q, want %q", err["code"], "not_owner")
	}
}

func TestHandleSetAIPrivacy_CFV_ContentEntityOwner(t *testing.T) {
	t.Parallel()
	h, sessions, content, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	contentID := randomUUID()
	content.contents[contentID.String()] = gen.GetContentByIDRow{
		ID: contentID,

		AuthorID: user.UserID,
	}

	cfvID := randomUUID()
	sessions.cfvEntities[cfvID.String()] = gen.GetCustomFieldValueEntityRow{
		EntityType: "content",
		EntityID:   contentID,
	}

	body := bytes.NewBufferString(`{"entity_type":"custom_field_value","entity_id":"` + cfvID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleSetAIPrivacy_CFV_ContentEntityNotOwner(t *testing.T) {
	t.Parallel()
	h, sessions, content, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()
	otherUID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}

	contentID := randomUUID()
	content.contents[contentID.String()] = gen.GetContentByIDRow{
		ID: contentID,

		AuthorID: otherUID, // owned by someone else
	}

	cfvID := randomUUID()
	sessions.cfvEntities[cfvID.String()] = gen.GetCustomFieldValueEntityRow{
		EntityType: "content",
		EntityID:   contentID,
	}

	body := bytes.NewBufferString(`{"entity_type":"custom_field_value","entity_id":"` + cfvID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (not owner)", w.Code, http.StatusForbidden)
	}
}

func TestHandleSetAIPrivacy_CFV_NotFound(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	cfvID := randomUUID()
	// Don't seed cfvEntities — the lookup should return ErrNoRows.

	body := bytes.NewBufferString(`{"entity_type":"custom_field_value","entity_id":"` + cfvID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "field_value_not_found" {
		t.Errorf("error code = %q, want %q", err["code"], "field_value_not_found")
	}
}

func TestHandleSetAIPrivacy_CFV_UnknownEntityType(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	cfvID := randomUUID()
	sessions.cfvEntities[cfvID.String()] = gen.GetCustomFieldValueEntityRow{
		EntityType: "space", // unknown entity type — should be denied
		EntityID:   randomUUID(),
	}

	body := bytes.NewBufferString(`{"entity_type":"custom_field_value","entity_id":"` + cfvID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (unknown entity type should be denied)", w.Code, http.StatusForbidden)
	}
}

func TestHandleSetAIPrivacy_InvalidEntityType(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	body := bytes.NewBufferString(`{"entity_type":"invalid","entity_id":"` + user.UserID.String() + `","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	err := decodeError(t, w.Body)
	if err["code"] != "validation_error" {
		t.Errorf("error code = %q, want %q", err["code"], "validation_error")
	}
}

func TestHandleSetAIPrivacy_MissingEntityID(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	body := bytes.NewBufferString(`{"entity_type":"profile","ai_exempt":true}`)
	req := httptest.NewRequest(http.MethodPut, "/me/ai-privacy", body)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleSetAIPrivacy(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// Task 6.08 — Admin routes
// ---------------------------------------------------------------------------

func TestHandleAdminListSessions_Success(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	// Seed sessions for the tenant.
	for range 2 {
		id := randomUUID()
		sessions.sessions[id.String()] = gen.AiAgentSession{
			ID: id,

			UserID: user.UserID,
			Status: "completed",
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/agent/sessions", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAdminListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}

	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Meta.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Meta.Total)
	}
}

func TestHandleAdminListSessions_StatusFilter(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	sessions.sessions[randomUUID().String()] = gen.AiAgentSession{ID: randomUUID(), UserID: user.UserID, Status: "running"}
	sessions.sessions[randomUUID().String()] = gen.AiAgentSession{ID: randomUUID(), UserID: user.UserID, Status: "completed"}

	req := httptest.NewRequest(http.MethodGet, "/admin/agent/sessions?status=running", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAdminListSessions(w, req)

	var resp struct {
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}

	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Meta.Total != 1 {
		t.Errorf("total = %d, want 1 (status filter failed)", resp.Meta.Total)
	}
}

func TestHandleAdminGetSession_Success(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	sessID := randomUUID()
	sessions.sessions[sessID.String()] = gen.AiAgentSession{
		ID: sessID,

		UserID: user.UserID,
		Status: "completed",
	}
	sessions.toolCalls[sessID.String()] = []gen.AiAgentToolCall{
		{ID: randomUUID(), SessionID: sessID, ToolName: "rag.search", Status: "success"},
	}

	r := chi.NewRouter()
	r.Get("/admin/agent/sessions/{id}", h.HandleAdminGetSession)

	req := httptest.NewRequest(http.MethodGet, "/admin/agent/sessions/"+sessID.String(), nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data struct {
			ToolCalls []map[string]any `json:"tool_calls"`
		} `json:"data"`
	}

	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Data.ToolCalls) != 1 {
		t.Errorf("tool_calls = %d, want 1", len(resp.Data.ToolCalls))
	}
}

func TestHandleAdminGetSession_NotFound(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	r := chi.NewRouter()
	r.Get("/admin/agent/sessions/{id}", h.HandleAdminGetSession)

	req := httptest.NewRequest(http.MethodGet, "/admin/agent/sessions/"+randomUUID().String(), nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAdminToolStats_Success(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	sessions.toolStats = []gen.GetAgentToolStatsRow{
		{ToolName: "content.search", TotalCalls: 10, SuccessCount: 8, ErrorCount: 2},
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/agent/tool-stats", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAdminToolStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}

	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Data) != 1 {
		t.Fatalf("stats count = %d, want 1", len(resp.Data))
	}

	if resp.Data[0]["success_rate"] != 0.8 {
		t.Errorf("success_rate = %v, want 0.8", resp.Data[0]["success_rate"])
	}
}

func TestHandleAdminToolStats_DateRange(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	req := httptest.NewRequest(http.MethodGet, "/admin/agent/tool-stats?from=2024-01-01&to=2024-12-31", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAdminToolStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAdminToolStats_InvalidDateRange(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	req := httptest.NewRequest(http.MethodGet, "/admin/agent/tool-stats?from=not-a-date", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAdminToolStats(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAdminCostReport_Success(t *testing.T) {
	t.Parallel()
	h, sessions, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndTenant()

	username := "testuser"
	sessions.costReport = []gen.GetAgentCostReportRow{
		{UserID: user.UserID, Username: &username, SessionCount: 5, TotalTokens: int64(1000), TotalToolCalls: int64(20), TotalCostUsd: 0.15},
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/agent/cost-report", nil)
	req = withAuthContext(req, user, tn)
	w := httptest.NewRecorder()
	h.HandleAdminCostReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}

	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Data) != 1 {
		t.Fatalf("report rows = %d, want 1", len(resp.Data))
	}

	if resp.Data[0]["username"] != "testuser" {
		t.Errorf("username = %v, want testuser", resp.Data[0]["username"])
	}
}

// ---------------------------------------------------------------------------
// Utility helper tests
// ---------------------------------------------------------------------------

func TestParseURLUUID_Valid(t *testing.T) {
	t.Parallel()

	id := randomUUID()
	r := chi.NewRouter()

	var captured pgtype.UUID

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		var ok bool

		captured, ok = parseURLUUID(w, r, "id")
		if !ok {
			t.Error("parseURLUUID returned ok=false for valid UUID")
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !captured.Valid || captured.String() != id.String() {
		t.Errorf("captured = %v, want %v", captured, id)
	}
}

func TestParseURLUUID_Invalid(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, ok := parseURLUUID(w, r, "id")
		if ok {
			t.Error("parseURLUUID returned ok=true for invalid UUID")
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestParseDateRange_Valid(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/?from=2024-01-15&to=2024-06-30", nil)

	from, to, ok := parseDateRange(httptest.NewRecorder(), req)
	if !ok {
		t.Fatal("parseDateRange returned ok=false for valid dates")
	}

	if !from.Valid || from.Time.Year() != 2024 {
		t.Errorf("from = %v, want 2024-01-15", from)
	}

	if !to.Valid || to.Time.Year() != 2024 {
		t.Errorf("to = %v, want 2024-07-01", to)
	}
}

func TestParseDateRange_Empty(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	from, to, ok := parseDateRange(httptest.NewRecorder(), req)
	if !ok {
		t.Fatal("parseDateRange returned ok=false for empty dates")
	}

	if from.Valid || to.Valid {
		t.Error("expected both dates to be invalid when not provided")
	}
}

func TestParseDateRange_InvalidFrom(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/?from=bad", nil)
	w := httptest.NewRecorder()

	_, _, ok := parseDateRange(w, req)
	if ok {
		t.Error("expected ok=false for invalid 'from' date")
	}

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestToFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input any
		want  float64
	}{
		{float64(3.14), 3.14},
		{int64(42), 42.0},
		{int(7), 7.0},
		{nil, 0.0},
		{string("1.5"), 1.5},
	}
	for _, tt := range tests {
		if got := toFloat(tt.input); got != tt.want {
			t.Errorf("toFloat(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestToInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input any
		want  int64
	}{
		{int64(42), 42},
		{int(7), 7},
		{float64(3.9), 3},
		{nil, 0},
		{string("100"), 100},
	}
	for _, tt := range tests {
		if got := toInt64(tt.input); got != tt.want {
			t.Errorf("toInt64(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
