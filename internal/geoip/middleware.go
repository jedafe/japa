package geoip

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/jedafe/japa/internal/clientip"
)

// countryHeaderResolver returns the request header through which the
// configured edge CDN reports the visitor's ISO-3166 alpha-2 country code
// (ADR-134): Cloudflare → CF-IPCountry, BunnyCDN → CDN-RequestCountryCode.
// Wired once at server startup via SetCountryHeaderResolver; nil or ""
// disables the fallback. The header is only trusted when the immediate peer
// is a trusted proxy — a direct client can spoof it, so trust is gated by
// clientip.IsTrustedPeer, matching how forwarding headers are handled.
// Stored atomically so reads stay lock-free and tests can swap it safely
// (same pattern as clientip.trustedPtr).
var countryHeaderResolver atomic.Pointer[func() string]

// SetCountryHeaderResolver wires the CDN-provider-aware header source.
func SetCountryHeaderResolver(fn func() string) {
	if fn == nil {
		countryHeaderResolver.Store(nil)

		return
	}

	countryHeaderResolver.Store(&fn)
}

// countryHeader returns the header name to consult, or "".
func countryHeader() string {
	if fn := countryHeaderResolver.Load(); fn != nil {
		return (*fn)()
	}

	return ""
}

// normalizeCountryCode validates a CDN-provided country code: two ASCII
// letters, uppercased. Cloudflare sentinels XX (unknown) and T1 (Tor)
// resolve to "" so they never surface as a resolved country.
func normalizeCountryCode(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if len(v) != 2 || v == "XX" || v == "T1" {
		return ""
	}

	for _, c := range v {
		if c < 'A' || c > 'Z' {
			return ""
		}
	}

	return v
}

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
// Edge CDN fallback (ADR-134): when the local MaxMind lookup yields no
// country — missing database, unknown IP — and a CDN provider is configured,
// the provider's country header fills CountryCode if the immediate peer is
// a trusted proxy. The local DB stays authoritative whenever it resolves;
// the header only covers what the DB cannot.
//
// This middleware MUST be wired from day one. Retrofitting it later
// would require touching every handler that needs geo data.
func Middleware(reader *Reader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientip.FromRequest(r)
			gc := cdnCountryFallback(reader.Lookup(ip), r, countryHeader())
			ctx := context.WithValue(r.Context(), GeoContextKey, gc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// cdnCountryFallback fills CountryCode from the configured CDN's
// visitor-country header when the MaxMind lookup produced none. The header
// is only trusted from a trusted-proxy peer — a direct client could forge
// it. Empty header, a resolved country, or an untrusted peer all leave the
// context untouched.
func cdnCountryFallback(gc GeoContext, r *http.Request, header string) GeoContext {
	if gc.CountryCode != "" || header == "" || !clientip.IsTrustedPeer(r) {
		return gc
	}

	if cc := normalizeCountryCode(r.Header.Get(header)); cc != "" {
		gc.CountryCode = cc
		gc.IsValid = true
	}

	return gc
}
