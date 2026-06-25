# Section 37 — IP Geolocation

**Status:** ✅ Locked

## Decision

MaxMind GeoLite2-City database, self-hosted (free tier, no API key needed, just attribution). Wired into Go request context from day one — retrofitting this later would require touching every handler. The `geoip2-golang` library reads the binary `.mmdb` file directly in-process (no network call, sub-millisecond lookup).

```go
// Middleware: runs on every request, adds GeoContext to ctx
func GeoIPMiddleware(db *geoip2.Reader) func(http.Handler) http.Handler {
  return func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      ip := realIP(r) // handles X-Forwarded-For, X-Real-IP
      record, err := db.City(net.ParseIP(ip))
      if err == nil {
        ctx := context.WithValue(r.Context(), GeoContextKey, GeoContext{
          CountryCode:  record.Country.IsoCode,       // "NG", "CA", "GB"
          CountryName:  record.Country.Names["en"],
          Subdivisions: record.Subdivisions,
          City:         record.City.Names["en"],
          TimeZone:     record.Location.TimeZone,
          Latitude:     record.Location.Latitude,
          Longitude:    record.Location.Longitude,
        })
        next.ServeHTTP(w, r.WithContext(ctx))
        return
      }
      next.ServeHTTP(w, r)
    })
  }
}
```

## Database Update

GeoLite2 is updated weekly. An Asynq low-tier task `maintenance:update_geoip_db` runs on Sunday 02:00 UTC — downloads the latest `.mmdb` from MaxMind (requires free account license key in `JC_MAXMIND_LICENSE_KEY`), atomically swaps the reader pointer without restarting the server.

## Consumers

| Consumer | Usage |
|---|---|
| Ad targeting (Section 25) | `CountryCode` for geo-targeted campaigns |
| Primary Space suggestion | Pre-select user's country Space during onboarding (Section 09) |
| Analytics (Section 40) | Country/city breakdown in platform analytics |
| Rate limiting (ADR-022) | Per-country rate limit tiers if needed |
| IP blocking (Section 45) | CIDR + country block enforcement |
| Timezone default (Section 23) | `TimeZone` prefills user's timezone on first visit |
| Sitemap exclusion (Section 26) | Empty geo spaces (low post_count) excluded from sitemap |

## See Also

ADR-037.
