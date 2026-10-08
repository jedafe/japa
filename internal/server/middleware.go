// Package server — HTTP middleware registration.
//
// Middleware is applied in the order registered. The order matters:
//  1. RequestID — generate a unique ID so every log line for a request
//     can be correlated.
//  2. Recovery — catch panics and return a 500 instead of crashing the
//     process. Uses slog for structured logging so panics appear in the
//     same log stream as other application logs.
//  3. Logger — log every request with method, path, status, and duration.
//  4. SecurityHeaders — set nosniff, frame, referrer and CSP headers.
//  5. CORS — allow cross-origin requests from the SvelteKit frontends.
//
// Client IP resolution is NOT a global middleware. chi's RealIP middleware
// was removed due to IP spoofing vulnerabilities (GHSA-3fxj-6jh8-hvhx) — it
// trusted X-Forwarded-For unconditionally. Every consumer that needs the
// real client IP (rate limiters, geoip, spam blocklist, audit logs, etc.)
// resolves it per-request via clientip.FromRequest, which honours
// forwarding headers only when the immediate peer is a trusted proxy
// (JC_TRUSTED_PROXIES). See internal/clientip and JEOS Section 37.
package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	errtracking "github.com/jedafe/japa/internal/errors"
	mw "github.com/jedafe/japa/internal/middleware"
)

// RegisterMiddleware attaches the standard middleware stack to the router.
// Every request passes through these middleware in the order registered.
func RegisterMiddleware(r *chi.Mux) {
	r.Use(chimw.RequestID)
	// chimw.RealIP removed — deprecated due to IP spoofing vulnerabilities (GHSA-3fxj-6jh8-hvhx).
	r.Use(recoveryMiddleware)
	r.Use(chimw.Logger)
	r.Use(mw.SecurityHeaders())
	r.Use(corsMiddleware)
}

// recoveryMiddleware catches panics in handlers so a single bug does not
// crash the entire process. It logs the panic with slog (structured logging)
// and returns a JSON 500 error following the JapaConnect API error format.
//
// http.ErrAbortHandler is re-panicked so the standard library can handle
// intentional connection aborts — this matches the behavior of chi's
// built-in Recoverer.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				if err, ok := rvr.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rvr)
				}

				slog.Error(
					"panic recovered",
					"panic", rvr,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", requestIDFromContext(r.Context()),
				)

				// Forward the panic to GlitchTip (AUDIT-BE-002). No-op when no
				// DSN is configured. Wrap non-error panic values so they carry a
				// message and stack.
				panicErr, ok := rvr.(error)
				if !ok {
					panicErr = fmt.Errorf("panic: %v", rvr)
				}

				errtracking.CaptureException(panicErr)

				errtracking.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware sets the headers that allow the SvelteKit frontends to make
// cross-origin requests to this API. In production the AllowedOrigins list
// should be restricted to the known frontend domains; in development it
// permits all origins so local dev works without configuration.
//
// OPTIONS preflights are short-circuited with a 204 only when the origin is
// allowed, so the browser can proceed with the actual request immediately.
// Preflights from disallowed origins fall through to the router instead:
// routes that registered their own OPTIONS handler under a route-level CORS
// middleware (mcp.PublicCORS on the machine-facing MCP/OAuth endpoints)
// answer with their own policy, and every other route gets chi's 405 — the
// correct rejection for a disallowed preflight. Answering every OPTIONS here
// would shadow those route-level policies and return a bare 204 with no
// Access-Control-Allow-Origin, which fails the browser preflight anyway.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The response body varies by Origin, so caches must key on it.
		w.Header().Add("Vary", "Origin")

		origin := r.Header.Get("Origin")
		allowed := isOriginAllowed(origin)
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", mw.AllowedCORSHeaders())
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "300")
		}

		// Only the global middleware answers preflights for approved origins;
		// disallowed preflights continue down the chain (see doc comment).
		if r.Method == http.MethodOptions && allowed {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isOriginAllowed returns true if the given origin should receive CORS
// headers. The allowlist itself lives in internal/middleware so the
// WebSocket upgrade check uses exactly the same rules.
func isOriginAllowed(origin string) bool {
	return mw.IsOriginAllowed(origin)
}

// requestIDFromContext extracts the Chi request ID from the context for
// use in log messages or response headers. Returns an empty string when
// no request ID has been set, which happens if the RequestID middleware
// was not applied.
//
// This is a helper for future handlers; it is defined here alongside the
// middleware it depends on.
func requestIDFromContext(ctx interface{ Value(any) any }) string {
	v := ctx.Value(chimw.RequestIDKey)

	id, ok := v.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(id)
}
