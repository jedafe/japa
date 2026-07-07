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
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

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
