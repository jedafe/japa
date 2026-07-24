package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"

	"github.com/jedafe/japa/internal/clientip"
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
			if expectedToken != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Token")), []byte(expectedToken)) == 1 {
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

// ClientIP extracts the client IP from the request, honouring forwarding
// headers only when the connecting peer is a trusted proxy.
func ClientIP(r *http.Request) string {
	return clientip.FromRequest(r)
}
