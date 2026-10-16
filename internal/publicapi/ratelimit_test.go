package publicapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	valkeypkg "github.com/jedafe/japa/internal/valkey"
)

// fakeTierResolver is a stub TierResolver for middleware tests.
type fakeTierResolver struct {
	tier string
}

func (f *fakeTierResolver) ResolveUserTier(_ context.Context, _ pgtype.UUID) (string, error) {
	return f.tier, nil
}

// mockRedisClient creates a real Redis client for integration tests.
// Falls back to a no-op handler if Valkey is unreachable (tests degrade
// gracefully just like the middleware itself).
func mockRedisClient() valkeycompat.Cmdable {
	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{"localhost:6379"},
	})
	if err != nil {
		return nil
	}

	compat := valkeycompat.NewAdapter(vk)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := compat.Ping(ctx).Err(); err != nil {
		vk.Close()
		return nil // Valkey not available — tests will skip
	}

	return compat
}

func TestSlidingWindowRateLimit_BasicTier(t *testing.T) {
	t.Parallel()

	rdb := mockRedisClient()
	if rdb == nil {
		t.Skip("Valkey not available, skipping integration test")
	}

	// Clean up test keys.
	ctx := t.Context()
	keys, _ := rdb.Keys(ctx, "jc:api_rate:test_basic_*").Result()

	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	handler := SlidingWindowRateLimit(rdb)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Simulate a basic-tier API key.
	keyCtx := &APIKeyContext{
		KeyPrefix:     "test_basic_" + time.Now().Format("150405"),
		RateLimitTier: "basic",
	}

	for i := range 100 {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 101st request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
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
	t.Parallel()

	rdb := mockRedisClient()
	if rdb == nil {
		t.Skip("Valkey not available, skipping integration test")
	}

	ctx := t.Context()
	prefix := "test_headers_" + time.Now().Format("150405")

	keys, _ := rdb.Keys(ctx, valkeypkg.MustKey("api_rate:%s:*", prefix)).Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	handler := SlidingWindowRateLimit(rdb)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	keyCtx := &APIKeyContext{
		KeyPrefix:     prefix,
		RateLimitTier: "basic",
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
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

func TestSlidingWindowRateLimit_UserTierCapsKey(t *testing.T) {
	t.Parallel()

	rdb := mockRedisClient()
	if rdb == nil {
		t.Skip("Valkey not available, skipping integration test")
	}

	// Clean up test keys.
	ctx := t.Context()
	prefix := "test_unlimited_" + time.Now().Format("150405")

	keys, _ := rdb.Keys(ctx, valkeypkg.MustKey("api_rate:%s:*", prefix)).Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	// A regular user with an "unlimited" key must be capped at the basic
	// tier because "unlimited" is not a valid user tier.
	handler := SlidingWindowRateLimitWithSettings(rdb, nil, &fakeTierResolver{tier: "regular"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	keyCtx := &APIKeyContext{
		KeyPrefix:     prefix,
		RateLimitTier: "unlimited",
	}

	// First 100 requests should pass (basic cap).
	for i := range 100 {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 101st request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 101: expected 429 for capped tier, got %d", w.Code)
	}
}

func TestSlidingWindowRateLimit_NilKeyPassesThrough(t *testing.T) {
	t.Parallel()

	rdb := mockRedisClient()
	if rdb == nil {
		t.Skip("Valkey not available, skipping integration test")
	}

	handler := SlidingWindowRateLimit(rdb)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No API key in context — should pass through.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for nil key context, got %d", w.Code)
	}
}

func TestSlidingWindowRateLimit_NilRedisDegradesGracefully(t *testing.T) {
	t.Parallel()

	handler := SlidingWindowRateLimit(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	keyCtx := &APIKeyContext{
		KeyPrefix:     "test_nil_redis",
		RateLimitTier: "basic",
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), apiKeyContextKey, keyCtx))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when Redis is nil, got %d", w.Code)
	}
}

func TestGetTierLimit(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			got := getTierLimit(tt.tier)
			if got != tt.expected {
				t.Errorf("getTierLimit(%q) = %d, want %d", tt.tier, got, tt.expected)
			}
		})
	}
}
