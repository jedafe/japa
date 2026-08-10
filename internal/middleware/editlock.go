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
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/valkey"
)

// editLockTTLDefault is the fallback TTL when the system setting is unavailable.
// MUST match the valkey-keys-catalog.md entry (90s) for audit conformance (AUDIT-BE-028).
const editLockTTLDefault = 90 * time.Second

// editLockSettingsReader is the subset of config.Settings the edit lock needs.
type editLockSettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

var defaultTenantUUID = pgtype.UUID{Bytes: [16]byte{}, Valid: true}

// deleteOwnershipScript atomically checks ownership and deletes the lock key.
// Returns 1 if the key was deleted (ownership verified), 0 otherwise.
var deleteOwnershipScript = valkeycompat.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end
`)

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
func EditLockMiddleware(rdb valkeycompat.Cmdable, settings editLockSettingsReader) func(http.Handler) http.Handler {
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
			key := valkey.MustKey("editor_lock:%s:%s", contentType, contentID)
			ctx := r.Context()

			// Read TTL from system settings with fallback to the default.
			ttl := editLockTTLDefault
			if settings != nil {
				ttl = time.Duration(settings.GetIntOr(ctx, defaultTenantUUID, "content.edit_lock_ttl_seconds", 120)) * time.Second
			}

			if r.Method == http.MethodDelete {
				handleEditLockDelete(ctx, w, r, rdb, key, userID, next)
				return
			}

			handleEditLockAcquire(ctx, w, r, rdb, key, userID, ttl, next)
		})
	}
}

// handleEditLockDelete releases the edit lock on DELETE, but only if the
// current user owns the lock. A user who doesn't own the lock is not
// allowed to release it — that would let anyone break another user's
// edit protection.
//
// Uses a Lua script for atomic check-and-delete to prevent TOCTOU races:
// between the GET and DEL in the old implementation, another process could
// release and re-acquire the lock, causing the wrong user's lock to be
// deleted.
func handleEditLockDelete(ctx context.Context, w http.ResponseWriter, r *http.Request, rdb valkeycompat.Cmdable, key, userID string, next http.Handler) {
	result, err := deleteOwnershipScript.Run(ctx, rdb, []string{key}, userID).Int64()
	if err != nil && !errors.Is(err, valkeycompat.Nil) {
		slog.Warn("editlock: delete ownership script failed",
			"key", key, "err", err)
		// Degrade — let the request through.
		next.ServeHTTP(w, r)

		return
	}

	if result == 1 {
		// Lock deleted successfully — ownership was verified.
		next.ServeHTTP(w, r)
		return
	}

	// result == 0: either key doesn't exist, or ownership mismatch.
	// Check which case it is.
	existing, getErr := rdb.Get(ctx, key).Result()
	if getErr != nil {
		// Key doesn't exist — nothing to delete, pass through.
		next.ServeHTTP(w, r)
		return
	}

	if existing != userID {
		// Lock belongs to another user — do not release it.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "lock_not_owned",
				"message": "Cannot release edit lock held by another user.",
			},
		})

		return
	}

	// Shouldn't reach here normally, but pass through to be safe.
	next.ServeHTTP(w, r)
}

// handleEditLockAcquire checks and acquires (or refreshes) an edit lock for
// POST, PUT, and PATCH requests. If another user already holds the lock a
// 409 is returned without forwarding to the handler.
//
// Uses SET NX EX for atomic lock acquisition — a single Valkey command that
// sets the key only if it does not already exist, with a TTL. This eliminates
// the TOCTOU race condition in the previous GET+SET implementation where two
// concurrent requests could both see no lock and both acquire it.
func handleEditLockAcquire(ctx context.Context, w http.ResponseWriter, r *http.Request, rdb valkeycompat.Cmdable, key, userID string, ttl time.Duration, next http.Handler) {
	// Atomic acquire: SET key userID NX EX ttl_seconds
	// NX = only set if key does not exist (acquire new lock)
	// EX = set expiry in seconds (auto-release if editor stalls)
	set, err := rdb.SetNX(ctx, key, userID, ttl).Result()
	if err != nil {
		slog.Warn("editlock: failed to acquire lock (SET NX), allowing request",
			"key", key, "err", err)
		// Degraded — let the request through.
		next.ServeHTTP(w, r)

		return
	}

	if set {
		// Lock acquired successfully.
		next.ServeHTTP(w, r)
		return
	}

	// Lock already exists. Check if we own it (same user editing again).
	existing, err := rdb.Get(ctx, key).Result()
	if err != nil {
		// Key disappeared between SET NX and GET — race with expiry.
		// Try to acquire again.
		set2, err2 := rdb.SetNX(ctx, key, userID, ttl).Result()
		if err2 != nil || !set2 {
			slog.Warn("editlock: lost lock during re-acquire, allowing request",
				"key", key, "err", err2)
			next.ServeHTTP(w, r)

			return
		}

		next.ServeHTTP(w, r)

		return
	}

	if existing == userID {
		// Same user — refresh the TTL.
		if err := rdb.Set(ctx, key, userID, ttl).Err(); err != nil {
			slog.Warn("editlock: failed to refresh lock TTL",
				"key", key, "err", err)
		}

		next.ServeHTTP(w, r)

		return
	}

	// Different user holds the lock — reject with 409 Conflict.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    "edit_conflict",
			"message": "This content is currently being edited by another user.",
		},
	})
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
