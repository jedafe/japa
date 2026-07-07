// Package middleware provides cross-cutting HTTP middleware for JapaConnect.
//
// The idempotency middleware intercepts state-changing requests (POST, PUT,
// PATCH) that carry an Idempotency-Key header and guarantees that
// re-executing the same request returns the same response. The key hash
// binds method + path + idempotency key + authenticated user ID so that
// identical requests from different callers are treated independently.
//
// Cached responses are stored in Valkey (Redis-protocol) under the
// "jc:idempotency:" prefix with a 24-hour TTL. When Valkey is unreachable
// the middleware degrades gracefully and passes the request through — the
// same philosophy the rate limiter uses: a missing cache must never crash
// the API.
package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/auth"
)

// idempotencyTTL controls how long a cached response is kept in Valkey.
// 24 hours covers the typical browser refresh window and overnight
// automated retries without hoarding stale entries.
const idempotencyTTL = 24 * time.Hour

// cachedResponse is the JSON-serialisable payload stored in Valkey for
// replay. Only the status code and body bytes are cached — response
// headers are not replayed to avoid leaking middleware-set headers like
// rate-limit counters or cache-control directives from a previous request.
type cachedResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// responseCapture wraps http.ResponseWriter so the downstream handler's
// output is written to both the real writer and an in-memory buffer.
// The buffer is what gets persisted to Valkey; the real writer is what
// the client actually receives.
type responseCapture struct {
	http.ResponseWriter
	body   bytes.Buffer
	status int
}

// WriteHeader intercepts the status code so it can be stored alongside
// the body.
func (rc *responseCapture) WriteHeader(code int) {
	rc.status = code
	rc.ResponseWriter.WriteHeader(code)
}

// Write mirrors the body bytes to the in-memory buffer while forwarding
// them to the client.
func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b)
	return rc.ResponseWriter.Write(b)
}

// IdempotencyMiddleware returns middleware that caches and replays
// responses for requests carrying an Idempotency-Key header. Requests
// without the header pass through untouched. POST, PUT, and PATCH are
// the only methods considered — idempotency for GET is redundant since
// GET is already safe by definition.
//
// The hash input is SHA-256(method + ":" + path + ":" + key + ":" +
// callerID). callerID is the authenticated user's UUID when present,
// otherwise the client IP is used so anonymous callers don't collide
// with each other. This means the same idempotency key issued by two
// different users produces two distinct cache entries.
//
// When Valkey is unreachable or the key lookup/SET fails, the request
// is forwarded to the handler without caching — the API stays available
// even when the idempotency layer is down.
func IdempotencyMiddleware(rdb valkeycompat.Cmdable) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only state-changing methods are idempotent-safe.
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// A nil Valkey client means the service is completely
			// unavailable — pass through immediately rather than
			// failing the request on every call.
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			callerID := idempotencyCallerID(r)
			hash := idempotencyHash(r.Method, r.URL.Path, key, callerID)
			cacheKey := "jc:idempotency:" + hash

			ctx := r.Context()

			// Attempt to retrieve a cached response.
			cached, err := rdb.Get(ctx, cacheKey).Bytes()
			if err == nil && len(cached) > 0 {
				var resp cachedResponse
				if json.Unmarshal(cached, &resp) == nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(resp.Status)
					_, _ = w.Write([]byte(resp.Body))
					return
				}
				// Corrupted cache entry — treat as a miss.
				slog.Warn("idempotency: corrupted cache entry, recomputing",
					"key", cacheKey)
			}

			// Acquire a processing lock to prevent concurrent execution.
			lockKey := cacheKey + ":lock"
			acquired, err := rdb.SetNX(ctx, lockKey, "1", 30*time.Second).Result()
			if err != nil {
				slog.Warn("idempotency: failed to acquire lock", "err", err)
				next.ServeHTTP(w, r)
				return
			}
			if !acquired {
				// Another request is processing. Wait briefly then check cache.
				time.Sleep(100 * time.Millisecond)
				cached, err := rdb.Get(ctx, cacheKey).Bytes()
				if err == nil && len(cached) > 0 {
					var resp cachedResponse
					if json.Unmarshal(cached, &resp) == nil {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(resp.Status)
						_, _ = w.Write([]byte(resp.Body))
						return
					}
				}
				next.ServeHTTP(w, r)
				return
			}
			defer rdb.Del(ctx, lockKey)

			// Wrap the writer so we can capture the handler's output.
			capture := &responseCapture{ResponseWriter: w}
			next.ServeHTTP(capture, r)

			// Only cache successful writes (2xx).
			if capture.status < 200 || capture.status >= 300 {
				return
			}

			cachedBody := cachedResponse{
				Status: capture.status,
				Body:   capture.body.String(),
			}

			payload, err := json.Marshal(cachedBody)
			if err != nil {
				slog.Error("idempotency: failed to marshal cached response",
					"err", err)
				return
			}

			if err := rdb.Set(ctx, cacheKey, payload, idempotencyTTL).Err(); err != nil {
				slog.Warn("idempotency: failed to cache response",
					"key", cacheKey, "err", err)
			}
		})
	}
}

// idempotencyCallerID extracts a stable identifier for the current caller.
// Authenticated users are identified by their UUID; anonymous callers
// fall back to RemoteAddr so concurrent anonymous requests with the same
// idempotency key are not falsely merged.
func idempotencyCallerID(r *http.Request) string {
	if user := auth.FromContext(r.Context()); user != nil && user.UserID.Valid {
		return user.UserID.String()
	}

	return r.RemoteAddr
}

// idempotencyHash produces a deterministic hex-encoded SHA-256 digest from
// the four inputs that uniquely identify an idempotent request.
func idempotencyHash(method, path, key, callerID string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s:%s:%s:%s", method, path, key, callerID)

	return hex.EncodeToString(h.Sum(nil))
}
