package publicapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// RateLimitMiddleware enforces per-API-key rate limiting using a fixed-window
// counter backed by Valkey. Keys without a validated context (no X-API-Key header)
// are passed through — they are already covered by the loose tier on /api/v1.
// Keys with an "unlimited" tier bypass the limiter. When Valkey is unreachable
// the middleware degrades gracefully and allows the request through.
func RateLimitMiddleware(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := APIKeyFromContext(r.Context())
			if key == nil || key.RateLimitTier == "unlimited" {
				next.ServeHTTP(w, r)
				return
			}
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			limit := getTierLimit(key.RateLimitTier)
			bucket := fmt.Sprintf("jc:api_rate:%s:%d", key.KeyPrefix, time.Now().Unix()/60)

			count, err := rdb.Incr(r.Context(), bucket).Result()
			if err != nil {
				slog.Warn("api rate limiter: valkey error, allowing request", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				rdb.Expire(r.Context(), bucket, 70*time.Second)
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(0, limit-int(count))))

			if int(count) > limit {
				w.Header().Set("Retry-After", "60")
				writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
