package geoip

import (
	"context"
	"net/http"

	"github.com/jedafe/japa/internal/clientip"
)

// Middleware adds GeoContext to every request's context. If no geoip
// database is loaded or the IP can't be resolved, the context still
// gets a GeoContext with IsValid=false - downstream handlers check
// IsValid before using geo data.
//
// The client IP is resolved via clientip.FromRequest, which only honours
// X-Forwarded-For / X-Real-IP when the immediate peer is a trusted proxy
// (JC_TRUSTED_PROXIES). The previous implementation trusted those headers
// unconditionally, allowing IP spoofing for geo resolution.
//
// This middleware MUST be wired from day one. Retrofitting it later
// would require touching every handler that needs geo data.
func Middleware(reader *Reader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientip.FromRequest(r)
			gc := reader.Lookup(ip)
			ctx := context.WithValue(r.Context(), GeoContextKey, gc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
