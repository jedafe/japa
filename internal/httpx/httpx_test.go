package httpx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jedafe/japa/internal/httpx"
)

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	httpx.WriteJSON(rec, http.StatusCreated, map[string]string{"id": "abc"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body["id"] != "abc" {
		t.Fatalf("body = %v, want id=abc", body)
	}
}

func TestWriteJSONNilBody(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	httpx.WriteJSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	httpx.WriteError(rec, http.StatusBadRequest, "invalid_json", "Invalid JSON body")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body httpx.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Error.Code != "invalid_json" || body.Error.Message != "Invalid JSON body" {
		t.Fatalf("body = %+v, want invalid_json/Invalid JSON body", body.Error)
	}
}

func TestParseUUIDParam(t *testing.T) {
	t.Parallel()

	const valid = "11111111-1111-1111-1111-111111111111"

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid", value: valid},
		{name: "invalid", value: "not-a-uuid", wantErr: true},
		{name: "missing", value: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tc.value)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			id, err := httpx.ParseUUIDParam(req, "id")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !id.Valid {
				t.Fatal("expected a valid uuid")
			}
		})
	}
}

func TestParseOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  int32
	}{
		{query: "", want: 0},
		{query: "?offset=25", want: 25},
		{query: "?offset=-5", want: 0},
		{query: "?offset=abc", want: 0},
		{query: "?offset=2147483648", want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/"+tc.query, nil)
			if got := httpx.ParseOffset(req); got != tc.want {
				t.Fatalf("ParseOffset(%q) = %d, want %d", tc.query, got, tc.want)
			}
		})
	}
}
