// Package csrf implements double-submit cookie CSRF protection for the
// JapaConnect API. A cryptographically random token is issued to the browser as
// a non-HttpOnly cookie (csrf_token). The SPA reads it and echoes it back as the
// X-CSRF-Token header on mutating requests. The server rejects any POST/PATCH/
// PUT/DELETE whose header does not match the cookie with HTTP 403.
//
// Ref: apps/control audit (Critical finding C1) — Control Panel mutating routes
// must be CSRF-protected via the double-submit cookie pattern.
package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
)

const (
	// CookieName is the double-submit cookie read by the SPA and echoed back as
	// the X-CSRF-Token header.
	CookieName = "csrf_token"
	// HeaderName is the request header the SPA must send, equal to the cookie.
	HeaderName = "X-CSRF-Token"
)

// GenerateToken returns a cryptographically random token (64 hex chars).
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetCookie writes the double-submit CSRF cookie. The cookie is intentionally NOT
// HttpOnly so the SPA can read it and echo it back as the X-CSRF-Token header.
// secure should be true in production (TLS everywhere); path "/" so it is sent on
// every API request, and SameSite=Lax provides baseline browser CSRF defence.
func SetCookie(w http.ResponseWriter, domain string, secure bool) {
	token, err := GenerateToken()
	if err != nil {
		slog.Error("csrf: generate token", "error", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Domain:   domain,
		// MaxAge:0 keeps the cookie session-scoped (cleared when the browser
		// closes). This is safe because EnsureCookie re-issues a fresh token on
		// every /control request (and on /me), so the SPA never operates with a
		// stale or missing double-submit token.
		MaxAge:   0, // session-scoped; re-issued via EnsureCookie on /control
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// EnsureCookie is middleware that sets the CSRF cookie on the response when it is
// not already present. Apply it to authenticated entrypoints (login, /me) so the
// SPA always has a token available for subsequent mutating requests.
func EnsureCookie(domain string, secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := r.Cookie(CookieName); err != nil {
				SetCookie(w, domain, secure)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Middleware returns middleware enforcing the double-submit CSRF pattern for
// mutating requests (POST/PATCH/PUT/DELETE) whose path matches one of
// protectPrefixes. It requires the X-CSRF-Token header to equal the csrf_token
// cookie value, rejecting missing or mismatched tokens with HTTP 403. Non-mutating
// methods and paths outside the protected prefixes pass through untouched.
//
// Call with protectPrefixes such as []string{"/api/v1/control/"} to scope
// protection to the Control Panel (Critical finding C1). NOTE: the prefix MUST
// include the "/api/v1" base path — the /control group is mounted under
// r.Route("/api/v1") in internal/server/server.go, so the real request paths are
// "/api/v1/control/...". A bare "/control/" prefix would never match and silently
// disable protection. The set can be widened to all app routes once public POST
// endpoints are exempted.
func Middleware(protectPrefixes []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutatingMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			if !pathHasPrefix(r.URL.Path, protectPrefixes) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				writeRejection(w)
				return
			}
			header := r.Header.Get(HeaderName)
			if header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
				writeRejection(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isMutatingMethod reports whether an HTTP method changes server state and thus
// must carry a CSRF token.
func isMutatingMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// pathHasPrefix reports whether path begins with any of the given prefixes.
func pathHasPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// writeRejection writes the standard 403 response for a missing or mismatched CSRF
// token, matching the project's error envelope (conventions/api-response-format.md).
func writeRejection(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"code":"csrf_token_invalid","message":"Invalid or missing CSRF token"}}`))
}
