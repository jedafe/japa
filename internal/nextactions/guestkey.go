package nextactions

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
)

// GuestCookieName is the anonymous-visitor identity cookie — the guest
// SessionKey source for §122 state and event feeds. Cookie keying beats the
// client-IP fallback: it survives IP rotation and splits CGNAT collisions.
const GuestCookieName = "jc_guest"

const guestCookieMaxAge = 365 * 24 * time.Hour // 1 year

// GuestSessionKey returns the stable guest identity: the jc_guest cookie
// when present, else the client IP (cookie-less clients, crawlers).
// Exported for other event feeds (features.SetUsageTracker) that must key
// guest session_id identically.
func GuestSessionKey(r *http.Request) string {
	if c, err := r.Cookie(GuestCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return clientip.FromRequest(r)
}

// guestSessionKey is the package-internal alias.
func guestSessionKey(r *http.Request) string { return GuestSessionKey(r) }

// GuestSessionMiddleware issues the jc_guest cookie to anonymous requests
// that lack one, so the first /api/v1 touch establishes identity before any
// tool.used events or next-actions state write. Authenticated requests are
// skipped — OptionalAuth must run before this middleware.
func GuestSessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.FromContext(r.Context()) == nil {
			if _, err := r.Cookie(GuestCookieName); err != nil {
				id := make([]byte, 16)
				if _, err := rand.Read(id); err == nil {
					http.SetCookie(w, &http.Cookie{
						Name:     GuestCookieName,
						Value:    hex.EncodeToString(id),
						Path:     "/",
						MaxAge:   int(guestCookieMaxAge.Seconds()),
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
						Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
					})
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
