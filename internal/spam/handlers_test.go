package spam

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// --- test helpers ---

// ctxWithUser adds an authenticated user to the request context.
func ctxWithUser(r *http.Request, userID, tenantID string) *http.Request {
	var uid, tid pgtype.UUID
	_ = uid.Scan(userID)
	_ = tid.Scan(tenantID)
	user := &auth.AuthenticatedUser{UserID: uid}

	return r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, user))
}

// ctxWithTenant adds a tenant to the request context.
func ctxWithTenant(r *http.Request, tenantID string) *http.Request {
	var tid pgtype.UUID
	_ = tid.Scan(tenantID)
	t := &tenant.Tenant{ID: tid}

	return r.WithContext(context.WithValue(r.Context(), tenant.TenantContextKey, t))
}

// setupRouter creates a chi.Router with the spam admin routes registered.
func setupRouter(h *Handlers) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/admin/spam", func(r chi.Router) {
		r.Get("/ip-blocks", h.ListIPBlocks)
		r.Post("/ip-blocks", h.CreateIPBlock)
		r.Delete("/ip-blocks/{id}", h.DeleteIPBlock)
		r.Get("/censored-words", h.ListCensoredWords)
		r.Post("/censored-words", h.CreateCensoredWord)
		r.Delete("/censored-words/{id}", h.DeleteCensoredWord)
		r.Get("/user-blocks", h.ListUserBlocks)
		r.Delete("/user-blocks/{blockerId}/{blockedId}", h.DeleteUserBlock)
	})

	return r
}

const (
	testTenantID = "00000000-0000-0000-0000-000000000001"
	testUserID   = "00000000-0000-0000-0000-000000000002"
)

func addContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = ctxWithTenant(r, testTenantID)
		r = ctxWithUser(r, testUserID, testTenantID)
		next.ServeHTTP(w, r)
	})
}

// --- writeJSON / writeError tests ---

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	errpkg.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("body.status = %q, want ok", body["status"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	errpkg.WriteError(w, http.StatusBadRequest, "test_code", "test message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var body map[string]map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["error"]["code"] != "test_code" {
		t.Errorf("error.code = %q, want test_code", body["error"]["code"])
	}

	if body["error"]["message"] != "test message" {
		t.Errorf("error.message = %q, want test message", body["error"]["message"])
	}
}

// --- parsePageSize tests ---

func TestParsePageSize_Default(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/admin/spam/ip-blocks", nil)

	got := parsePageSize(r)
	if got != defaultPageSize {
		t.Errorf("parsePageSize() = %d, want %d", got, defaultPageSize)
	}
}

func TestParsePageSize_Custom(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/admin/spam/ip-blocks?page_size=10", nil)

	got := parsePageSize(r)
	if got != 10 {
		t.Errorf("parsePageSize() = %d, want 10", got)
	}
}

func TestParsePageSize_Clamped(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/admin/spam/ip-blocks?page_size=999", nil)

	got := parsePageSize(r)
	if got != maxPageSize {
		t.Errorf("parsePageSize() = %d, want %d", got, maxPageSize)
	}
}

func TestParsePageSize_Invalid(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/admin/spam/ip-blocks?page_size=abc", nil)

	got := parsePageSize(r)
	if got != defaultPageSize {
		t.Errorf("parsePageSize() = %d, want %d", got, defaultPageSize)
	}
}

// --- validation tests ---

func TestCreateIPBlock_MissingCIDR(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	body := `{"block_type":"full","reason":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/spam/ip-blocks", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]map[string]string

	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp["error"]["code"] != "missing_cidr" {
		t.Errorf("error code = %q, want missing_cidr", resp["error"]["code"])
	}
}

func TestCreateIPBlock_MissingBlockType(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	body := `{"cidr":"192.168.1.0/24"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/spam/ip-blocks", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]map[string]string

	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp["error"]["code"] != "missing_block_type" {
		t.Errorf("error code = %q, want missing_block_type", resp["error"]["code"])
	}
}

func TestCreateIPBlock_InvalidBlockType(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	body := `{"cidr":"192.168.1.0/24","block_type":"invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/spam/ip-blocks", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]map[string]string

	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp["error"]["code"] != "invalid_block_type" {
		t.Errorf("error code = %q, want invalid_block_type", resp["error"]["code"])
	}
}

func TestCreateIPBlock_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	req := httptest.NewRequest(http.MethodPost, "/admin/spam/ip-blocks", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateCensoredWord_MissingPattern(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	body := `{"action":"block"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/spam/censored-words", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]map[string]string

	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp["error"]["code"] != "missing_pattern" {
		t.Errorf("error code = %q, want missing_pattern", resp["error"]["code"])
	}
}

func TestCreateCensoredWord_MissingAction(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	body := `{"pattern":"spam"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/spam/censored-words", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]map[string]string

	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp["error"]["code"] != "missing_action" {
		t.Errorf("error code = %q, want missing_action", resp["error"]["code"])
	}
}

func TestCreateCensoredWord_InvalidAction(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	body := `{"pattern":"spam","action":"delete"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/spam/censored-words", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]map[string]string

	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp["error"]["code"] != "invalid_action" {
		t.Errorf("error code = %q, want invalid_action", resp["error"]["code"])
	}
}

func TestCreateCensoredWord_ReplaceRequiresReplacement(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	body := `{"pattern":"bad","action":"replace"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/spam/censored-words", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]map[string]string

	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp["error"]["code"] != "missing_replacement" {
		t.Errorf("error code = %q, want missing_replacement", resp["error"]["code"])
	}
}

func TestDeleteIPBlock_InvalidID(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	req := httptest.NewRequest(http.MethodDelete, "/admin/spam/ip-blocks/not-a-uuid", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDeleteCensoredWord_InvalidID(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	req := httptest.NewRequest(http.MethodDelete, "/admin/spam/censored-words/not-a-uuid", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDeleteUserBlock_InvalidBlockerID(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	req := httptest.NewRequest(http.MethodDelete, "/admin/spam/user-blocks/not-a-uuid/some-id", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDeleteUserBlock_InvalidBlockedID(t *testing.T) {
	t.Parallel()

	h := &Handlers{Store: nil}
	router := setupRouter(h)
	wrapped := addContextMiddleware(router)

	validID := "00000000-0000-0000-0000-000000000002"
	req := httptest.NewRequest(http.MethodDelete, "/admin/spam/user-blocks/"+validID+"/not-a-uuid", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- requireTenant/requireUser tests ---

func TestRequireTenant_Missing(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/spam/ip-blocks", nil)
	// No tenant in context
	_, ok := requireTenant(w, req)
	if ok {
		t.Error("requireTenant should return false when tenant is missing")
	}

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestRequireUser_Missing(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/spam/ip-blocks", nil)
	// No user in context
	_, ok := requireUser(w, req)
	if ok {
		t.Error("requireUser should return false when user is missing")
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// --- store validation tests (reused from spam_test.go) ---

func TestCreateIPBlockInput_Validation(t *testing.T) {
	t.Parallel()
	// Verify that block type and censor action constants are defined correctly.
	if !IsValidBlockType("full") {
		t.Error("IsValidBlockType('full') = false, want true")
	}

	if !IsValidBlockType("registration") {
		t.Error("IsValidBlockType('registration') = false, want true")
	}

	if !IsValidBlockType("posting") {
		t.Error("IsValidBlockType('posting') = false, want true")
	}

	if !IsValidBlockType("api") {
		t.Error("IsValidBlockType('api') = false, want true")
	}

	if IsValidBlockType("unknown") {
		t.Error("IsValidBlockType('unknown') = true, want false")
	}

	if !IsValidCensorAction("block") {
		t.Error("IsValidCensorAction('block') = false, want true")
	}

	if !IsValidCensorAction(CensorActionReplace) {
		t.Error("IsValidCensorAction('replace') = false, want true")
	}

	if !IsValidCensorAction("flag") {
		t.Error("IsValidCensorAction('flag') = false, want true")
	}

	if IsValidCensorAction("unknown") {
		t.Error("IsValidCensorAction('unknown') = true, want false")
	}
}
