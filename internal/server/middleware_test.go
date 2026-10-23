package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jedafe/japa/internal/csrf"
	errtracking "github.com/jedafe/japa/internal/errors"
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
	// Not parallel: t.Setenv mutates process-global env vars. JC_ENV is
	// pinned to a dev-like value so localhost origins are allowed.
	t.Setenv("JC_ENV", "test")

	r := chi.NewRouter()
	r.Use(corsMiddleware)

	r.Get("/data", func(w http.ResponseWriter, _ *http.Request) {
		errtracking.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	for _, h := range []string{"Content-Type", "Authorization", "X-API-Key", "X-Device-Id"} {
		if !strings.Contains(allowHeaders, h) {
			t.Errorf("Access-Control-Allow-Headers missing %q: %q", h, allowHeaders)
		}
	}
}

// TestCORSMiddleware_PreflightAllowed verifies that an OPTIONS preflight
// from an allowlisted (non-localhost) origin is still answered by the global
// middleware with a 204 and the allowlist CORS headers. The environment is
// pinned to production so the allowlist is deterministic.
func TestCORSMiddleware_PreflightAllowed(t *testing.T) {
	// Not parallel: t.Setenv mutates process-global env vars.
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_CORS_ORIGINS", "https://app.example.com")

	r := chi.NewRouter()
	r.Use(corsMiddleware)

	r.Get("/data", func(w http.ResponseWriter, _ *http.Request) {
		errtracking.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodOptions, "/data", nil)
	req.Header.Set("Origin", "https://app.example.com")

	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
}

// TestCORSMiddleware_PreflightRoutePolicy verifies that an OPTIONS preflight
// from a disallowed origin still reaches a route that registered its own
// OPTIONS handler under a route-level CORS middleware — the mcp.PublicCORS
// pattern on /oauth/* and /.well-known/*. The route's own permissive policy
// answers the preflight instead of the global middleware's bare 204 (which
// carried no Access-Control-Allow-Origin and failed browser preflights).
func TestCORSMiddleware_PreflightRoutePolicy(t *testing.T) {
	// Not parallel: t.Setenv mutates process-global env vars.
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_CORS_ORIGINS", "https://app.example.com")

	// Mirrors mcp.PublicCORS: permissive ACAO:* policy that answers OPTIONS
	// itself. Simulated rather than imported to keep this test focused on
	// the global middleware's pass-through behavior.
	routeRan := false
	publicCORS := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routeRan = true

			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	r := chi.NewRouter()
	r.Use(corsMiddleware)

	r.With(publicCORS).Options("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	r.With(publicCORS).Post("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/oauth/token", nil)
	req.Header.Set("Origin", "https://mcp-client.example.com")

	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if !routeRan {
		t.Fatal("route-level CORS middleware never ran — the global middleware shadowed the preflight")
	}

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q (route-level policy must answer the preflight)", got, "*")
	}
}

// TestCORSMiddleware_PreflightDisallowedNoRoute verifies that an OPTIONS
// preflight from a disallowed origin to a route without an OPTIONS handler
// falls through to the router, which answers 405 — not the global
// middleware's bare 204 with no CORS headers.
func TestCORSMiddleware_PreflightDisallowedNoRoute(t *testing.T) {
	// Not parallel: t.Setenv mutates process-global env vars.
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_CORS_ORIGINS", "https://app.example.com")

	r := chi.NewRouter()
	r.Use(corsMiddleware)

	r.Get("/data", func(w http.ResponseWriter, _ *http.Request) {
		errtracking.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodOptions, "/data", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for a disallowed origin", got)
	}
}

// TestCORSMiddleware_ActualRequest verifies that a normal GET request with
// an Origin header receives the CORS headers and passes through to the
// handler.
func TestCORSMiddleware_ActualRequest(t *testing.T) {
	// Not parallel: t.Setenv mutates process-global env vars. JC_ENV is
	// pinned to a dev-like value so localhost origins are allowed.
	t.Setenv("JC_ENV", "test")

	r := chi.NewRouter()
	r.Use(corsMiddleware)

	r.Get("/data", func(w http.ResponseWriter, _ *http.Request) {
		errtracking.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	r.Use(corsMiddleware)

	r.Get("/data", func(w http.ResponseWriter, _ *http.Request) {
		errtracking.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (no origin sent)", got)
	}
}

// TestCSRFMiddleware_ExemptContentView verifies that the public
// POST /api/v1/content/{id}/view ping is exempt from the double-submit
// CSRF check. Anonymous browsers fire this endpoint without a Bearer token
// or CSRF cookie, so it must not be rejected with a 403.
func TestCSRFMiddleware_ExemptContentView(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Use(csrf.Middleware([]string{"/"}, csrfExempt, csrf.WithOriginCheck(mw.IsOriginAllowed)))
	r.Post("/api/v1/content/{id}/view", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/content/123e4567-e89b-12d3-a456-426614174000/view", nil)
	req.AddCookie(&http.Cookie{Name: csrf.CookieName, Value: "stale-token"})

	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
