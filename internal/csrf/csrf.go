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

	errpkg "github.com/jedafe/japa/internal/errors"
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
		Name:   CookieName,
		Value:  token,
		Path:   "/",
		Domain: domain,
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

// ExemptFunc reports whether CSRF enforcement should be skipped for a request.
// It is consulted only for mutating requests that match a protected prefix.
// Use it to skip requests authenticated by a non-cookie credential (e.g.
// Authorization: Bearer or X-API-Key), which are immune to browser CSRF because
// a cross-site request cannot set arbitrary request headers. Skipping them keeps
// service-to-service and API-token clients working without a double-submit cookie.
type ExemptFunc func(r *http.Request) bool

// OriginAllowFunc reports whether the given Origin header value is an allowed
// origin for this API. It is used by the Origin validation defense-in-depth
// layer (see WithOriginCheck). The function should return false for empty
// origins and disallowed origins, and true only for explicitly allowed origins
// (e.g. the known frontend domains).
type OriginAllowFunc func(origin string) bool

// middlewareConfig holds optional CSRF middleware behaviour configured via
// functional options.
type middlewareConfig struct {
	originAllowed OriginAllowFunc
}

// Option configures optional CSRF middleware behaviour.
type Option func(*middlewareConfig)

// WithOriginCheck adds Origin header validation as a defense-in-depth layer
// against CSRF. When provided, mutating requests whose Origin header is present
// but not allowed are rejected with 403 before the double-submit cookie check.
// Requests without an Origin header (server-to-server, API clients) skip the
// check and fall through to the normal double-submit validation.
//
// This protects against subdomain cookie injection attacks where an attacker
// can set the csrf_token cookie on the victim's browser via a vulnerable
// subdomain: even if both the cookie and header are forged to the same value,
// the cross-site request carries a foreign Origin that this check rejects.
func WithOriginCheck(fn OriginAllowFunc) Option {
	return func(c *middlewareConfig) {
		c.originAllowed = fn
	}
}

// Middleware returns middleware enforcing the double-submit CSRF pattern for
// mutating requests (POST/PATCH/PUT/DELETE) whose path matches one of
// protectPrefixes. It requires the X-CSRF-Token header to equal the csrf_token
// cookie value, rejecting missing or mismatched tokens with HTTP 403. Non-mutating
// methods and paths outside the protected prefixes pass through untouched.
//
// If exempt is non-nil and returns true for a request, enforcement is skipped
// (see ExemptFunc). This is how API-token / service-to-service auth paths stay
// out of scope.
//
// Optional behaviour can be configured with opts (see WithOriginCheck).
//
// Call with protectPrefixes such as []string{"/control/"} to scope
// protection to the Control Panel (Critical finding C1). NOTE: the /control group
// is mounted at the TOP LEVEL in internal/server/server.go (r.Route("/control", ...)
// lives OUTSIDE the /api/v1 group), so the real request paths are "/control/...".
// A "/api/v1/control/" prefix would never match and silently disable protection.
// The set can be widened to all app routes once public POST endpoints are exempted.
func Middleware(protectPrefixes []string, exempt ExemptFunc, opts ...Option) func(http.Handler) http.Handler {
	cfg := &middlewareConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

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
			if exempt != nil && exempt(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Defense-in-depth: reject mutating requests whose Origin header
			// is present but not in the allowlist. A cross-site form POST or
			// fetch carries the attacker's Origin, which this check blocks
			// even if the double-submit cookie was forged via subdomain
			// injection. Absent Origin headers (server-to-server, API clients
			// that are already exempt) skip this check.
			if cfg.originAllowed != nil {
				if origin := r.Header.Get("Origin"); origin != "" && !cfg.originAllowed(origin) {
					writeRejection(w)
					return
				}
			}

			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				// No CSRF cookie and no non-cookie credential (exempt would have
				// passed above). This is an unauthenticated request — let the
				// downstream RequireAuth middleware return 401 missing_token
				// instead of a misleading 403 csrf_token_invalid. A real CSRF
				// attempt always carries the victim's cookie (the browser sends
				// it automatically), so reaching this branch with no cookie
				// means the request is simply unauthenticated, not a CSRF attack.
				next.ServeHTTP(w, r)
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

// IsNonCookieAuthenticated reports whether a request carries a non-cookie
// credential. Requests authenticated this way (Bearer JWT or X-API-Key) are not
// vulnerable to CSRF — a browser cannot forge a custom request header on a
// cross-site request — so CSRF enforcement is unnecessary and would only break
// legitimate API clients. Cookie-authenticated browser requests (the admin /
// control SPAs) have no such header and therefore remain protected.
func IsNonCookieAuthenticated(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" {
		return true
	}
	if r.Header.Get("X-API-Key") != "" {
		return true
	}
	return false
}

// TokenHandler returns an http.HandlerFunc that issues a double-submit CSRF
// token: it ensures the csrf_token cookie is set on the response and echoes the
// same token back in the JSON body using the standard {data:{...}} envelope.
// The SPA calls this once at bootstrap (and again after a 403) to learn the token
// it must send as the X-CSRF-Token header on subsequent mutating requests.
//
// If a valid csrf_token cookie is already present on the request, the existing
// value is reused instead of generating a new one. This avoids token rotation
// races when multiple concurrent requests hit /csrf (e.g. the SSR hooks.server.ts
// fetches /csrf on every page load) — each response would otherwise set a
// different cookie value, invalidating tokens issued by earlier responses.
func TokenHandler(domain string, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Reuse the existing cookie value if present so concurrent /csrf calls
		// don't clobber each other's tokens.
		if existing, err := r.Cookie(CookieName); err == nil && existing.Value != "" {
			errpkg.WriteJSON(w, http.StatusOK, map[string]any{
				"data": map[string]any{"csrfToken": existing.Value},
			})
			return
		}
		token, err := GenerateToken()
		if err != nil {
			slog.Error("csrf: generate token", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "csrf_token_error", "failed to issue CSRF token")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     CookieName,
			Value:    token,
			Path:     "/",
			Domain:   domain,
			MaxAge:   0, // session-scoped
			HttpOnly: false,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"csrfToken": token},
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

// writeRejection writes the standard 403 response for a missing or mismatched
// CSRF token, using the same error envelope as writeError for consistency
// (conventions/api-response-format.md).
func writeRejection(w http.ResponseWriter) {
	errpkg.WriteError(w, http.StatusForbidden, "csrf_token_invalid", "Invalid or missing CSRF token")
}
