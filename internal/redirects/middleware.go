package redirects

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// maxPathLength is the maximum path length the redirect middleware will check.
// Per ADR-056: "Path length sanitized to prevent cache key flooding (max 2048 chars).".
const maxPathLength = 2048

// Middleware returns a Chi middleware that resolves admin-managed URL redirects
// (ADR-056) before the request reaches route handlers. It runs early in the
// chain — after the standard middleware (RequestID, Recovery, Logger, Security,
// CORS) and IP block middleware, but before the Tenant/Auth middleware groups.
//
// Requests that do not match a redirect rule pass through unchanged.
//
// Paths under /api/, /health, /ready, and /seaweedfs/ are skipped — those are
// infrastructure/API paths that must never be redirected.
//
// Ref: japaconnect-engineering/adr/ADR-056-url-redirects.md.
func Middleware(service *Service, store any) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Skip API, infrastructure, and internal paths.
			if shouldSkipPath(path) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip overly long paths (ADR-056 cache key flooding mitigation).
			if len(path) > maxPathLength {
				next.ServeHTTP(w, r)
				return
			}

			targetURL, httpStatus := service.ResolveRedirect(r.Context(), pgtype.UUID{}, path)
			if targetURL == "" || httpStatus == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// targetURL is safe: it originates from the redirects table, which is
			// validated by validateTargetURL at create/update time (only root-relative
			// paths and absolute http/https URLs are allowed). The Valkey cache is
			// populated exclusively from DB reads. No unvalidated user input reaches
			// this call. gosec G710 is a false positive here.
			http.Redirect(w, r, targetURL, httpStatus) // #nosec G710 -- target_url validated at storage time (ADR-056)
		})
	}
}

// shouldSkipPath returns true for paths that must never be redirected.
func shouldSkipPath(path string) bool {
	if path == "" {
		return true
	}
	// API routes — never redirect.
	if strings.HasPrefix(path, "/api/") {
		return true
	}
	// Infrastructure endpoints.
	if path == "/health" || path == "/ready" {
		return true
	}
	// SeaweedFS upload fallback.
	if strings.HasPrefix(path, "/seaweedfs/") {
		return true
	}

	return false
}
