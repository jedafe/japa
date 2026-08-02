package valkey

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// stampedeLockTTL is the TTL for the distributed lock used to prevent cache
// stampedes. It must be long enough for the DB query to complete but short
// enough that a crashed lock holder doesn't block other requests for too long.
const stampedeLockTTL = 5 * time.Second

// stampedeWait is how long a non-lock-holder waits before re-checking the
// cache. The lock holder populates the cache during this window.
const stampedeWait = 100 * time.Millisecond

// GetOrComputeNative is a read-through cache helper with distributed lock
// stampede protection for the native valkey-go Client API. It is designed
// for hot cache keys where multiple concurrent requests may miss the cache
// simultaneously.
//
// Flow:
//  1. Try GET from cache. On hit, unmarshal and return.
//  2. On miss, acquire a short-lived distributed lock (SET NX EX).
//     - If lock acquired: fetch from DB, write to cache, release lock.
//     - If lock NOT acquired: another request is populating the cache.
//       Wait briefly, re-check cache. If still empty, proceed to DB
//       (rare fallback — avoids indefinite blocking).
//
// The fetchFn is only called on a cache miss. The result is cached for
// the given TTL. Cache write failures are logged but never fail the request.
//
// Pass nil for the client to disable caching (degraded mode — fetchFn is
// always called).
func GetOrComputeNative[T any](
	ctx context.Context,
	c valkeygo.Client,
	cacheKey string,
	ttl time.Duration,
	fetchFn func(ctx context.Context) (T, error),
) (T, error) {
	var zero T

	if c == nil {
		return fetchFn(ctx)
	}

	// 1. Try cache.
	if cached, err := c.Do(ctx, c.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
		var result T
		if json.Unmarshal([]byte(cached), &result) == nil {
			return result, nil
		}
	}

	// 2. Acquire distributed lock.
	lockKey := cacheKey + ":lock"
	acquired, _ := c.Do(ctx, c.B().Set().Key(lockKey).Value("1").Nx().Ex(stampedeLockTTL).Build()).ToString()

	if acquired != "OK" {
		// Another request is populating the cache; wait and re-check.
		time.Sleep(stampedeWait)
		if cached, err := c.Do(ctx, c.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
			var result T
			if json.Unmarshal([]byte(cached), &result) == nil {
				return result, nil
			}
		}
		// Lock wait expired; proceed to DB (rare).
	} else {
		// We hold the lock; release it when done.
		defer func() {
			_ = c.Do(ctx, c.B().Del().Key(lockKey).Build()).Error()
		}()
	}

	// 3. Fetch from DB.
	result, err := fetchFn(ctx)
	if err != nil {
		return zero, err
	}

	// 4. Write to cache (best-effort).
	if encoded, err := json.Marshal(result); err == nil {
		if err := c.Do(ctx, c.B().Set().Key(cacheKey).Value(string(encoded)).Ex(ttl).Build()).Error(); err != nil {
			slog.Warn("cachehelper: failed to write cache", "key", cacheKey, "error", err)
		}
	}

	return result, nil
}

// GetOrComputeCompat is the same as GetOrComputeNative but for the
// valkeycompat.Cmdable API (go-redis compatible). Used by packages that
// store a valkeycompat.Cmdable reference (e.g. feed.Service).
func GetOrComputeCompat[T any](
	ctx context.Context,
	rdb valkeycompat.Cmdable,
	cacheKey string,
	ttl time.Duration,
	fetchFn func(ctx context.Context) (T, error),
) (T, error) {
	var zero T

	if rdb == nil {
		return fetchFn(ctx)
	}

	// 1. Try cache.
	if cached, err := rdb.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		var result T
		if json.Unmarshal([]byte(cached), &result) == nil {
			return result, nil
		}
	}

	// 2. Acquire distributed lock.
	lockKey := cacheKey + ":lock"
	acquired, _ := rdb.Set(ctx, lockKey, "1", stampedeLockTTL).Result()

	if acquired != "OK" {
		// Another request is populating the cache; wait and re-check.
		time.Sleep(stampedeWait)
		if cached, err := rdb.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var result T
			if json.Unmarshal([]byte(cached), &result) == nil {
				return result, nil
			}
		}
		// Lock wait expired; proceed to DB (rare).
	} else {
		// We hold the lock; release it when done.
		defer func() {
			_ = rdb.Del(ctx, lockKey).Err()
		}()
	}

	// 3. Fetch from DB.
	result, err := fetchFn(ctx)
	if err != nil {
		return zero, err
	}

	// 4. Write to cache (best-effort).
	if encoded, err := json.Marshal(result); err == nil {
		if err := rdb.Set(ctx, cacheKey, encoded, ttl).Err(); err != nil {
			slog.Warn("cachehelper: failed to write cache", "key", cacheKey, "error", err)
		}
	}

	return result, nil
}
