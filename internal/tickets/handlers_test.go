package tickets

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Handler unit tests (httptest, no DB)
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %s", body["status"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "invalid_input", "Bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var body map[string]map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if body["error"]["code"] != "invalid_input" {
		t.Errorf("expected error code invalid_input, got %s", body["error"]["code"])
	}
	if body["error"]["message"] != "Bad request" {
		t.Errorf("expected error message Bad request, got %s", body["error"]["message"])
	}
}

func TestParsePageSize(t *testing.T) {
	// Default page size.
	r := httptest.NewRequest("GET", "/tickets", nil)
	if ps := parsePageSize(r); ps != DefaultPageSize {
		t.Errorf("expected default page size %d, got %d", DefaultPageSize, ps)
	}

	// Custom page size.
	r2 := httptest.NewRequest("GET", "/tickets?page_size=50", nil)
	if ps := parsePageSize(r2); ps != 50 {
		t.Errorf("expected page size 50, got %d", ps)
	}

	// Page size exceeding max.
	r3 := httptest.NewRequest("GET", "/tickets?page_size=200", nil)
	if ps := parsePageSize(r3); ps != MaxPageSize {
		t.Errorf("expected max page size %d, got %d", MaxPageSize, ps)
	}

	// Invalid page size.
	r4 := httptest.NewRequest("GET", "/tickets?page_size=abc", nil)
	if ps := parsePageSize(r4); ps != DefaultPageSize {
		t.Errorf("expected default page size for invalid input, got %d", ps)
	}
}

func TestParsePageSizeOffset(t *testing.T) {
	// Default offset.
	r := httptest.NewRequest("GET", "/tickets", nil)
	if po := parsePageSizeOffset(r); po != 0 {
		t.Errorf("expected default offset 0, got %d", po)
	}

	// Custom offset.
	r2 := httptest.NewRequest("GET", "/tickets?page_offset=20", nil)
	if po := parsePageSizeOffset(r2); po != 20 {
		t.Errorf("expected offset 20, got %d", po)
	}

	// Negative offset.
	r3 := httptest.NewRequest("GET", "/tickets?page_offset=-5", nil)
	if po := parsePageSizeOffset(r3); po != 0 {
		t.Errorf("expected 0 for negative offset, got %d", po)
	}
}

func TestCreateTicketHandlerMissingBody(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest("POST", "/support", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// This will fail because there's no tenant/user in context,
	// but we're testing the handler structure.
	h.CreateTicket(w, req)

	// Should return unauthorized or service unavailable (no tenant/user).
	if w.Code == http.StatusOK {
		t.Error("expected non-OK status for missing context")
	}
}

func TestListMyTicketsHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest("GET", "/support/my", nil)
	w := httptest.NewRecorder()

	h.ListMyTickets(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetTicketHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest("GET", "/support/my/123", nil)
	w := httptest.NewRecorder()

	h.GetMyTicket(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAddReplyHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest("POST", "/support/my/123/reply", bytes.NewBuffer([]byte(`{"body":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.AddReply(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListTicketsHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest("GET", "/api/v1/admin/tickets", nil)
	w := httptest.NewRecorder()

	h.ListTickets(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetTicketEventsHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest("GET", "/api/v1/admin/tickets/123/events", nil)
	w := httptest.NewRecorder()

	h.GetTicketEvents(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListTicketTypesHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	req := httptest.NewRequest("GET", "/api/v1/admin/ticket-types", nil)
	w := httptest.NewRecorder()

	h.ListTicketTypes(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreateTicketTypeHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	body := `{"name":"Support","slug":"support"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/ticket-types", bytes.NewBuffer([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTicketType(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestUpdateTicketHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	body := `{"status":"in_progress"}`
	req := httptest.NewRequest("PATCH", "/api/v1/admin/tickets/123", bytes.NewBuffer([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateTicket(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAddInternalNoteHandlerNoAuth(t *testing.T) {
	svc := &Service{}
	h := NewHandlers(svc)

	body := `{"body":"Internal note"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/tickets/123/notes", bytes.NewBuffer([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.AddInternalNote(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
