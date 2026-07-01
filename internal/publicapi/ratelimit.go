package publicapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

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
func SlidingWindowRateLimit(rdb *redis.Client) func(http.Handler) http.Handler {
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

			limit := getTierLimit(key.RateLimitTier)
			now := time.Now()
			windowStart := now.Add(-windowSize)

			// Unique ID for this request: nanosecond timestamp + random suffix
			// to avoid collisions within the sorted set.
			uniqueID := fmt.Sprintf("%d.%d", now.UnixNano(), now.UnixNano()%1000000)
			setKey := fmt.Sprintf("jc:api_rate:%s:%d", key.KeyPrefix, now.Unix()/60)

			ctx := r.Context()

			// Execute the sliding-window logic atomically via MULTI.
			pipe := rdb.Pipeline()
			pipe.ZRemRangeByScore(ctx, setKey, "0", fmt.Sprintf("%d", windowStart.UnixNano()))
			pipe.ZAdd(ctx, setKey, redis.Z{
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
				writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded",
					"Rate limit exceeded. Please retry after "+retryAfter+" seconds.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
