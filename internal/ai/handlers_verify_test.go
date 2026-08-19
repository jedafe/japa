package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/auth"
)

// TestHandleVerifyHITL_MissingAuth verifies the endpoint rejects unauthenticated
// requests before doing any work.
func TestHandleVerifyHITL_MissingAuth(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	req := httptest.NewRequest(http.MethodPost, "/ai/hitl/verify", bytes.NewReader([]byte(`{"claim":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleVerifyHITL(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// TestHandleVerifyHITL_MissingTenant verifies the endpoint rejects requests
// without a tenant in context.
func TestHandleVerifyHITL_MissingTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	user, _ := testUserAndTenant()
	req := httptest.NewRequest(http.MethodPost, "/ai/hitl/verify", bytes.NewReader([]byte(`{"claim":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	// Only user context, no tenant.
	ctx := context.WithValue(req.Context(), auth.UserContextKey, user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.HandleVerifyHITL(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// TestHandleVerifyHITL_MissingClaim verifies 400 when the claim is empty.
func TestHandleVerifyHITL_MissingClaim(t *testing.T) {
	t.Parallel()

	h := &Handlers{Settings: &providerSettingsReader{}}
	user, tenant := testUserAndTenant()
	req := httptest.NewRequest(http.MethodPost, "/ai/hitl/verify", bytes.NewReader([]byte(`{"claim":""}`)))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, tenant)
	w := httptest.NewRecorder()

	h.HandleVerifyHITL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestHandleVerifyHITL_MissingSettings verifies 503 when the settings reader is
// not wired.
func TestHandleVerifyHITL_MissingSettings(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	user, tenant := testUserAndTenant()
	req := httptest.NewRequest(http.MethodPost, "/ai/hitl/verify", bytes.NewReader([]byte(`{"claim":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, tenant)
	w := httptest.NewRecorder()

	h.HandleVerifyHITL(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// TestHandleVerifyHITL_MissingAPIKey verifies 503 when external search is
// configured as perplexity but the API key is empty.
func TestHandleVerifyHITL_MissingAPIKey(t *testing.T) {
	t.Parallel()

	h := &Handlers{
		Settings: &providerSettingsReader{provider: "perplexity", apiKey: ""},
	}
	user, tenant := testUserAndTenant()
	req := httptest.NewRequest(http.MethodPost, "/ai/hitl/verify", bytes.NewReader([]byte(`{"claim":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, tenant)
	w := httptest.NewRecorder()

	h.HandleVerifyHITL(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	body := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	err, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error not an object: %#v", body)
	}
	if err["code"] != "verification_unavailable" {
		t.Errorf("error code = %q, want verification_unavailable", err["code"])
	}
}

// Note: a full end-to-end success test for HandleVerifyHITL would require a
// configurable external search base URL (the search clients hard-code Tavily,
// Brave, and Perplexity endpoints). Perplexity provider-level citation parsing
// is covered in perplexity_test.go.
