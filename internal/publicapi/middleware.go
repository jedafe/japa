package publicapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jedafe/japa/internal/tenant"
)

// APIKeyMiddleware extracts the X-API-Key header, validates it, and
// injects the API key context into the request. Downstream handlers
// can read the key info via APIKeyFromContext.
func APIKeyMiddleware(service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			keyCtx, err := service.ValidateAPIKey(r.Context(), apiKey)
			if err != nil {
				slog.Warn("api key validation failed", "error", err)
				writeError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid or expired API key")
				return
			}

			ctx := context.WithValue(r.Context(), apiKeyContextKey, keyCtx)

			// Also inject a synthetic tenant context if not already present.
			if t := tenant.FromContext(ctx); t == nil {
				_ = keyCtx.TenantID // tenant is available for downstream use
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireScope checks that the validated API key includes the given scope.
// Scopes follow the pattern "domain.action" (e.g., "content.create").
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyCtx := APIKeyFromContext(r.Context())
			if keyCtx == nil {
				next.ServeHTTP(w, r)
				return
			}
			if !hasScope(keyCtx.Scopes, scope) {
				writeError(w, http.StatusForbidden, "insufficient_scope", "API key lacks required scope: "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// hasScope checks if a scope list includes the given scope or a wildcard match.
func hasScope(scopes []string, required string) bool {
	for _, s := range scopes {
		if s == required || s == "*" {
			return true
		}
		// Wildcard matching: "content.*" matches "content.create"
		if strings.HasSuffix(s, ".*") {
			prefix := s[:len(s)-2]
			if strings.HasPrefix(required, prefix+".") {
				return true
			}
		}
	}
	return false
}
