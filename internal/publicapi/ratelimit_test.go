package publicapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// mockRedisClient creates a real Redis client for integration tests.
// Falls back to a no-op handler if Valkey is unreachable (tests degrade
// gracefully just like the middleware itself).
func mockRedisClient() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil // Valkey not available — tests will skip
	}
	return rdb
}

func TestSlidingWindowRateLimit_BasicTier(t *testing.T) {
	rdb := mockRedisClient()
	if rdb == nil {
		t.Skip("Valkey not available, skipping integration test")
	}
	defer rdb.Close()

	// Clean up test keys.
	ctx := context.Background()
	keys, _ := rdb.Keys(ctx, "jc:api_rate:test_basic_*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	handler := SlidingWindowRateLimit(rdb)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Simulate a basic-tier API key.
	keyCtx := &APIKeyContext{
		KeyPrefix:     "test_basic_" + time.Now().Format("150405"),
		RateLimitTier: "basic",
	}

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 101st request should be rate limited.
	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 101: expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestSlidingWindowRateLimit_ResponseHeaders(t *testing.T) {
	rdb := mockRedisClient()
	if rdb == nil {
		t.Skip("Valkey not available, skipping integration test")
	}
	defer rdb.Close()

	ctx := context.Background()
	prefix := "test_headers_" + time.Now().Format("150405")
	keys, _ := rdb.Keys(ctx, "jc:api_rate:"+prefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	handler := SlidingWindowRateLimit(rdb)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	keyCtx := &APIKeyContext{
		KeyPrefix:     prefix,
		RateLimitTier: "basic",
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("X-RateLimit-Limit") != "100" {
		t.Errorf("expected X-RateLimit-Limit 100, got %s", w.Header().Get("X-RateLimit-Limit"))
	}
	if w.Header().Get("X-RateLimit-Remaining") != "99" {
		t.Errorf("expected X-RateLimit-Remaining 99, got %s", w.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestSlidingWindowRateLimit_UnlimitedTierBypasses(t *testing.T) {
	rdb := mockRedisClient()
	if rdb == nil {
		t.Skip("Valkey not available, skipping integration test")
	}
	defer rdb.Close()

	handler := SlidingWindowRateLimit(rdb)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	keyCtx := &APIKeyContext{
		KeyPrefix:     "test_unlimited",
		RateLimitTier: "unlimited",
	}

	// Send 200 requests — all should pass.
	for i := 0; i < 200; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 for unlimited tier, got %d", i+1, w.Code)
		}
	}
}

func TestSlidingWindowRateLimit_NilKeyPassesThrough(t *testing.T) {
	rdb := mockRedisClient()
	if rdb == nil {
		t.Skip("Valkey not available, skipping integration test")
	}
	defer rdb.Close()

	handler := SlidingWindowRateLimit(rdb)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No API key in context — should pass through.
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for nil key context, got %d", w.Code)
	}
}

func TestSlidingWindowRateLimit_NilRedisDegradesGracefully(t *testing.T) {
	handler := SlidingWindowRateLimit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	keyCtx := &APIKeyContext{
		KeyPrefix:     "test_nil_redis",
		RateLimitTier: "basic",
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when Redis is nil, got %d", w.Code)
	}
}

func TestGetTierLimit(t *testing.T) {
	tests := []struct {
		tier     string
		expected int
	}{
		{"basic", 100},
		{"standard", 1000},
		{"premium", 10000},
		{"unlimited", 100}, // unlimited bypasses before this is called, but default falls back
		{"unknown", 100},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			got := getTierLimit(tt.tier)
			if got != tt.expected {
				t.Errorf("getTierLimit(%q) = %d, want %d", tt.tier, got, tt.expected)
			}
		})
	}
}
