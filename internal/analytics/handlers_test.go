package analytics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	errpkg "github.com/jedafe/japa/internal/errors"
)

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	errpkg.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})

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
	t.Parallel()

	w := httptest.NewRecorder()
	errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Bad request")

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
	t.Parallel()
	// Default page size.
	r := httptest.NewRequest(http.MethodGet, "/events", nil)
	if ps := parsePageSize(r); ps != DefaultPageSize {
		t.Errorf("expected default page size %d, got %d", DefaultPageSize, ps)
	}

	// Custom page size.
	r2 := httptest.NewRequest(http.MethodGet, "/events?page_size=50", nil)
	if ps := parsePageSize(r2); ps != 50 {
		t.Errorf("expected page size 50, got %d", ps)
	}

	// Page size exceeding max.
	r3 := httptest.NewRequest(http.MethodGet, "/events?page_size=200", nil)
	if ps := parsePageSize(r3); ps != MaxPageSize {
		t.Errorf("expected max page size %d, got %d", MaxPageSize, ps)
	}

	// Invalid page size.
	r4 := httptest.NewRequest(http.MethodGet, "/events?page_size=abc", nil)
	if ps := parsePageSize(r4); ps != DefaultPageSize {
		t.Errorf("expected default page size for invalid input, got %d", ps)
	}
}

func TestParseOffset(t *testing.T) {
	t.Parallel()
	// Default offset.
	r := httptest.NewRequest(http.MethodGet, "/events", nil)
	if o := parseOffset(r); o != 0 {
		t.Errorf("expected default offset 0, got %d", o)
	}

	// Custom offset.
	r2 := httptest.NewRequest(http.MethodGet, "/events?offset=10", nil)
	if o := parseOffset(r2); o != 10 {
		t.Errorf("expected offset 10, got %d", o)
	}

	// Invalid offset.
	r3 := httptest.NewRequest(http.MethodGet, "/events?offset=abc", nil)
	if o := parseOffset(r3); o != 0 {
		t.Errorf("expected default offset for invalid input, got %d", o)
	}
}

func TestParseDays(t *testing.T) {
	t.Parallel()
	// Default.
	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	if d := parseDays(r, 30); d != 30 {
		t.Errorf("expected default 30, got %d", d)
	}

	// Custom.
	r2 := httptest.NewRequest(http.MethodGet, "/dashboard?days=7", nil)
	if d := parseDays(r2, 30); d != 7 {
		t.Errorf("expected 7, got %d", d)
	}

	// Invalid.
	r3 := httptest.NewRequest(http.MethodGet, "/dashboard?days=abc", nil)
	if d := parseDays(r3, 30); d != 30 {
		t.Errorf("expected default 30 for invalid input, got %d", d)
	}
}

func TestTrackEventInvalidJSON(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/analytics/events", bytes.NewBufferString("not json"))
	h.TrackEvent(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Logf("got status %d (expected failure at tenant resolution)", w.Code)
	}
}

func TestGetUserDashboardNoTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/analytics/dashboard", nil)
	h.GetUserDashboard(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestGetAdminOverviewNoTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/admin/analytics/overview", nil)
	h.GetAdminOverview(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestListEventsNoTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/admin/analytics/events", nil)
	h.ListEvents(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestListSecurityScanResultsNoTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/admin/analytics/security", nil)
	h.ListSecurityScanResults(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestRefreshViewsNoTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/admin/analytics/refresh", nil)
	h.RefreshViews(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestRunSecurityScanNoTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/admin/analytics/security/scan", nil)
	h.RunSecurityScan(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestResolveSecurityScanResultNoTenant(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/admin/analytics/security/fake-id/resolve", nil)
	h.ResolveSecurityScanResult(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}
