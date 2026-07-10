package middleware

import (
	"net"
	"net/http"
	"strings"
)

// InternalGuard returns a middleware that restricts access to internal
// callers only. A request is considered internal if:
//
//  1. It carries a matching X-Internal-Token header, OR
//  2. The client IP is a loopback address (127.0.0.1, ::1, etc.)
//
// This protects endpoints that should never be called from the public
// internet (e.g. server-to-server rendering helpers).
func InternalGuard(expectedToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check token header first — fastest path for service-to-service calls.
			if expectedToken != "" && r.Header.Get("X-Internal-Token") == expectedToken {
				next.ServeHTTP(w, r)
				return
			}

			// Fall back to loopback IP check.
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}

			if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
				next.ServeHTTP(w, r)
				return
			}

			http.Error(w, "forbidden: internal endpoint", http.StatusForbidden)
		})
	}
}

// ClientIP extracts the client IP from the request, preferring
// X-Forwarded-For and X-Real-IP headers when present.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain.
		if idx := strings.IndexByte(xff, ','); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
