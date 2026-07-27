package trending

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestParseLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  int32
	}{
		{name: "absent", query: "", want: defaultLimit},
		{name: "empty", query: "limit=", want: defaultLimit},
		{name: "valid", query: "limit=5", want: 5},
		{name: "at max", query: fmt.Sprintf("limit=%d", maxLimit), want: maxLimit},
		{name: "above max is clamped", query: "limit=1000", want: maxLimit},
		{name: "zero falls back", query: "limit=0", want: defaultLimit},
		{name: "negative falls back", query: "limit=-3", want: defaultLimit},
		{name: "non numeric falls back", query: "limit=abc", want: defaultLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/trending/content?"+tt.query, nil)

			if got := parseLimit(req); got != tt.want {
				t.Errorf("parseLimit(%q) = %d, want %d", tt.query, got, tt.want)
			}
		})
	}
}

func TestParseWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  string
	}{
		{query: "", want: "24h"},
		{query: "window=24h", want: "24h"},
		{query: "window=7d", want: "7d"},
		{query: "window=30d", want: "30d"},
		{query: "window=1y", want: "24h"},
		{query: "window=", want: "24h"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/trending/content?"+tt.query, nil)

			if got := parseWindow(req); got != tt.want {
				t.Errorf("parseWindow(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestParseGeoCode(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/trending/content?geo=NG", nil)
	if got := parseGeoCode(req); got != "NG" {
		t.Errorf("parseGeoCode() = %q, want NG", got)
	}

	// When no geo param is provided, parseGeoCode falls back to the visitor
	// context's ContentCountry, which defaults to "NG" (ADR-064).
	bare := httptest.NewRequest(http.MethodGet, "/trending/content", nil)
	if got := parseGeoCode(bare); got != "NG" {
		t.Errorf("parseGeoCode() = %q, want NG (visitor default)", got)
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]any{"ok": true})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if !body["ok"] {
		t.Errorf("body = %v, want ok=true", body)
	}
}

func TestWriteJSONNilBody(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}

	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
}

// decodeErrorCode extracts error.code from a JSON error envelope.
func decodeErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Error.Message == "" {
		t.Error("error message is empty")
	}

	return body.Error.Code
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "bad_request", "Bad input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}

	if code := decodeErrorCode(t, rec); code != "bad_request" {
		t.Errorf("error code = %q, want bad_request", code)
	}
}

func TestHandleServiceError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "no rows", err: pgx.ErrNoRows, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{
			name:       "wrapped no rows",
			err:        fmt.Errorf("querying trending: %w", pgx.ErrNoRows),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "unknown error",
			err:        pgx.ErrTxClosed,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			handleServiceError(rec, tt.err)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if code := decodeErrorCode(t, rec); code != tt.wantCode {
				t.Errorf("error code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

func TestWindowInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		window string
		want   string
	}{
		{window: "24h", want: "24 hours"},
		{window: "7d", want: "7 days"},
		{window: "30d", want: "30 days"},
		{window: "", want: "24 hours"},
		{window: "bogus", want: "24 hours"},
	}

	for _, tt := range tests {
		t.Run(tt.window, func(t *testing.T) {
			t.Parallel()

			if got := windowInterval(tt.window); got != tt.want {
				t.Errorf("windowInterval(%q) = %q, want %q", tt.window, got, tt.want)
			}
		})
	}
}
