package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// startTestCompat spins up an in-memory Valkey (miniredis) and returns a
// go-redis-compatible Cmdable so the cache helpers can be tested without a
// real server. Mirrors the helper in internal/auth/bruteforce_valkey_test.go.
func startTestCompat(t *testing.T) valkeycompat.Cmdable {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}

	compat := valkeycompat.NewAdapter(vk)
	t.Cleanup(func() { vk.Close() })

	return compat
}

// startTestNative returns the native valkey-go Client backed by miniredis,
// for testing GetOrComputeNative.
func startTestNative(t *testing.T) valkey.Client {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(func() { vk.Close() })

	return vk
}

// mustJSON marshals v for test seeding.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// TestGetOrComputeCompat_NilClientBypassesCache verifies that a nil Cmdable
// always calls fetchFn (degraded mode).
func TestGetOrComputeCompat_NilClientBypassesCache(t *testing.T) {
	t.Parallel()

	var calls int32
	got, err := GetOrComputeCompat[int](t.Context(), nil, "k", time.Minute, func(ctx context.Context) (int, error) {
		atomic.AddInt32(&calls, 1)
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 fetchFn call, got %d", calls)
	}
}

// TestGetOrComputeCompat_CacheHitSkipsFetch verifies a cache hit returns the
// cached value without invoking fetchFn.
func TestGetOrComputeCompat_CacheHitSkipsFetch(t *testing.T) {
	t.Parallel()
	rdb := startTestCompat(t)
	ctx := t.Context()

	// Pre-seed the cache.
	if err := rdb.Set(ctx, "jc:test:hit", mustJSON(t, 99), time.Minute).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var calls int32
	got, err := GetOrComputeCompat[int](ctx, rdb, "jc:test:hit", time.Minute, func(ctx context.Context) (int, error) {
		atomic.AddInt32(&calls, 1)
		return 0, errors.New("should not be called")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 99 {
		t.Fatalf("expected cached 99, got %d", got)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("fetchFn should not be called on cache hit, got %d calls", calls)
	}
}

// TestGetOrComputeCompat_StampedeProtection is the regression test for the
// broken distributed lock. Under concurrent cache misses, only ONE goroutine
// should call fetchFn (the lock holder); the rest should wait and read the
// populated cache. Before the fix, rdb.Set always returned "OK" so every
// goroutine acquired the "lock" and stampeded the DB.
func TestGetOrComputeCompat_StampedeProtection(t *testing.T) {
	t.Parallel()
	rdb := startTestCompat(t)
	ctx := t.Context()

	var fetchCalls int32
	const concurrency = 20

	// fetchFn is slow so the waiters actually have to wait.
	slowFetch := func(ctx context.Context) (int, error) {
		atomic.AddInt32(&fetchCalls, 1)
		time.Sleep(150 * time.Millisecond)
		return 7, nil
	}

	var wg sync.WaitGroup
	wg.Add(concurrency)
	results := make([]int, concurrency)
	errs := make([]error, concurrency)
	start := make(chan struct{})

	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()
			<-start // release all goroutines together to maximise contention.
			v, err := GetOrComputeCompat[int](ctx, rdb, "jc:test:stampede", 5*time.Second, slowFetch)
			results[idx] = v
			errs[idx] = err
		}(i)
	}

	close(start)
	wg.Wait()

	// Every goroutine must observe the correct result.
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
		if results[i] != 7 {
			t.Fatalf("goroutine %d: expected 7, got %d", i, results[i])
		}
	}

	// The whole point of the lock: exactly ONE fetchFn call.
	if got := atomic.LoadInt32(&fetchCalls); got != 1 {
		t.Fatalf("stampede protection failed: expected 1 fetchFn call, got %d "+
			"(distributed lock not exclusive — was rdb.Set used instead of SetNX?)", got)
	}
}

// TestGetOrComputeCompat_FetchErrorPropagates verifies that a fetchFn error
// is returned and the cache is not poisoned.
func TestGetOrComputeCompat_FetchErrorPropagates(t *testing.T) {
	t.Parallel()
	rdb := startTestCompat(t)
	ctx := t.Context()

	sentinel := errors.New("db down")
	_, err := GetOrComputeCompat[int](ctx, rdb, "jc:test:err", time.Minute, func(ctx context.Context) (int, error) {
		return 0, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}

	// Cache should not have been written.
	if rdb.Get(ctx, "jc:test:err").Err() == nil {
		t.Fatal("cache should not be poisoned by a fetch error")
	}
}

// TestGetOrComputeCompat_LockKeyReleased verifies the lock key is deleted
// after a successful fetch so a subsequent miss can re-acquire it.
func TestGetOrComputeCompat_LockKeyReleased(t *testing.T) {
	t.Parallel()
	rdb := startTestCompat(t)
	ctx := t.Context()

	_, err := GetOrComputeCompat[int](ctx, rdb, "jc:test:release", time.Minute, func(ctx context.Context) (int, error) {
		return 1, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The lock key should be gone after the holder finishes.
	if err := rdb.Get(ctx, "jc:test:release:lock").Err(); err == nil {
		t.Fatal("lock key should be released after fetch completes")
	}
}

// TestGetOrComputeNative_StampedeProtection is the native-API equivalent,
// guarding against regressions in the already-correct .Nx().Ex() path.
func TestGetOrComputeNative_StampedeProtection(t *testing.T) {
	t.Parallel()
	c := startTestNative(t)
	ctx := t.Context()

	var fetchCalls int32
	const concurrency = 20

	slowFetch := func(ctx context.Context) (int, error) {
		atomic.AddInt32(&fetchCalls, 1)
		time.Sleep(150 * time.Millisecond)
		return 5, nil
	}

	var wg sync.WaitGroup
	wg.Add(concurrency)
	results := make([]int, concurrency)
	errs := make([]error, concurrency)
	start := make(chan struct{})

	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()
			<-start
			v, err := GetOrComputeNative[int](ctx, c, "jc:test:native:stampede", 5*time.Second, slowFetch)
			results[idx] = v
			errs[idx] = err
		}(i)
	}

	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
		if results[i] != 5 {
			t.Fatalf("goroutine %d: expected 5, got %d", i, results[i])
		}
	}

	if got := atomic.LoadInt32(&fetchCalls); got != 1 {
		t.Fatalf("expected 1 fetchFn call, got %d", got)
	}
}
