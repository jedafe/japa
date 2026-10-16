// handlers_test.go — auth + validation coverage for the §22 appeal routes.
package appeals

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSubmitNoAuth(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodPost, "/appeals",
		strings.NewReader(`{"entity_type":"service_claim","entity_id":"x","reason":"y"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	h.HandleSubmit(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleSubmitInvalidBody(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodPost, "/appeals", strings.NewReader(`{bad json`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	h.HandleSubmit(w, req)

	// No auth context → 401 fires before body parsing would matter, but a
	// malformed body with no auth still must not reach the service.
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusBadRequest {
		t.Errorf("expected 401 or 400, got %d", w.Code)
	}
}

func TestHandleListMineNoAuth(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/me/appeals", nil)
	w := httptest.NewRecorder()

	h.HandleListMine(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
