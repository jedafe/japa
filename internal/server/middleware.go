// Package server — HTTP middleware registration.
//
// Middleware is applied in the order registered. The order matters:
//   1. RequestID — generate a unique ID so every log line for a request
//      can be correlated.
//   2. RealIP — extract the real client IP from X-Forwarded-For so logs
//      and rate limiters see the original caller, not the proxy.
//   3. Recovery — catch panics and return a 500 instead of crashing the
//      process. Uses slog for structured logging so panics appear in the
//      same log stream as other application logs.
//   4. Logger — log every request with method, path, status, and duration.
//   5. CORS — allow cross-origin requests from the SvelteKit frontends.
package server

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// RegisterMiddleware attaches the standard middleware stack to the router.
// Every request passes through these middleware in the order registered.
func RegisterMiddleware(r *chi.Mux) {
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(recoveryMiddleware)
	r.Use(chimw.Logger)
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
				if rvr == http.ErrAbortHandler {
					panic(rvr)
				}

				slog.Error("panic recovered",
					"panic", rvr,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", requestIDFromContext(r.Context()),
				)

				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
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
// The middleware short-circuits OPTIONS preflight requests with a 204 so
// the browser can proceed with the actual request immediately.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "300")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isOriginAllowed returns true if the given origin should receive CORS
// headers. In development all origins are allowed. In production the list
// would be restricted to known domains, but for the foundation we allow
// everything and rely on the reverse proxy for origin filtering.
func isOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	// In development, allow all origins for local dev convenience.
	// In production, only allow configured origins.
	env := strings.ToLower(strings.TrimSpace(os.Getenv("JC_ENV")))
	if env == "" || env == "development" {
		return true
	}
	// Production: allow the site itself and common localhost ports for admin tools.
	allowed := []string{
		"https://japaconnect.com",
		"https://www.japaconnect.com",
		"https://admin.japaconnect.com",
		"https://control.japaconnect.com",
		"https://docs.japaconnect.com",
		"https://learn.japaconnect.com",
	}
	for _, a := range allowed {
		if origin == a {
			return true
		}
	}
	return false
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
