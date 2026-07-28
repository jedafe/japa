// Package visitor implements the VisitorContext resolver for geo-aware
// personalization (ADR-064). It runs after the GeoIP middleware and resolves
// the effective content country for the current request, using this priority:
//
//  1. Explicit ?geo= query parameter (when present and valid)
//  2. Authenticated user's content_country_preference (when set)
//  3. GeoIP country code (from the GeoIP middleware)
//  4. System default ("NG")
//
// The resolved context is cached in Valkey under jc:visitor:ctx:{session_id}
// with a 30-minute TTL (ADR-064) to avoid repeated DB lookups for
// authenticated users. The cache key is the JWT JTI (session ID) for
// authenticated requests and the client IP for anonymous requests.
//
// Ref: japaconnect-engineering/sections/37-ip-geolocation.md (VisitorContext)
//
//	japaconnect-engineering/adr/ADR-064-geo-aware-personalization.md
package visitor

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// ContextKey is the context key for the resolved VisitorContext.
type ContextKey struct{}

// VisitorContext is the resolved geo-personalization context for a request.
type VisitorContext struct {
	// ContentCountry is the ISO 3166-1 alpha-2 country code used for
	// personalizing content, feeds, and search results.
	ContentCountry string

	// Source indicates where ContentCountry came from: "query", "preference",
	// "geoip", or "default".
	Source string

	// IsAuthenticated is true when the visitor has a valid session.
	IsAuthenticated bool

	// UserID is the authenticated user's UUID string, empty for anonymous.
	UserID string

	// GeoIPCountry is the raw GeoIP country code (may differ from
	// ContentCountry when the user has an override).
	GeoIPCountry string
}

// FromContext extracts the VisitorContext from the request context.
// Returns a zero-value context with ContentCountry="NG" when none is set.
func FromContext(ctx context.Context) VisitorContext {
	vc, ok := ctx.Value(ContextKey{}).(VisitorContext)
	if !ok {
		return VisitorContext{ContentCountry: "NG", Source: "default"}
	}
	return vc
}

// cacheTTL is the Valkey cache TTL for resolved visitor contexts (ADR-064).
const cacheTTL = 30 * time.Minute

// Resolver is the VisitorContext resolver middleware. It depends on:
//   - a read-only sqlc Queries for user preference lookups
//   - a Redis client for caching
//   - a settings reader for the geo_personalization.enabled killswitch
type Resolver struct {
	read     *gen.Queries
	rdb      valkeycompat.Cmdable
	settings *config.Settings
}

// NewResolver creates a VisitorContext resolver.
func NewResolver(read *gen.Queries, rdb valkeycompat.Cmdable, settings *config.Settings) *Resolver {
	return &Resolver{read: read, rdb: rdb, settings: settings}
}

// Middleware returns an HTTP middleware that resolves the VisitorContext for
// every request and adds it to the request context. It must be wired AFTER
// the GeoIP middleware and AFTER the auth middleware.
func (res *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vc := res.resolve(r)
		ctx := context.WithValue(r.Context(), ContextKey{}, vc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolve determines the VisitorContext for a single request.
func (res *Resolver) resolve(r *http.Request) VisitorContext {
	geoCtx := geoip.FromContext(r.Context())
	user := auth.FromContext(r.Context())
	t := tenant.FromContext(r.Context())

	vc := VisitorContext{
		GeoIPCountry: geoCtx.CountryCode,
	}

	// Killswitch (ADR-064): when geo_personalization.enabled is false,
	// short-circuit to the system default for every request. The ?geo=
	// override is still honored (it's an explicit user action, not
	// personalization), but preference/GeoIP sources are skipped.
	if res.settings != nil && t != nil {
		if !res.settings.GetBoolOr(r.Context(), t.ID, "geo_personalization.enabled", true) {
			if q := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("geo"))); len(q) == 2 {
				vc.ContentCountry = q
				vc.Source = "query"
			} else {
				vc.ContentCountry = "NG"
				vc.Source = "default"
			}
			if user != nil {
				vc.IsAuthenticated = true
				vc.UserID = user.UserID.String()
			}
			return vc
		}
	}

	// 1. Explicit ?geo= query parameter — always wins when present.
	if q := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("geo"))); len(q) == 2 {
		vc.ContentCountry = q
		vc.Source = "query"
		// Still set auth state for downstream use.
		if user != nil {
			vc.IsAuthenticated = true
			vc.UserID = user.UserID.String()
		}
		return vc
	}

	// Determine cache key and auth state.
	// ADR-064: cache key is jc:visitor:ctx:{session_id} for authenticated
	// users (using the JWT JTI as session ID) and jc:visitor:ctx:ip:{ip}
	// for anonymous visitors.
	var cacheKey string
	if user != nil {
		vc.IsAuthenticated = true
		vc.UserID = user.UserID.String()
		if user.SessionID != "" {
			cacheKey = "jc:visitor:ctx:" + user.SessionID
		} else {
			// Fallback for tokens without JTI (legacy/compat).
			cacheKey = "jc:visitor:ctx:user:" + user.UserID.String()
		}
	} else {
		cacheKey = "jc:visitor:ctx:ip:" + clientIP(r)
	}

	// Try cache first.
	if res.rdb != nil {
		if cached, err := res.rdb.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			parts := strings.SplitN(cached, "|", 2)
			vc.ContentCountry = parts[0]
			if len(parts) > 1 {
				vc.Source = parts[1]
			} else {
				vc.Source = "cached"
			}
			return vc
		}
	}

	// 2. Authenticated user's content_country_preference.
	if user != nil && t != nil {
		if pref, err := res.read.GetUserContentCountryPreference(r.Context(),
			gen.GetUserContentCountryPreferenceParams{
				TenantID: t.ID, UserID: user.UserID,
			}); err == nil && pref != nil && *pref != "" {
			vc.ContentCountry = strings.ToUpper(*pref)
			vc.Source = "preference"
			res.cache(r.Context(), cacheKey, vc.ContentCountry, vc.Source)
			return vc
		}
	}

	// 3. GeoIP country code.
	if geoCtx.IsValid && geoCtx.CountryCode != "" {
		vc.ContentCountry = strings.ToUpper(geoCtx.CountryCode)
		vc.Source = "geoip"
		res.cache(r.Context(), cacheKey, vc.ContentCountry, vc.Source)
		return vc
	}

	// 4. System default.
	vc.ContentCountry = "NG"
	vc.Source = "default"
	res.cache(r.Context(), cacheKey, vc.ContentCountry, vc.Source)
	return vc
}

// cache writes the resolved context to Valkey (best-effort, never blocks).
func (res *Resolver) cache(ctx context.Context, key, country, source string) {
	if res.rdb == nil {
		return
	}
	val := country + "|" + source
	_ = res.rdb.Set(ctx, key, val, cacheTTL).Err()
}

// clientIP extracts the client IP from the request, respecting proxy headers.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	idx := strings.LastIndex(r.RemoteAddr, ":")
	if idx > 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

// SetUserPreference updates the user's content_country_preference and
// invalidates their cached VisitorContext.
func SetUserPreference(ctx context.Context, q *gen.Queries, rdb valkeycompat.Cmdable, tenantID, userID pgtype.UUID, country string) error {
	c := strings.ToUpper(strings.TrimSpace(country))
	if c == "" {
		// Setting empty string clears the preference (falls back to GeoIP).
		if err := q.SetUserContentCountryPreference(ctx, gen.SetUserContentCountryPreferenceParams{
			TenantID: tenantID, UserID: userID,
		}); err != nil {
			return err
		}
	} else {
		cc := &c
		if err := q.SetUserContentCountryPreference(ctx, gen.SetUserContentCountryPreferenceParams{
			TenantID: tenantID, UserID: userID, ContentCountryPreference: cc,
		}); err != nil {
			return err
		}
	}

	// Invalidate cache. Session-based keys (jc:visitor:ctx:{jti}) can't be
	// enumerated without SCAN, so we delete the user-based fallback key and
	// rely on the 30-min TTL for session-scoped keys. Preference changes are
	// infrequent and 30-min staleness is acceptable per ADR-064.
	if rdb != nil {
		_ = rdb.Del(ctx, "jc:visitor:ctx:user:"+userID.String()).Err()
	}
	return nil
}
