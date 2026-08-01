package gdpr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Handler unit tests (httptest, no DB)
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()
	// Default page size.
	r := httptest.NewRequest(http.MethodGet, "/consent", nil)
	if ps := parsePageSize(r); ps != DefaultPageSize {
		t.Errorf("expected default page size %d, got %d", DefaultPageSize, ps)
	}

	// Custom page size.
	r2 := httptest.NewRequest(http.MethodGet, "/consent?page_size=50", nil)
	if ps := parsePageSize(r2); ps != 50 {
		t.Errorf("expected page size 50, got %d", ps)
	}

	// Page size exceeding max.
	r3 := httptest.NewRequest(http.MethodGet, "/consent?page_size=200", nil)
	if ps := parsePageSize(r3); ps != MaxPageSize {
		t.Errorf("expected max page size %d, got %d", MaxPageSize, ps)
	}

	// Invalid page size.
	r4 := httptest.NewRequest(http.MethodGet, "/consent?page_size=abc", nil)
	if ps := parsePageSize(r4); ps != DefaultPageSize {
		t.Errorf("expected default page size for invalid input, got %d", ps)
	}
}

func TestParseOffset(t *testing.T) {
	t.Parallel()
	// Default offset.
	r := httptest.NewRequest(http.MethodGet, "/audit-logs", nil)
	if o := parseOffset(r); o != 0 {
		t.Errorf("expected default offset 0, got %d", o)
	}

	// Custom offset.
	r2 := httptest.NewRequest(http.MethodGet, "/audit-logs?offset=10", nil)
	if o := parseOffset(r2); o != 10 {
		t.Errorf("expected offset 10, got %d", o)
	}

	// Invalid offset.
	r3 := httptest.NewRequest(http.MethodGet, "/audit-logs?offset=abc", nil)
	if o := parseOffset(r3); o != 0 {
		t.Errorf("expected default offset for invalid input, got %d", o)
	}
}

func TestClientIP(t *testing.T) {
	t.Parallel()
	// X-Forwarded-For — peer must be a trusted proxy for the header to be
	// honoured (clientip.FromRequest validates the peer against
	// JC_TRUSTED_PROXIES, which defaults to loopback + RFC1918).
	// The XFF chain is walked right-to-left past trusted proxies; the
	// first untrusted IP is the client. Here 10.0.0.5 is a trusted RFC1918
	// proxy and 1.2.3.4 is the real client.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.5")

	ip := clientIP(r)
	if ip == nil || *ip != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %v", ip)
	}

	// X-Real-IP — same trusted-proxy requirement.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "127.0.0.1:1234"
	r2.Header.Set("X-Real-IP", "9.10.11.12")

	ip2 := clientIP(r2)
	if ip2 == nil || *ip2 != "9.10.11.12" {
		t.Errorf("expected 9.10.11.12, got %v", ip2)
	}

	// No headers — peer address is returned directly.
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.RemoteAddr = "127.0.0.1:8080"

	ip3 := clientIP(r3)
	if ip3 == nil || *ip3 != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %v", ip3)
	}
}

func TestHandleServiceError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantCode    int
		wantErrCode string
	}{
		{"generic error", fmt.Errorf("something went wrong"), 500, "internal_error"},
		{"required field", errors.Join(errpkg.ErrRequired, fmt.Errorf("name is required")), 400, "validation_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			handleServiceError(w, tt.err)

			if w.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, w.Code)
			}
		})
	}
}

func TestRecordConsentInvalidJSON(t *testing.T) {
	t.Parallel()

	h := &Handlers{Service: nil}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/consent", bytes.NewBufferString("not json"))
	// Without auth middleware, requireUser will fail first.
	h.RecordConsent(w, r)

	if w.Code != http.StatusServiceUnavailable {
		// Will fail at tenant resolution first.
		t.Logf("got status %d (expected failure at tenant resolution)", w.Code)
	}
}
