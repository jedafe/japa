package comments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Sentinel-like errors for testing handleServiceError.
var (
	errNoRows        = pgx.ErrNoRows
	errDuplicate     = errors.New("duplicate key value violates unique constraint")
	errDepthExceeded = errors.New("maximum comment depth exceeded (max 3)")
	errGeneric       = errors.New("something went wrong")
)

// ---------------------------------------------------------------------------
// writeJSON / writeError
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]any{"data": "ok"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body["data"] != "ok" {
		t.Errorf("data = %v, want ok", body["data"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "missing_body", "Comment body is required")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field is not a map: %v", body["error"])
	}

	if errObj["code"] != "missing_body" {
		t.Errorf("error.code = %v, want missing_body", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// parsePageSize
// ---------------------------------------------------------------------------

func TestParsePageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  int32
	}{
		{"default", "", 20},
		{"custom", "?page_size=10", 10},
		{"clamped", "?page_size=500", 100},
		{"invalid", "?page_size=abc", 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/"+tt.query, nil)
			got := parsePageSize(r)

			if got != tt.want {
				t.Errorf("parsePageSize(%q) = %d, want %d", tt.query, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseUUIDParam
// ---------------------------------------------------------------------------

func TestParseUUIDParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"invalid", "not-a-uuid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.value)
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

			got, err := parseUUIDParam(r, "id")
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseUUIDParam() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && !got.Valid {
				t.Error("UUID should be valid")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// handleServiceError — table-driven
// ---------------------------------------------------------------------------

func TestHandleServiceError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not_found", errNoRows, http.StatusNotFound, "not_found"},
		{"duplicate", errDuplicate, http.StatusConflict, "already_exists"},
		{"depth_exceeded", errDepthExceeded, http.StatusBadRequest, "depth_exceeded"},
		{"internal", errGeneric, http.StatusInternalServerError, "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			handleServiceError(rr, tt.err)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}

			var body map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			errObj, ok := body["error"].(map[string]any)
			if !ok {
				t.Fatalf("error field is not a map: %v", body["error"])
			}

			if errObj["code"] != tt.wantCode {
				t.Errorf("error.code = %v, want %s", errObj["code"], tt.wantCode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestDefaultPageSize(t *testing.T) {
	t.Parallel()

	if defaultPageSize != 20 {
		t.Errorf("defaultPageSize = %d, want 20", defaultPageSize)
	}
}
