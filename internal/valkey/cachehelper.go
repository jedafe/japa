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

// stampedeWait is how long a non-lock-holder waits between re-checking the
// cache. The lock holder populates the cache during this window. The wait is
// repeated in a bounded loop (see waitForCache) until the cache is populated,
// the lock holder dies (lock key gone), or the lock TTL elapses.
const stampedeWait = 100 * time.Millisecond

// waitForCacheNative polls the cache for a populated value while another
// request holds the distributed lock. It returns the decoded value when the
// lock holder writes it. If the lock holder dies (lock key disappears) or the
// total wait budget (stampedeLockTTL) elapses without a cache write, it
// returns false and the caller must fetch itself (rare fallback).
//
// The loop is context-aware: a cancelled context aborts the wait early.
func waitForCacheNative[T any](ctx context.Context, c valkeygo.Client, cacheKey, lockKey string) (T, bool) {
	var zero T
	deadline := time.Now().Add(stampedeLockTTL)
	ticker := time.NewTicker(stampedeWait)
	defer ticker.Stop()

	for {
		// Stop waiting if the caller's context is done or the lock TTL
		// has elapsed (the holder is presumed dead).
		if ctx.Err() != nil || time.Now().After(deadline) {
			return zero, false
		}

		// If the lock key is gone but the cache is still empty, the
		// holder finished without writing (error path) — take over.
		lockExists, _ := c.Do(ctx, c.B().Get().Key(lockKey).Build()).ToString()
		if lockExists == "" {
			// Lock released — one last cache check before taking over.
			if cached, err := c.Do(ctx, c.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
				var result T
				if json.Unmarshal([]byte(cached), &result) == nil {
					return result, true
				}
			}
			return zero, false
		}

		select {
		case <-ctx.Done():
			return zero, false
		case <-ticker.C:
		}

		// Re-check the cache after the tick.
		if cached, err := c.Do(ctx, c.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
			var result T
			if json.Unmarshal([]byte(cached), &result) == nil {
				return result, true
			}
		}
	}
}

// waitForCacheCompat is the valkeycompat.Cmdable equivalent of
// waitForCacheNative.
func waitForCacheCompat[T any](ctx context.Context, rdb valkeycompat.Cmdable, cacheKey, lockKey string) (T, bool) {
	var zero T
	deadline := time.Now().Add(stampedeLockTTL)
	ticker := time.NewTicker(stampedeWait)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil || time.Now().After(deadline) {
			return zero, false
		}

		// Lock released? Do a final cache check before taking over.
		if err := rdb.Get(ctx, lockKey).Err(); err != nil {
			if cached, err := rdb.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
				var result T
				if json.Unmarshal([]byte(cached), &result) == nil {
					return result, true
				}
			}
			return zero, false
		}

		select {
		case <-ctx.Done():
			return zero, false
		case <-ticker.C:
		}

		if cached, err := rdb.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var result T
			if json.Unmarshal([]byte(cached), &result) == nil {
				return result, true
			}
		}
	}
}

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
//     Poll the cache in a bounded loop (waitForCacheNative) until the
//     value appears, the holder dies, or the lock TTL elapses. Only then
//     (rare) does the caller fetch itself.
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

	// 2. Acquire distributed lock (SET NX EX — only one request wins).
	lockKey := cacheKey + ":lock"
	acquired, _ := c.Do(ctx, c.B().Set().Key(lockKey).Value("1").Nx().Ex(stampedeLockTTL).Build()).ToString()

	if acquired != "OK" {
		// Another request is populating the cache; wait for it in a
		// bounded, context-aware loop instead of a single sleep.
		if result, ok := waitForCacheNative[T](ctx, c, cacheKey, lockKey); ok {
			return result, nil
		}
		// Lock holder died or TTL elapsed — fall through and fetch.
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
//
// Flow:
//  1. Try GET from cache. On hit, unmarshal and return.
//  2. On miss, acquire a short-lived distributed lock with SetNX (SET NX EX).
//     - If lock acquired: fetch from DB, write to cache, release lock.
//     - If lock NOT acquired: another request is populating the cache.
//     Poll the cache in a bounded loop (waitForCacheCompat) until the
//     value appears, the holder dies, or the lock TTL elapses. Only then
//     (rare) does the caller fetch itself.
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

	// 2. Acquire distributed lock with SET NX EX so only one request
	//    populates the cache. A plain SET would always succeed and defeat
	//    the stampede protection (VALKEY-LOCK bug). SetNX returns true only
	//    when the key did not already exist.
	lockKey := cacheKey + ":lock"
	acquired, _ := rdb.SetNX(ctx, lockKey, "1", stampedeLockTTL).Result()

	if !acquired {
		// Another request is populating the cache; wait for it in a
		// bounded, context-aware loop instead of a single sleep.
		if result, ok := waitForCacheCompat[T](ctx, rdb, cacheKey, lockKey); ok {
			return result, nil
		}
		// Lock holder died or TTL elapsed — fall through and fetch.
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
