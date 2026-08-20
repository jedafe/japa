package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/csrf"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// deviceName extracts a human-readable device label from the User-Agent
// header, truncated to 255 chars. Returns nil when the header is absent
// or empty so the refresh_tokens.device_name column stays NULL.
func deviceName(r *http.Request) *string {
	ua := strings.TrimSpace(r.UserAgent())
	if ua == "" {
		return nil
	}
	if len(ua) > 255 {
		ua = ua[:255]
	}
	return &ua
}

// ipPtr returns a pointer to the client IP, or nil when empty. Uses the
// trusted-proxy-aware clientIP resolver so the stored value cannot be
// spoofed via X-Forwarded-For from an untrusted peer.
func ipPtr(r *http.Request) *string {
	ip := clientIP(r)
	if ip == "" {
		return nil
	}
	return &ip
}

// isMobileClient returns true when the request carries a non-empty
// X-Device-Id header, which the Flutter mobile app sends on every request.
func isMobileClient(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("X-Device-Id")) != ""
}

// mobileDeviceID returns the X-Device-Id header value, or an empty string.
func mobileDeviceID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Device-Id"))
}

// sessionDeviceName returns the mobile device ID when present, otherwise
// falls back to the User-Agent string. This lets /me/sessions distinguish
// mobile sessions without adding a new refresh_tokens column.
func sessionDeviceName(r *http.Request) *string {
	if id := mobileDeviceID(r); id != "" {
		return &id
	}
	return deviceName(r)
}

// writeSessionResponse writes a successful session response from the provided
// data map (which must already contain "access_token"). For mobile clients
// (X-Device-Id present) it adds the refresh token to the JSON body and skips
// cookie issuance. For web/SPA clients it issues the HttpOnly refresh cookie
// and, when requested, the CSRF cookie.
func writeSessionResponse(w http.ResponseWriter, r *http.Request, cookieDomain string, secureCookies bool, refreshToken string, refreshTTL time.Duration, data map[string]interface{}, setCSRFCookie bool) {
	if isMobileClient(r) {
		data["refresh_token"] = refreshToken
	} else {
		http.SetCookie(w, &http.Cookie{
			Name:     "jc_refresh",
			Value:    refreshToken,
			Path:     "/api/v1",
			Domain:   cookieDomain,
			MaxAge:   int(refreshTTL.Seconds()),
			HttpOnly: true,
			Secure:   secureCookies,
			SameSite: http.SameSiteStrictMode,
		}) // #nosec G124 -- Secure is set via secureCookies (true in production); gosec can't verify the struct field statically

		if setCSRFCookie {
			csrf.SetCookie(w, cookieDomain, secureCookies)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": data})
}
