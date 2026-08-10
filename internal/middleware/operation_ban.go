package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/valkey"
)

// OperationType represents a restrictable user operation.
type OperationType string

const (
	OpPost       OperationType = "post"
	OpComment    OperationType = "comment"
	OpDiscussion OperationType = "discussion"
	OpBlog       OperationType = "blog"
	OpMessage    OperationType = "message"
)

// AllOperations returns every restrictable operation.
func AllOperations() []OperationType {
	return []OperationType{OpPost, OpComment, OpDiscussion, OpBlog, OpMessage}
}

type opBanContextKey struct{}

// WithOperation returns middleware that tags the request with an operation type.
// Apply on specific routes: r.With(WithOperation(OpPost)).Post("/content", ...)
func WithOperation(op OperationType) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), opBanContextKey{}, op)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetOperation extracts the operation type from context, or empty string if none.
func GetOperation(ctx context.Context) OperationType {
	op, _ := ctx.Value(opBanContextKey{}).(OperationType)
	return op
}

// OperationBanEnforcement returns middleware that checks if the user is banned
// from the operation declared in context (set by WithOperation).
// If no operation is set, the request passes through.
func OperationBanEnforcement(rdb valkeycompat.Cmdable,
	isRestricted func(ctx context.Context, userID string, op string) (bool, *time.Time),
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			op := GetOperation(r.Context())
			if op == "" {
				next.ServeHTTP(w, r)
				return
			}

			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
				next.ServeHTTP(w, r)
				return
			}

			userID := user.UserID.String()
			cacheKey := valkey.MustKey("restrictions:%s", userID)

			// Check Valkey cache for operation-specific restrictions.
			if rdb != nil {
				cached, err := rdb.Get(r.Context(), cacheKey).Bytes()
				if err == nil && len(cached) > 0 {
					var restrictions struct {
						BannedOps map[string]*string `json:"banned_ops"` // op -> expires_at (nil=permanent)
					}
					if json.Unmarshal(cached, &restrictions) == nil {
						if expiryStr, ok := restrictions.BannedOps[string(op)]; ok {
							// Check if ban has expired.
							if expiryStr != nil {
								if expiry, err := time.Parse(time.RFC3339, *expiryStr); err == nil && expiry.Before(time.Now()) {
									// Expired — let through (cache will refresh on next request).
									next.ServeHTTP(w, r)
									return
								}
							}
							var expiresAt *time.Time
							if expiryStr != nil {
								if t, err := time.Parse(time.RFC3339, *expiryStr); err == nil {
									expiresAt = &t
								}
							}
							writeOperationBanResponse(w, string(op), expiresAt)
							return
						}
					}
				}
			}

			// Fallback to DB check.
			restricted, expiresAt := isRestricted(r.Context(), userID, string(op))
			if restricted {
				writeOperationBanResponse(w, string(op), expiresAt)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeOperationBanResponse writes a 403 for operation-specific bans.
func writeOperationBanResponse(w http.ResponseWriter, operation string, expiresAt *time.Time) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)

	messages := map[string]string{
		"post":       "You are temporarily restricted from creating posts.",
		"comment":    "You are temporarily restricted from commenting.",
		"discussion": "You are temporarily restricted from creating discussions.",
		"blog":       "You are temporarily restricted from publishing blog posts.",
		"message":    "You are temporarily restricted from sending messages.",
	}
	msg, ok := messages[operation]
	if !ok {
		msg = "You are temporarily restricted from this action."
	}

	resp := map[string]any{
		"error": map[string]any{
			"code":      "operation_restricted",
			"message":   msg,
			"operation": operation,
		},
	}
	if expiresAt != nil {
		resp["error"].(map[string]any)["expires_at"] = expiresAt.Format(time.RFC3339)
	}

	json.NewEncoder(w).Encode(resp)
}
