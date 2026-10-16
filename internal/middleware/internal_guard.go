package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"

	"github.com/jedafe/japa/internal/clientip"
)

// InternalGuard returns a middleware that restricts access to internal
// callers only.
//
// When expectedToken is non-empty (production), the request MUST carry the
// matching X-Internal-Token header — even from loopback. Trusting loopback
// alone is insufficient: any local process or SSRF primitive can reach
// 127.0.0.1, so bare-loopback access would let an attacker pivot to these
// endpoints without the secret.
//
// When expectedToken is empty (dev), loopback access is allowed without a
// token so local development works without configuring a secret. The client
// IP is resolved via clientip.FromRequest (not the raw RemoteAddr) so the
// trusted-proxy model is enforced consistently with the rest of the stack.
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

			// A configured token is required: loopback alone never suffices.
			if expectedToken != "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"code": "forbidden", "message": "Access to this endpoint is restricted to internal callers"},
				})

				return
			}

			// Dev mode (no token configured): fall back to a loopback check
			// resolved through the trusted-proxy-aware client IP helper.
			if ip := net.ParseIP(clientip.FromRequest(r)); ip != nil && ip.IsLoopback() {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"code": "forbidden", "message": "Access to this endpoint is restricted to internal callers"},
			})
		})
	}
}

// ClientIP extracts the client IP from the request, honouring forwarding
// headers only when the connecting peer is a trusted proxy.
func ClientIP(r *http.Request) string {
	return clientip.FromRequest(r)
}
