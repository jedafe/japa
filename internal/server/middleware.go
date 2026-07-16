// Package server — HTTP middleware registration.
//
// Middleware is applied in the order registered. The order matters:
//  1. RequestID — generate a unique ID so every log line for a request
//     can be correlated.
//  2. RealIP — extract the real client IP from X-Forwarded-For so logs
//     and rate limiters see the original caller, not the proxy.
//  3. Recovery — catch panics and return a 500 instead of crashing the
//     process. Uses slog for structured logging so panics appear in the
//     same log stream as other application logs.
//  4. Logger — log every request with method, path, status, and duration.
//  5. CORS — allow cross-origin requests from the SvelteKit frontends.
package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	errtracking "github.com/jedafe/japa/internal/errors"
)

// RegisterMiddleware attaches the standard middleware stack to the router.
// Every request passes through these middleware in the order registered.
func RegisterMiddleware(r *chi.Mux) {
	r.Use(chimw.RequestID)
	// chimw.RealIP removed — deprecated due to IP spoofing vulnerabilities (GHSA-3fxj-6jh8-hvhx).
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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-Request-ID")
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
// headers. In development, localhost and 127.0.0.1 with any port are allowed.
// In production, only origins from JC_CORS_ORIGINS (comma-separated) are
// permitted. The wildcard "*" is never allowed in production.
func isOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}

	env := strings.ToLower(strings.TrimSpace(os.Getenv("JC_ENV")))
	isProd := env == "production"

	if !isProd {
		// Development: allow localhost and 127.0.0.1 with any port.
		return isLocalOrigin(origin)
	}

	// Production: only allow explicitly configured origins.
	// Never allow wildcard "*".
	if origin == "*" {
		return false
	}

	allowedCSV := os.Getenv("JC_CORS_ORIGINS")
	if allowedCSV == "" {
		// Fall back to production origins derived from JC_SITE_URL if env var not set.
		return isDefaultProductionOrigin(origin, os.Getenv("JC_SITE_URL"))
	}

	for _, allowed := range strings.Split(allowedCSV, ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" || allowed == "*" {
			continue // skip empty entries and never allow wildcard
		}

		if origin == allowed {
			return true
		}
	}

	return false
}

// isLocalOrigin returns true if the origin is localhost or 127.0.0.1
// with any port, for development convenience.
func isLocalOrigin(origin string) bool {
	// Strip scheme for matching.
	stripped := origin
	if idx := strings.Index(origin, "://"); idx != -1 {
		stripped = origin[idx+3:]
	}
	// Remove trailing port if present.
	host := stripped
	if idx := strings.LastIndex(stripped, ":"); idx != -1 {
		host = stripped[:idx]
	}

	return host == "localhost" || host == "127.0.0.1" || host == "[::1]"
}

// isDefaultProductionOrigin checks against the production origins derived
// from JC_SITE_URL when JC_CORS_ORIGINS is not set.
func isDefaultProductionOrigin(origin string, siteURL string) bool {
	if siteURL == "" {
		return false
	}

	// Extract domain from siteURL (e.g. "https://example.com" -> "example.com")
	domain := siteURL
	if idx := len("https://"); len(domain) > idx && domain[:idx] == "https://" {
		domain = domain[idx:]
	} else if idx := len("http://"); len(domain) > idx && domain[:idx] == "http://" {
		domain = domain[idx:]
	}
	// Remove trailing path
	for i, c := range domain {
		if c == '/' {
			domain = domain[:i]
			break
		}
	}

	// Generate allowed origins from domain
	allowed := []string{
		"https://" + domain,
		"https://www." + domain,
		"https://admin." + domain,
		"https://control." + domain,
		"https://docs." + domain,
		"https://learn." + domain,
		"https://api." + domain,
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
