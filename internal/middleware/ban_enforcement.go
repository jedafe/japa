package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/auth"
)

// BanEnforcement returns middleware that checks if the authenticated user
// is banned. It checks the Valkey cache first (jc:perm:{user_id} with a
// ban flag) and falls back to the isBanned callback for a database lookup.
//
// The isBanned function should query the user_bans table and return true
// when an active (non-expired) ban exists for the given user ID.
//
// When a ban is found, the middleware writes a 403 JSON response and
// stops the request chain. When Valkey is unreachable or the user is
// not banned, the request passes through normally.
func BanEnforcement(rdb valkeycompat.Cmdable, isBanned func(ctx context.Context, userID string) (bool, *time.Time)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
				next.ServeHTTP(w, r)
				return
			}

			userID := user.UserID.String()
			cacheKey := "jc:perm:" + userID

			// Check Valkey cache for ban status.
			if rdb != nil {
				cached, err := rdb.Get(r.Context(), cacheKey).Bytes()
				if err == nil && len(cached) > 0 {
					var permData struct {
						Banned    bool       `json:"banned"`
						ExpiresAt *time.Time `json:"banned_expires_at,omitempty"`
					}
					if json.Unmarshal(cached, &permData) == nil && permData.Banned {
						// Check if ban has expired.
						if permData.ExpiresAt != nil && permData.ExpiresAt.Before(time.Now()) {
							rdb.Del(r.Context(), cacheKey)
							// Cache the "not banned" state briefly to avoid DB hit on next request.
							rdb.Set(r.Context(), cacheKey, []byte(`{"banned":false}`), 5*time.Minute)
							next.ServeHTTP(w, r)
							return
						}
						writeBanResponse(w, permData.ExpiresAt)
						return
					}
				}
			}

			// Fallback to DB check.
			banned, expiresAt := isBanned(r.Context(), userID)
			if banned {
				// Update cache so subsequent requests hit Valkey.
				if rdb != nil {
					data, _ := json.Marshal(map[string]any{
						"banned":             true,
						"banned_expires_at": expiresAt,
					})
					rdb.Set(r.Context(), cacheKey, data, 5*time.Minute)
				}
				writeBanResponse(w, expiresAt)
				return
			}

			// Cache non-banned state to avoid repeated DB hits.
			if rdb != nil {
				rdb.Set(r.Context(), cacheKey, []byte(`{"banned":false}`), 5*time.Minute)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeBanResponse writes a JSON 403 error indicating the user's
// account has been banned, with an optional expiration timestamp.
func writeBanResponse(w http.ResponseWriter, expiresAt *time.Time) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)

	resp := map[string]any{
		"error": map[string]string{
			"code":    "account_banned",
			"message": "Your account has been banned.",
		},
	}
	if expiresAt != nil {
		resp["expires_at"] = expiresAt.Format(time.RFC3339)
	}

	json.NewEncoder(w).Encode(resp)
}
