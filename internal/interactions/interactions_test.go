package interactions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/httpx"
)

// ---------------------------------------------------------------------------
// writeJSON / writeError
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	httpx.WriteJSON(rr, http.StatusOK, map[string]any{"data": "ok"})

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
	httpx.WriteError(rr, http.StatusNotFound, "not_found", "Resource not found")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field is not a map: %v", body["error"])
	}

	if errObj["code"] != "not_found" {
		t.Errorf("error.code = %v, want not_found", errObj["code"])
	}

	if errObj["message"] != "Resource not found" {
		t.Errorf("error.message = %v, want 'Resource not found'", errObj["message"])
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
		{"default", "", defaultPageSize},
		{"custom", "?page_size=50", 50},
		{"clamped_to_max", "?page_size=999", MaxPageSize},
		{"invalid", "?page_size=abc", defaultPageSize},
		{"zero", "?page_size=0", defaultPageSize},
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
// parsePageOffset
// ---------------------------------------------------------------------------

func TestParsePageOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  int32
	}{
		{"default", "", 0},
		{"custom", "?page_offset=20", 20},
		{"invalid", "?page_offset=abc", 0},
		{"negative", "?page_offset=-1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/"+tt.query, nil)
			got := parsePageOffset(r)

			if got != tt.want {
				t.Errorf("parsePageOffset(%q) = %d, want %d", tt.query, got, tt.want)
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
			rctx.URLParams.Add("user_id", tt.value)
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

			got, err := httpx.ParseUUIDParam(r, "user_id")
			if (err != nil) != tt.wantErr {
				t.Fatalf("httpx.ParseUUIDParam() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && !got.Valid {
				t.Error("UUID should be valid")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToggleBlock — self-block validation
// ---------------------------------------------------------------------------

func TestToggleBlock_SelfBlock(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	ctx := t.Context()
	userID := pgtype.UUID{Valid: true}

	_, err := svc.ToggleBlock(ctx, userID, userID)
	if err == nil {
		t.Fatal("expected error for self-block")
	}

	if !strings.Contains(err.Error(), "cannot block yourself") {
		t.Errorf("error = %q, want contains 'cannot block yourself'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestConstants(t *testing.T) {
	t.Parallel()

	if defaultPageSize != 20 {
		t.Errorf("defaultPageSize = %d, want 20", defaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}
