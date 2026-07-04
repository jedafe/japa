package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// mustUUID scans a UUID string into a pgtype.UUID, failing the test on error.
func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

// ---------------------------------------------------------------------------
// NewHandlers
// ---------------------------------------------------------------------------

func TestNewHandlers(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	if h == nil {
		t.Fatal("NewHandlers returned nil")
	}
}

// ---------------------------------------------------------------------------
// Search — missing auth
// ---------------------------------------------------------------------------

func TestSearch_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "unauthorized" {
		t.Errorf("error.code = %v, want unauthorized", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// Search — missing tenant
// ---------------------------------------------------------------------------

func TestSearch_TenantUnavailable(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	req = withAuthCtxDirect(req, &auth.AuthenticatedUser{
		UserID:   mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
		TenantID: mustUUID(t, "00000000-0000-0000-0000-000000000001"),
	})
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "tenant_unavailable" {
		t.Errorf("error.code = %v, want tenant_unavailable", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// Search — missing query parameter
// ---------------------------------------------------------------------------

func TestSearch_MissingQuery(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "missing_query" {
		t.Errorf("error.code = %v, want missing_query", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// Search — valid query with default limit
// ---------------------------------------------------------------------------

func TestSearch_ValidQueryDefaultLimit(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=nigeria", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %v", body)
	}

	results, ok := data["results"].([]any)
	if !ok {
		t.Fatalf("expected results array, got %v", data)
	}

	if len(results) != 0 {
		t.Errorf("expected empty stub results, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Search — valid query with custom limit
// ---------------------------------------------------------------------------

func TestSearch_ValidQueryCustomLimit(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=lagos&limit=5", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Search — invalid limit falls back to default
// ---------------------------------------------------------------------------

func TestSearch_InvalidLimitFallback(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit=abc", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Search — limit out of range falls back to default
// ---------------------------------------------------------------------------

func TestSearch_OutOfRangeLimitFallback(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit=200", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Search — limit zero falls back to default
// ---------------------------------------------------------------------------

func TestSearch_ZeroLimitFallback(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit=0", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Search — negative limit falls back to default
// ---------------------------------------------------------------------------

func TestSearch_NegativeLimitFallback(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit=-5", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// writeJSON
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"hello": "world"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if got["hello"] != "world" {
		t.Errorf("body.hello = %q, want world", got["hello"])
	}
}

// ---------------------------------------------------------------------------
// writeError
// ---------------------------------------------------------------------------

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "bad", "something wrong")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope, got %v", body)
	}

	if errObj["code"] != "bad" {
		t.Errorf("error.code = %v, want bad", errObj["code"])
	}

	if errObj["message"] != "something wrong" {
		t.Errorf("error.message = %v, want 'something wrong'", errObj["message"])
	}
}

// ---------------------------------------------------------------------------
// SearchResult struct
// ---------------------------------------------------------------------------

func TestSearchResultJSON(t *testing.T) {
	t.Parallel()

	sr := SearchResult{
		Type:    "content",
		ID:      "abc-123",
		Title:   "Test Post",
		Excerpt: "A short excerpt",
		URL:     "/blog/test-post",
	}

	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got SearchResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Type != sr.Type {
		t.Errorf("Type = %q, want %q", got.Type, sr.Type)
	}

	if got.Title != sr.Title {
		t.Errorf("Title = %q, want %q", got.Title, sr.Title)
	}
}

// ---------------------------------------------------------------------------
// SearchResponse struct
// ---------------------------------------------------------------------------

func TestSearchResponseJSON(t *testing.T) {
	t.Parallel()

	resp := SearchResponse{
		Results: []SearchResult{
			{Type: "content", ID: "1", Title: "First", URL: "/first"},
			{Type: "user", ID: "2", Title: "Second", URL: "/users/second"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got SearchResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(got.Results))
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// withAuthCtxDirect injects an AuthenticatedUser directly into the request context.
func withAuthCtxDirect(r *http.Request, user *auth.AuthenticatedUser) *http.Request {
	return r.WithContext(&authValueCtx{Context: r.Context(), key: auth.UserContextKey, val: user})
}

// withAuthAndTenant injects both auth user and tenant into the request context.
func withAuthAndTenant(t *testing.T, r *http.Request) *http.Request {
	t.Helper()

	user := &auth.AuthenticatedUser{
		UserID:   mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
		TenantID: mustUUID(t, "00000000-0000-0000-0000-000000000001"),
	}
	ten := &tenant.Tenant{
		ID:   mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Name: "Test Tenant",
		Slug: "test",
	}

	return r.WithContext(&multiValueCtx{
		Context: r.Context(),
		values: map[any]any{
			auth.UserContextKey:     user,
			tenant.TenantContextKey: ten,
		},
	})
}

// authValueCtx is a context that stores a single key-value pair.
type authValueCtx struct {
	context.Context
	key any
	val any
}

func (c *authValueCtx) Value(key any) any {
	if key == c.key {
		return c.val
	}

	return c.Context.Value(key)
}

// multiValueCtx is a context that stores multiple key-value pairs.
type multiValueCtx struct {
	context.Context
	values map[any]any
}

func (c *multiValueCtx) Value(key any) any {
	if v, ok := c.values[key]; ok {
		return v
	}

	return c.Context.Value(key)
}
