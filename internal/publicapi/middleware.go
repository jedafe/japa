package publicapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/redis/go-redis/v9"

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

			// IP allowlist enforcement: if the key has IP restrictions,
			// verify the client IP is within the allowed CIDR ranges.
			clientIP := extractClientIP(r)
			if clientIP != "" {
				allowed, err := service.hasIPAccess(r.Context(), keyCtx.KeyID, clientIP)
				if err != nil {
					slog.Error("api key: IP allowlist check failed", "error", err)
					writeError(w, http.StatusInternalServerError, "ip_check_error", "Failed to verify IP access")
					return
				}
				if !allowed {
					slog.Warn("api key: IP not in allowlist",
						"key_id", keyCtx.KeyID.String(),
						"client_ip", clientIP)
					writeError(w, http.StatusForbidden, "ip_not_allowed", "Your IP address is not authorized for this API key")
					return
				}
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

// extractClientIP extracts the client's IP address from the request.
// It checks X-Forwarded-For, X-Real-IP, and falls back to RemoteAddr.
func extractClientIP(r *http.Request) string {
	// X-Forwarded-For: first IP is the original client.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP from the comma-separated list.
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// X-Real-IP: single client IP set by reverse proxy.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// RemoteAddr: strip port if present.
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
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

// RateLimitMiddleware enforces per-API-key rate limiting using a sliding-window
// counter backed by Valkey sorted sets. Keys without a validated context
// (no X-API-Key header) are passed through — they are already covered by the
// loose tier on /api/v1. Keys with an "unlimited" tier bypass the limiter.
// When Valkey is unreachable the middleware degrades gracefully and allows
// the request through.
//
// Deprecated: Use SlidingWindowRateLimit(rdb) directly. This wrapper
// exists for backward compatibility.
func RateLimitMiddleware(rdb *redis.Client) func(http.Handler) http.Handler {
	return SlidingWindowRateLimit(rdb)
}

// getTierLimit returns the per-minute request limit for a rate limit tier.
func getTierLimit(tier string) int {
	switch tier {
	case "standard":
		return 1000
	case "premium":
		return 10000
	default:
		return 100 // basic
	}
}
