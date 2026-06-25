package geoip

import (
	"context"
	"net/http"
	"strings"
)

// realIP extracts the client IP from the request, respecting
// X-Forwarded-For and X-Real-IP headers set by the reverse proxy.
func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// Fall back to RemoteAddr, stripping the port
	idx := strings.LastIndex(r.RemoteAddr, ":")
	if idx > 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

// Middleware adds GeoContext to every request's context. If no geoip
// database is loaded or the IP can't be resolved, the context still
// gets a GeoContext with IsValid=false - downstream handlers check
// IsValid before using geo data.
//
// This middleware MUST be wired from day one. Retrofitting it later
// would require touching every handler that needs geo data.
func Middleware(reader *Reader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			gc := reader.Lookup(ip)
			ctx := context.WithValue(r.Context(), GeoContextKey, gc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
