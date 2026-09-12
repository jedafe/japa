package users

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
	"github.com/jedafe/japa/internal/tenant"
)

func withAuthUser(r *http.Request) *http.Request {
	ctx := r.Context()

	var uid, tid pgtype.UUID

	uid.Scan("22222222-2222-2222-2222-222222222222")
	tid.Scan("11111111-1111-1111-1111-111111111111")

	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: uid,
	})

	return r.WithContext(ctx)
}

func withTenant(r *http.Request) *http.Request {
	ctx := r.Context()

	var tid pgtype.UUID

	tid.Scan("11111111-1111-1111-1111-111111111111")

	ctx = context.WithValue(ctx, tenant.TenantContextKey, &tenant.Tenant{
		ID: tid,
	})

	return r.WithContext(ctx)
}

// testService creates a Service with nil store for handler-only validation tests.
// Handlers that call the store will fail; these tests only cover input validation.
func testService() *Service {
	return &Service{store: &Store{}}
}

func writeJSONBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}

	return bytes.NewBuffer(b)
}

// ---------------------------------------------------------------------------
// ChangePassword — validation
// ---------------------------------------------------------------------------

func TestChangePassword_MissingBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	req := httptest.NewRequest(http.MethodPost, "/me/change-password", nil)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestChangePassword_EmptyFields(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	body := writeJSONBody(t, map[string]string{
		"current_password": "",
		"new_password":     "",
	})

	req := httptest.NewRequest(http.MethodPost, "/me/change-password", body)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestChangePassword_ShortPassword(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	body := writeJSONBody(t, map[string]string{
		"current_password": "oldpassword",
		"new_password":     "short",
	})

	req := httptest.NewRequest(http.MethodPost, "/me/change-password", body)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// GetPrivacy / UpdatePrivacy — validation
// ---------------------------------------------------------------------------

func TestGetPrivacy_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	req := httptest.NewRequest(http.MethodGet, "/me/privacy", nil)

	rr := httptest.NewRecorder()
	h.GetPrivacy(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestUpdatePrivacy_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	req := httptest.NewRequest(http.MethodPatch, "/me/privacy", bytes.NewBufferString("not json"))
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	h.UpdatePrivacy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// RevokeSession — validation
// ---------------------------------------------------------------------------

func TestRevokeSession_InvalidID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)

	router := chi.NewRouter()
	router.Delete("/me/sessions/{id}", h.RevokeSession)

	req := httptest.NewRequest(http.MethodDelete, "/me/sessions/not-a-uuid", nil)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// ListMyContent — unauthorized
// ---------------------------------------------------------------------------

func TestListMyContent_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	req := httptest.NewRequest(http.MethodGet, "/me/content", nil)

	rr := httptest.NewRecorder()
	h.ListMyContent(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// DeactivateAccount — missing password
// ---------------------------------------------------------------------------

func TestDeactivateAccount_MissingPassword(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	body := writeJSONBody(t, map[string]string{})

	req := httptest.NewRequest(http.MethodPost, "/me/deactivate", body)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	h.DeactivateAccount(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// GetMyDraft (GET /me/drafts/{id}) — auth + input validation
// ---------------------------------------------------------------------------

func TestGetMyDraft_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	req := httptest.NewRequest(http.MethodGet, "/me/drafts/22222222-2222-2222-2222-222222222222", nil)

	rr := httptest.NewRecorder()
	h.GetMyDraft(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestGetMyDraft_NoTenant(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	req := httptest.NewRequest(http.MethodGet, "/me/drafts/22222222-2222-2222-2222-222222222222", nil)
	req = withAuthUser(req) // authenticated but no tenant in context

	rr := httptest.NewRecorder()
	h.GetMyDraft(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestGetMyDraft_InvalidID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)

	router := chi.NewRouter()
	router.Get("/me/drafts/{id}", h.GetMyDraft)

	req := httptest.NewRequest(http.MethodGet, "/me/drafts/not-a-uuid", nil)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("response missing error envelope: %v", body)
	}

	if errObj["code"] != "invalid_id" {
		t.Errorf("error code = %v, want invalid_id", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// GetMyContent (GET /me/content/{id}) — auth + input validation
// ---------------------------------------------------------------------------

func TestGetMyContent_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	req := httptest.NewRequest(http.MethodGet, "/me/content/22222222-2222-2222-2222-222222222222", nil)

	rr := httptest.NewRecorder()
	h.GetMyContent(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestGetMyContent_NoTenant(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)
	req := httptest.NewRequest(http.MethodGet, "/me/content/22222222-2222-2222-2222-222222222222", nil)
	req = withAuthUser(req) // authenticated but no tenant in context

	rr := httptest.NewRecorder()
	h.GetMyContent(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestGetMyContent_InvalidID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(testService(), nil)

	router := chi.NewRouter()
	router.Get("/me/content/{id}", h.GetMyContent)

	req := httptest.NewRequest(http.MethodGet, "/me/content/not-a-uuid", nil)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("response missing error envelope: %v", body)
	}

	if errObj["code"] != "invalid_id" {
		t.Errorf("error code = %v, want invalid_id", errObj["code"])
	}
}
