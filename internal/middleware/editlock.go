// Package middleware provides cross-cutting HTTP middleware for JapaConnect.
//
// The edit lock middleware prevents concurrent edits on the same content
// resource by acquiring a short-lived Valkey key per content item. The
// lock key is "jc:editor_lock:{content_type}:{content_id}" with a 120-second
// TTL that auto-expires if the editor's session stalls. When Valkey is
// unreachable the middleware degrades gracefully — the same philosophy
// the rate limiter and idempotency middleware use.
package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jedafe/japa/internal/auth"
)

// editLockTTL controls how long an edit lock is held before auto-releasing.
// 120 seconds covers the typical editing window without requiring an explicit
// unlock on every save — the key auto-expires if the editor walks away.
const editLockTTL = 120 * time.Second

// EditLockMiddleware returns middleware that manages collaborative edit locks.
//
// On POST, PUT, and PATCH the middleware acquires a per-content lock in
// Valkey. If another user already holds the lock the request is rejected
// with 409 Conflict; if the same user holds it the TTL is refreshed.
// On DELETE the lock is released.
//
// The lock value is the authenticated user's UUID. Requests without an
// authenticated user pass through without acquiring a lock.
//
// The content type and ID are extracted from the URL path using a
// configurable extractor. When the extractor returns empty strings the
// middleware passes through — this prevents lock collisions on endpoints
// that don't map cleanly to a content resource.
//
// When Valkey is unreachable the middleware degrades gracefully and allows
// the request through, consistent with the rate limiter and idempotency
// middleware.
func EditLockMiddleware(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only state-changing methods need edit locks.
			if r.Method != http.MethodPost && r.Method != http.MethodPut &&
				r.Method != http.MethodPatch && r.Method != http.MethodDelete {
				next.ServeHTTP(w, r)
				return
			}

			// A nil Valkey client means the service is unavailable —
			// pass through immediately rather than failing every request.
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			contentType, contentID := extractContentFromPath(r.URL.Path)
			if contentType == "" || contentID == "" {
				next.ServeHTTP(w, r)
				return
			}

			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
				next.ServeHTTP(w, r)
				return
			}

			userID := user.UserID.String()
			key := "jc:editor_lock:" + contentType + ":" + contentID
			ctx := r.Context()

			if r.Method == http.MethodDelete {
				handleEditLockDelete(ctx, w, r, rdb, key, userID, next)
				return
			}

			handleEditLockAcquire(ctx, w, r, rdb, key, userID, next)
		})
	}
}

// handleEditLockDelete releases the edit lock on DELETE, but only if the
// current user owns the lock. A user who doesn't own the lock is not
// allowed to release it — that would let anyone break another user's
// edit protection.
func handleEditLockDelete(ctx context.Context, w http.ResponseWriter, r *http.Request, rdb *redis.Client, key, userID string, next http.Handler) {
	existing, err := rdb.Get(ctx, key).Result()
	if err == nil && existing != userID {
		// Lock belongs to another user — do not release it.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "lock_not_owned",
				"message": "Cannot release edit lock held by another user.",
			},
		})
		return
	}

	if err := rdb.Del(ctx, key).Err(); err != nil {
		slog.Warn("editlock: failed to release lock",
			"key", key, "err", err)
	}

	next.ServeHTTP(w, r)
}

// handleEditLockAcquire checks and acquires (or refreshes) an edit lock for
// POST, PUT, and PATCH requests. If another user already holds the lock a
// 409 is returned without forwarding to the handler.
func handleEditLockAcquire(ctx context.Context, w http.ResponseWriter, r *http.Request, rdb *redis.Client, key, userID string, next http.Handler) {
	existing, err := rdb.Get(ctx, key).Result()
	if err == nil && existing != userID {
		// Lock held by a different user — reject with 409 Conflict.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "edit_conflict",
				"message": "This content is currently being edited by another user.",
			},
		})
		return
	}

	// Either no lock exists or the same user holds it — set/refresh.
	if err := rdb.Set(ctx, key, userID, editLockTTL).Err(); err != nil {
		slog.Warn("editlock: failed to acquire lock, allowing request",
			"key", key, "err", err)
		// Degraded — let the request through.
	}

	next.ServeHTTP(w, r)
}

// extractContentFromPath derives a content type and ID from a URL path.
// It uses the last two non-empty path segments: the penultimate segment
// becomes the content type and the final segment becomes the ID. This
// handles the standard REST pattern /api/v1/{type}/{id} as well as
// nested routes like /api/v1/posts/{id}/comments/{cid} where the innermost
// resource is what matters for edit locking.
func extractContentFromPath(path string) (contentType, contentID string) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", ""
	}

	segments := strings.Split(trimmed, "/")
	// Need at least 4 segments to identify both a content type and an ID
	// while skipping version prefixes like /api/v1/{type}/{id}.
	if len(segments) < 4 {
		return "", ""
	}

	return segments[len(segments)-2], segments[len(segments)-1]
}
