package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	valkeypkg "github.com/jedafe/japa/internal/valkey"
)

// startTestRedis spins up an in-memory Valkey and returns a connected
// client. Fatal on failure so the caller doesn't have to handle errors.
func startTestRedis(t *testing.T) (*miniredis.Miniredis, valkeycompat.Cmdable) {
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
		t.Fatalf("valkey connect: %v", err)
	}

	compat := valkeycompat.NewAdapter(vk)

	t.Cleanup(func() { vk.Close() })

	return mr, compat
}

// mrKeys returns the keys matching pattern from the miniredis instance
// directly, bypassing valkeycompat which sends unsupported commands.
func mrKeys(mr *miniredis.Miniredis, pattern string) []string {
	all := mr.Keys()

	var matched []string

	for _, k := range all {
		if ok, _ := path.Match(pattern, k); ok {
			matched = append(matched, k)
		}
	}

	return matched
}

// handler returns a simple test handler that echoes back a 200 with a
// JSON body containing the request's method and path.
func handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"method": r.Method,
			"path":   r.URL.Path,
		})
	})
}

// errorHandler always returns a 500 to test non-2xx behaviour.
func errorHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	})
}

// doRequest is a thin helper to reduce boilerplate in each test case.
func doRequest(t *testing.T, mw func(http.Handler) http.Handler, h http.Handler, method, path, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, req)

	return rr
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestIdempotency_PassesThroughOnGET(t *testing.T) {
	t.Parallel()
	_, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	rr := doRequest(t, mw, handler(), http.MethodGet, "/api/v1/posts", "my-key")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestIdempotency_PassesThroughOnDELETE(t *testing.T) {
	t.Parallel()
	_, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	rr := doRequest(t, mw, handler(), http.MethodDelete, "/api/v1/posts/123", "my-key")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestIdempotency_PassesThroughWithoutKey(t *testing.T) {
	t.Parallel()
	mr, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	rr := doRequest(t, mw, handler(), http.MethodPost, "/api/v1/posts", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Nothing should have been stored in Valkey.
	keys := mrKeys(mr, "jc:idempotency:*")

	if len(keys) > 0 {
		t.Fatalf("expected no cached keys, got %v", keys)
	}
}

func TestIdempotency_NilRedisClientDegradesGracefully(t *testing.T) {
	t.Parallel()

	mw := IdempotencyMiddleware(nil, nil)

	rr := doRequest(t, mw, handler(), http.MethodPost, "/api/v1/posts", "my-key")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestIdempotency_CachesSuccessfulResponse(t *testing.T) {
	t.Parallel()
	mr, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	// First request — should hit the handler and cache the result.
	rr1 := doRequest(t, mw, handler(), http.MethodPost, "/api/v1/posts", "abc-123")
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr1.Code)
	}

	body1 := rr1.Body.Bytes()

	// Verify something was stored in Valkey.
	keys := mrKeys(mr, "jc:idempotency:*")

	if len(keys) != 1 {
		t.Fatalf("expected 1 cached key, got %d: %v", len(keys), keys)
	}

	// Second request — should return the cached response without
	// invoking the handler again.
	rr2 := doRequest(t, mw, handler(), http.MethodPost, "/api/v1/posts", "abc-123")
	if rr2.Code != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d", rr2.Code)
	}

	if !bytes.Equal(rr1.Body.Bytes(), rr2.Body.Bytes()) {
		t.Fatalf("response mismatch:\n  first:  %s\n  second: %s", body1, rr2.Body.String())
	}
}

func TestIdempotency_DifferentKeysProduceDifferentEntries(t *testing.T) {
	t.Parallel()
	mr, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	doRequest(t, mw, handler(), http.MethodPost, "/api/v1/posts", "key-1")
	doRequest(t, mw, handler(), http.MethodPost, "/api/v1/posts", "key-2")

	keys := mrKeys(mr, "jc:idempotency:*")

	if len(keys) != 2 {
		t.Fatalf("expected 2 cached keys, got %d: %v", len(keys), keys)
	}
}

func TestIdempotency_DifferentCallersWithSameKeyAreIndependent(t *testing.T) {
	t.Parallel()
	mr, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	// Simulate two anonymous callers by varying RemoteAddr.
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	req1.Header.Set("Idempotency-Key", "shared-key")
	req1.RemoteAddr = "10.0.0.1:54321"
	rr1 := httptest.NewRecorder()
	mw(handler()).ServeHTTP(rr1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	req2.Header.Set("Idempotency-Key", "shared-key")
	req2.RemoteAddr = "10.0.0.2:54321"
	rr2 := httptest.NewRecorder()
	mw(handler()).ServeHTTP(rr2, req2)

	keys := mrKeys(mr, "jc:idempotency:*")

	if len(keys) != 2 {
		t.Fatalf("expected 2 cached keys for different callers, got %d", len(keys))
	}
}

func TestIdempotency_DoesNotCacheErrorResponses(t *testing.T) {
	t.Parallel()
	mr, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	doRequest(t, mw, errorHandler(), http.MethodPost, "/api/v1/posts", "error-key")

	keys := mrKeys(mr, "jc:idempotency:*")

	if len(keys) != 0 {
		t.Fatalf("expected 0 cached keys for error response, got %d: %v", len(keys), keys)
	}
}

func TestIdempotency_CorruptedCacheEntryRecomputes(t *testing.T) {
	t.Parallel()
	_, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	// Store a corrupted (non-JSON) value at a known key.
	hash := idempotencyHash(http.MethodPost, "/api/v1/posts", "corrupt", "anon")
	cacheKey := valkeypkg.MustKey("idempotency:%s", hash)
	rdb.Set(t.Context(), cacheKey, "NOT-JSON", time.Duration(idempotencyTTLDefault)*time.Hour)

	// The request should not panic and should fall through to the
	// handler, returning a fresh response.
	rr := doRequest(t, mw, handler(), http.MethodPost, "/api/v1/posts", "corrupt")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var parsed map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if parsed["method"] != http.MethodPost {
		t.Fatalf("expected method POST, got %s", parsed["method"])
	}
}

func TestIdempotency_HashIsDeterministic(t *testing.T) {
	t.Parallel()

	h1 := idempotencyHash("POST", "/api/v1/posts", "key", "user-1")
	h2 := idempotencyHash("POST", "/api/v1/posts", "key", "user-1")

	if h1 != h2 {
		t.Fatalf("hash not deterministic: %s != %s", h1, h2)
	}

	// Different inputs must produce different hashes.
	h3 := idempotencyHash("POST", "/api/v1/posts", "key", "user-2")
	if h1 == h3 {
		t.Fatalf("different callerIDs must produce different hashes")
	}
}

func TestIdempotency_PreservesStatusCode(t *testing.T) {
	t.Parallel()
	_, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	custom := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})
	})

	rr1 := doRequest(t, mw, custom, http.MethodPost, "/api/v1/posts", "status-test")
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d", rr1.Code)
	}

	rr2 := doRequest(t, mw, custom, http.MethodPost, "/api/v1/posts", "status-test")
	if rr2.Code != http.StatusCreated {
		t.Fatalf("second request: expected 201, got %d", rr2.Code)
	}

	if !bytes.Equal(rr1.Body.Bytes(), rr2.Body.Bytes()) {
		t.Fatalf("cached body mismatch:\n  first:  %s\n  second: %s",
			rr1.Body.String(), rr2.Body.String())
	}
}

func TestIdempotency_PUTandPATCHAreCached(t *testing.T) {
	t.Parallel()
	mr, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	//nolint:paralleltest // parent checks shared Redis state after subtests complete
	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			rr1 := doRequest(t, mw, handler(), method, "/api/v1/posts/1", "method-"+method)
			if rr1.Code != http.StatusOK {
				t.Fatalf("first %s: expected 200, got %d", method, rr1.Code)
			}

			rr2 := doRequest(t, mw, handler(), method, "/api/v1/posts/1", "method-"+method)
			if rr2.Code != http.StatusOK {
				t.Fatalf("second %s: expected 200, got %d", method, rr2.Code)
			}

			if !bytes.Equal(rr1.Body.Bytes(), rr2.Body.Bytes()) {
				t.Fatalf("%s: cached body mismatch", method)
			}
		})
	}

	keys := mrKeys(mr, "jc:idempotency:*")

	if len(keys) != 2 {
		t.Fatalf("expected 2 cached keys, got %d", len(keys))
	}
}

func TestIdempotency_ValkeyErrorDegradesGracefully(t *testing.T) {
	t.Parallel()
	// Create a client pointing at a non-existent address so every
	// command fails.
	vk := valkeypkg.NewWithFallback("127.0.0.1:1")

	mw := IdempotencyMiddleware(vk.Redis(), nil)

	rr := doRequest(t, mw, handler(), http.MethodPost, "/api/v1/posts", "unreachable")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestIdempotency_ReaderClosedAfterCache(t *testing.T) {
	t.Parallel()
	_, rdb := startTestRedis(t)
	mw := IdempotencyMiddleware(rdb, nil)

	// The handler reads the body to verify it isn't empty.
	var receivedBody string

	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts",
		bytes.NewBufferString("hello-idempotency"))
	req.Header.Set("Idempotency-Key", "body-key")

	rr := httptest.NewRecorder()
	mw(echo).ServeHTTP(rr, req)

	if receivedBody != "hello-idempotency" {
		t.Fatalf("handler did not receive request body: got %q", receivedBody)
	}
}
