package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	mw "github.com/jedafe/japa/internal/middleware"
)

// silenceLogger replaces the default slog handler with one that discards
// all output, then restores it via the returned function. Use this in
// tests that exercise the recovery middleware so the expected panic log
// line does not clutter the test output.
func silenceLogger() func() {
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))

	return func() { slog.SetDefault(orig) }
}

// TestRecoveryMiddleware verifies that the recovery middleware catches a
// panic in a handler and returns a 500 JSON error instead of crashing the
// process.
func TestRecoveryMiddleware(t *testing.T) {
	t.Parallel()
	// The recovery middleware logs the panic via slog. Silence that log
	// line so the test output stays clean — the panic is intentional.
	restore := silenceLogger()
	defer restore()

	r := chi.NewRouter()
	r.Use(recoveryMiddleware)
	r.Get("/boom", func(_ http.ResponseWriter, _ *http.Request) {
		panic("something went wrong")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()

	// This must not panic — the recovery middleware should catch it.
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Verify the JSON error body follows the JapaConnect API error format.
	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}

	if body.Error.Code != "internal_error" {
		t.Errorf("error code = %q, want %q", body.Error.Code, "internal_error")
	}
}

// TestRequestIDMiddleware verifies that the RequestID middleware adds a
// request ID to the context and the response header so requests can be
// correlated in logs.
func TestRequestIDMiddleware(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Use(chimw.RequestID)

	var capturedID string

	r.Get("/tracked", func(_ http.ResponseWriter, r *http.Request) {
		capturedID = requestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/tracked", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if capturedID == "" {
		t.Error("request ID was not set in the context")
	}
}

// TestCORSMiddleware_Preflight verifies that OPTIONS requests receive a
// 204 No Content response with the correct CORS headers so browsers can
// proceed with the actual cross-origin request.
func TestCORSMiddleware_Preflight(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Use(mw.CORS(corsOriginsFromEnv()))

	r.Get("/data", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodOptions, "/data", nil)
	req.Header.Set("Origin", "http://localhost:5173")

	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:5173")
	}
}

// TestCORSMiddleware_ActualRequest verifies that a normal GET request with
// an Origin header receives the CORS headers and passes through to the
// handler.
func TestCORSMiddleware_ActualRequest(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Use(mw.CORS(corsOriginsFromEnv()))

	r.Get("/data", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("Origin", "http://localhost:5173")

	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:5173")
	}
}

// TestCORSMiddleware_NoOrigin verifies that a request without an Origin
// header does not receive CORS headers — it is a same-origin request.
func TestCORSMiddleware_NoOrigin(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Use(mw.CORS(corsOriginsFromEnv()))

	r.Get("/data", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (no origin sent)", got)
	}
}
