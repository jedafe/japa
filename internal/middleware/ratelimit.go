// Package middleware provides cross-cutting HTTP middleware for JapaConnect.
//
// The rate limiter uses a fixed-window counter backed by Valkey (Redis
// protocol). Each caller is identified by user ID when authenticated,
// falling back to client IP. When Valkey is unreachable the middleware
// degrades gracefully and allows the request through — a missing cache
// layer should never take the API down, mirroring the graceful-degradation
// pattern established by the GeoIP reader and Valkey client wrapper.
//
// Every rate-limited key is namespaced with the "jc:ratelimit:" prefix
// per the Valkey key convention documented in internal/valkey/client.go.
package middleware

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/valkey"
)

// RateLimitTier defines the maximum number of requests allowed within a
// given time window for a named tier.
type RateLimitTier struct {
	Requests int
	Window   time.Duration
}

// Tiers maps tier names to their rate limit configuration. Every new
// endpoint must be assigned one of these tiers at design time, per
// ADR-022. The values are deliberately generous for legitimate clients
// while still throttling abusive burst traffic.
var Tiers = map[string]RateLimitTier{
	"tight":           {Requests: 10, Window: time.Minute},
	"moderate":        {Requests: 60, Window: time.Minute},
	"loose":           {Requests: 300, Window: time.Minute},
	"webhook":         {Requests: 3000, Window: time.Minute},
	"form_submission": {Requests: 10, Window: time.Hour}, // §06: 10 submissions per user per hour
}

// RateLimit returns middleware that enforces a fixed-window rate limit
// for the given tier. The identifier is the authenticated user's ID when
// available, otherwise the client IP resolved by clientip.FromRequest,
// which honours X-Forwarded-For / X-Real-IP only when the immediate peer
// is a trusted proxy (JC_TRUSTED_PROXIES).
//
// When Valkey is unreachable, the middleware allows the request through
// (graceful degradation). When the tier name is unknown, a warning is
// logged at construction time and the returned middleware is a no-op
// pass-through — this is a programming error, not a runtime failure.
func RateLimit(rdb valkeycompat.Cmdable, tier string) func(http.Handler) http.Handler {
	limit, ok := Tiers[tier]
	if !ok {
		slog.Warn("rate limiter: unknown tier, allowing all traffic",
			"tier", tier)

		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}
	}

	windowSeconds := int64(limit.Window.Seconds())

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			identifier := callerIdentifier(r)

			// Exempt loopback callers from the rate limit. This lets the
			// SSR frontend (which calls the API from 127.0.0.1/::1 with no
			// X-Forwarded-For) fire the burst of parallel requests a page
			// load requires without exhausting the shared per-IP counter.
			//
			// This is safe in production: when a reverse proxy fronts the
			// API, clientip.FromRequest resolves the real client IP from
			// X-Forwarded-For (the proxy is a trusted peer), so the
			// identifier is the public client address — not loopback — and
			// the limit still applies. Only direct loopback connections
			// without a forwarding header (i.e. co-located SSR or local dev)
			// are exempt.
			if isLoopbackIdentifier(identifier) {
				next.ServeHTTP(w, r)
				return
			}

			bucket := time.Now().Unix() / windowSeconds
			key := valkey.MustKey("ratelimit:%s:%s:%d", tier, identifier, bucket)

			ctx := r.Context()

			// Pipeline INCR + EXPIRE in a single round trip. The first
			// request in a window initialises the counter and sets the
			// TTL so the key auto-cleans after the window expires.
			pipe := rdb.Pipeline()
			incrCmd := pipe.Incr(ctx, key)
			pipe.Expire(ctx, key, limit.Window+time.Second)

			if _, err := pipe.Exec(ctx); err != nil {
				// Valkey unreachable — degrade gracefully.
				next.ServeHTTP(w, r)
				return
			}

			count := incrCmd.Val()

			remaining := int64(limit.Requests) - count
			if remaining < 0 {
				remaining = 0
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit.Requests))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

			if count > int64(limit.Requests) {
				retryAfter := strconv.FormatInt(windowSeconds, 10)
				w.Header().Set("Retry-After", retryAfter)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{
						"code":    "rate_limited",
						"message": "Rate limit exceeded. Please retry after " + retryAfter + " seconds.",
					},
				})

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// callerIdentifier returns a stable per-caller key segment. Authenticated
// users are identified by their user ID; anonymous callers fall back to
// the client IP resolved by clientip.FromRequest, which honours
// X-Forwarded-For / X-Real-IP only when the peer is a trusted proxy.
func callerIdentifier(r *http.Request) string {
	if user := auth.FromContext(r.Context()); user != nil && user.UserID.Valid {
		return "user:" + user.UserID.String()
	}

	return "ip:" + clientip.FromRequest(r)
}

// isLoopbackIdentifier reports whether the caller identifier refers to a
// loopback IP address (127.0.0.0/8 or ::1). Authenticated users are never
// loopback. Anonymous identifiers use the "ip:" prefix produced by
// callerIdentifier; the address there is the client IP resolved by
// clientip.FromRequest, which is loopback only for direct local connections
// without a forwarding header.
func isLoopbackIdentifier(identifier string) bool {
	const ipPrefix = "ip:"
	if len(identifier) <= len(ipPrefix) || identifier[:len(ipPrefix)] != ipPrefix {
		return false
	}

	ip := net.ParseIP(identifier[len(ipPrefix):])
	if ip == nil {
		return false
	}

	return ip.IsLoopback()
}
