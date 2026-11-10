package nextactions

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
)

// GuestCookieName is the anonymous-visitor identity cookie — the guest
// SessionKey source for §122 state and event feeds. Cookie keying beats the
// client-IP fallback: it survives IP rotation and splits CGNAT collisions.
const GuestCookieName = "jc_guest"

// ConsentCookieName is the cookie-consent decision cookie written by the
// frontend banner on the shared base domain (visible to api.<domain>).
// The value is "all", "necessary", or a comma-list of granted consent_type
// tokens (see ConsentGrants) — absent/unknown values mean "strictly
// necessary only" (strict opt-in — §19).
const ConsentCookieName = "jc_consent"
const ConsentValueAll = "all"

const guestCookieMaxAge = 365 * 24 * time.Hour // 1 year

// ConsentGrants reports whether a jc_consent cookie value grants a
// consent_type category. "all" grants every category; "necessary" and
// absent/unknown values grant nothing; otherwise the cookie is a
// comma-list of granted consent_type tokens (the per-category decisions
// written by the banner's Customize view or a settings toggle).
func ConsentGrants(value, category string) bool {
	if value == ConsentValueAll {
		return true
	}
	for _, part := range strings.Split(value, ",") {
		if strings.TrimSpace(part) == category {
			return true
		}
	}
	return false
}

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
// tool.used events or next-actions state write. OptionalAuth must run
// before this middleware — authenticated requests never mint (session
// identity governs), but a stale jc_guest is still expired for them when
// consent is absent so a revoked decision can't outlive the login.
//
// §19 opt-in: jc_guest is a persistent non-essential identifier, so it is
// only minted after the visitor affirmatively consents (jc_consent=all).
// Without consent the request falls back to client-IP keying. An existing
// jc_guest presented under a missing/rejected consent decision is expired —
// e.g. a cookie minted before the banner, or a decision changed later.
// secure comes from cfg.IsProduction so the flag follows deployment config
// rather than hop-by-hop X-Forwarded-Proto presence.
func GuestSessionMiddleware(secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())

			hasConsent := false
			if c, err := r.Cookie(ConsentCookieName); err == nil {
				hasConsent = ConsentGrants(c.Value, "functional_cookies")
			}

			guest, guestErr := r.Cookie(GuestCookieName)
			hasGuest := guestErr == nil && guest.Value != ""
			switch {
			case user == nil && hasConsent && !hasGuest:
				id := make([]byte, 16)
				if _, err := rand.Read(id); err == nil {
					http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure follows cfg.IsProduction; dev serves over plain http
						Name:     GuestCookieName,
						Value:    hex.EncodeToString(id),
						Path:     "/",
						MaxAge:   int(guestCookieMaxAge.Seconds()),
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
						Secure:   secure,
					})
				}
			case !hasConsent && hasGuest:
				http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure follows cfg.IsProduction; dev serves over plain http
					Name:     GuestCookieName,
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Secure:   secure,
				})
			}
			next.ServeHTTP(w, r)
		})
	}
}
