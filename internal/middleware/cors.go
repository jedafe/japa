// Package middleware provides cross-cutting HTTP middleware for JapaConnect.
//
// CORS middleware sets the headers that allow cross-origin requests from
// configured frontend domains. Origins are read from the system_settings
// cors.allowed_origins key (or JC_CORS_ORIGINS env var). The middleware
// short-circuits OPTIONS preflight requests with a 204 so the browser can
// proceed with the actual request immediately.
package middleware

import (
	"net/http"
	"strings"
)

// CORS returns middleware that sets CORS headers from the given list of
// allowed origins. Origins are matched exactly — no wildcard support.
// When the request Origin header is empty or not in the allowed set,
// no CORS headers are set and the request passes through normally.
//
// The allowedOrigins parameter is typically populated from system_settings
// cors.allowed_origins or the JC_CORS_ORIGINS environment variable.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o != "" {
			originSet[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			if _, ok := originSet[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-API-Key")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
