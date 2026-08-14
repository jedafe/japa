package publicapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// rateLimitSettingsReader is the subset of config.Settings the rate limiter
// needs to read per-tier limits from system_settings (SEC-012 / Rule 3).
// *config.Settings satisfies this structurally. When nil, compiled-in
// defaults are used (preserving the pre-settings behaviour).
type rateLimitSettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// SlidingWindowRateLimit returns middleware that enforces per-API-key rate
// limiting using a sliding-window counter backed by Valkey sorted sets.
// Keys without a validated context (no X-API-Key header) pass through —
// they are already covered by the loose tier on /api/v1. Keys with an
// "unlimited" tier bypass the limiter entirely. When Valkey is unreachable
// the middleware degrades gracefully and allows the request through.
//
// The sorted set approach avoids the boundary-burst problem of fixed-window
// counters: a client cannot send N requests at the end of one window and
// N more at the start of the next and get 2N requests through in under
// 2×window time.
//
// Per-tier limits use compiled-in defaults. Use SlidingWindowRateLimitWithSettings
// to read limits from system_settings (SEC-012).
func SlidingWindowRateLimit(rdb valkeycompat.Cmdable) func(http.Handler) http.Handler {
	return SlidingWindowRateLimitWithSettings(rdb, nil)
}

// SlidingWindowRateLimitWithSettings is like SlidingWindowRateLimit but reads
// per-tier request limits from system_settings (`api.rate_limit.basic`,
// `api.rate_limit.standard`, `api.rate_limit.premium`) when a settings reader
// is wired. Falls back to compiled-in defaults otherwise (SEC-012 / Rule 3).
func SlidingWindowRateLimitWithSettings(rdb valkeycompat.Cmdable, sr rateLimitSettingsReader) func(http.Handler) http.Handler {
	const windowSize = time.Minute

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

			limit := resolveTierLimit(r.Context(), key.RateLimitTier, key.TenantID, sr)
			now := time.Now()
			windowStart := now.Add(-windowSize)

			// Unique ID for this request: nanosecond timestamp + 8 random bytes
			// to avoid collisions within the sorted set under high concurrency
			// (PUB-017 — the previous timestamp-only suffix could collide if two
			// requests arrived in the same nanosecond).
			randBytes := make([]byte, 8)
			_, _ = rand.Read(randBytes)
			uniqueID := fmt.Sprintf("%d.%s", now.UnixNano(), hex.EncodeToString(randBytes))
			setKey := valkey.MustKey("api_rate:%s:%d", key.KeyPrefix, now.Unix()/60)

			ctx := r.Context()

			// Execute the sliding-window logic atomically via MULTI.
			pipe := rdb.Pipeline()
			pipe.ZRemRangeByScore(ctx, setKey, "0", strconv.FormatInt(windowStart.UnixNano(), 10))
			pipe.ZAdd(ctx, setKey, valkeycompat.Z{
				Score:  float64(now.UnixNano()),
				Member: uniqueID,
			})
			pipe.Expire(ctx, setKey, windowSize+time.Second)
			zCardCmd := pipe.ZCard(ctx, setKey)

			if _, err := pipe.Exec(ctx); err != nil {
				slog.Warn("api sliding rate limiter: valkey error, allowing request", "error", err)
				next.ServeHTTP(w, r)

				return
			}

			count := zCardCmd.Val()

			remaining := int64(limit) - count
			if remaining < 0 {
				remaining = 0
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

			if count > int64(limit) {
				retryAfter := strconv.FormatInt(int64(windowSize.Seconds()), 10)
				w.Header().Set("Retry-After", retryAfter)
				errpkg.WriteError(w, http.StatusTooManyRequests, "rate_limit_exceeded",
					"Rate limit exceeded. Please retry after "+retryAfter+" seconds.")

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
