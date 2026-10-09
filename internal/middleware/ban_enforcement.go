package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/valkey"
)

// banExemptPrefixes are the endpoints a banned/suspended user can still
// reach so the §22 appeal loop stays usable — submit an appeal, read it
// back, and converse on the linked §38 staff ticket. Everything else
// stays blocked.
//
// Ref: japaconnect-engineering/sections/22-admin-panel.md (Appeals)
var banExemptPrefixes = []string{
	"/api/v1/appeals",
	"/api/v1/me/appeals",
	"/api/v1/me/tickets",
}

// banExemptExact are exact-path exemptions for the session bootstrap a
// restricted session needs: /me (the frontend resolves identity + status
// from it — without it the shell clears jc_token on the 403), /csrf
// (needed to mint a token for POST /appeals), and auth refresh/logout so
// the appeal channel survives access-token expiry. These are EXACT matches
// — other /me/* subresources (content, notifications…) stay blocked.
var banExemptExact = map[string]bool{
	"/api/v1/me":           true,
	"/api/v1/csrf":         true,
	"/api/v1/auth/refresh": true,
	"/api/v1/auth/logout":  true,
}

func isBanExemptPath(path string) bool {
	if banExemptExact[path] {
		return true
	}
	for _, p := range banExemptPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// BanEnforcement returns middleware that checks if the authenticated user
// is banned. It checks the Valkey cache first (jc:ban:{user_id}) and
// falls back to the isBanned callback for a database lookup.
//
// The isBanned function should query the user_bans table and return true
// when an active (non-expired) ban exists for the given user ID.
//
// When a ban is found, the middleware writes a 403 JSON response and
// stops the request chain — except on the appeal channel above, which a
// banned user must be able to reach to contest the ban. When Valkey is
// unreachable or the user is not banned, the request passes through
// normally.
func BanEnforcement(rdb valkeycompat.Cmdable, isBanned func(ctx context.Context, userID string) (bool, *time.Time)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid || isBanExemptPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			userID := user.UserID.String()
			cacheKey := valkey.MustKey("ban:%s", userID)

			// Check Valkey cache for ban status. The cache is invalidated
			// (Del) whenever a ban is created, so a cached "not banned"
			// state is safe to honour without a DB round-trip.
		cacheCheck:
			for {
				if rdb != nil {
					cached, err := rdb.Get(r.Context(), cacheKey).Bytes()
					if err == nil && len(cached) > 0 {
						var permData struct {
							Banned    bool       `json:"banned"`
							ExpiresAt *time.Time `json:"banned_expires_at,omitempty"`
						}
						if json.Unmarshal(cached, &permData) == nil {
							if permData.Banned {
								// Check if ban has expired.
								if permData.ExpiresAt != nil && permData.ExpiresAt.Before(time.Now()) {
									rdb.Del(r.Context(), cacheKey)
									// Fall through to DB check — the cached ban is
									// stale and the user may have been re-banned.
									break cacheCheck
								}
								writeBanResponse(w, permData.ExpiresAt)
								return
							}
							// Cache says not banned — skip the DB hit.
							next.ServeHTTP(w, r)
							return
						}
					}
				}
				break cacheCheck
			}

			// Fallback to DB check.
			banned, expiresAt := isBanned(r.Context(), userID)
			if banned {
				// Update cache so subsequent requests hit Valkey.
				if rdb != nil {
					data, _ := json.Marshal(map[string]any{
						"banned":            true,
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

	_ = json.NewEncoder(w).Encode(resp)
}
