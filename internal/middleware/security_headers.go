package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders returns middleware that sets security-related HTTP
// headers on every response. The Content-Security-Policy header varies
// by route prefix:
//   - /docs and /developer/swagger: permissive policy allowing inline
//     scripts and styles for documentation UIs.
//   - /api/: strict policy that blocks all resource loading.
//   - All other routes: no CSP header (handled by the browser default).
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			// X-XSS-Protection is intentionally NOT set: the auditor was removed
			// from every current browser, and "1; mode=block" can itself be used
			// for XS-Leaks (the filter blocks/rewrites pages based on reflected
			// content, leaking information cross-origin).
			w.Header().Set("X-XSS-Protection", "0")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			// HSTS: emitted unconditionally — browsers ignore STS on plain-HTTP
			// responses, so local dev over :8080 is unaffected while production
			// responses (TLS terminated at the edge) carry it through.
			// includeSubDomains is safe because every production surface lives
			// under the same TLS'd base domain; preload is deliberately omitted
			// — list submission is a one-way commitment.
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

			path := r.URL.Path
			if strings.HasPrefix(path, "/docs") || strings.HasPrefix(path, "/developer/swagger") {
				w.Header().Set("Content-Security-Policy",
					"default-src 'self'; script-src 'self' 'unsafe-inline' unpkg.com; style-src 'self' 'unsafe-inline' unpkg.com; img-src 'self' data:; font-src 'self' unpkg.com")
			} else if strings.HasPrefix(path, "/api/") {
				w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			}

			next.ServeHTTP(w, r)
		})
	}
}
