// Package visitor implements the VisitorContext resolver for geo-aware
// personalization (ADR-064). It runs after the GeoIP middleware and resolves
// the effective content country for the current request, using this priority:
//
//  1. Explicit ?geo= query parameter (when present and valid)
//  2. Authenticated user's content_country_preference (when set)
//  3. Archetype-resolved ContentCountry (origin_visitor → destination/origin;
//     diaspora/global_traveler → current country)
//  4. GeoIP country code (from the GeoIP middleware)
//  5. System default ("NG")
//
// The resolved context is cached in Valkey under jc:visitor:ctx:{session_id}
// with a 30-minute TTL (ADR-064) to avoid repeated DB lookups for
// authenticated users. The cache key is the JWT JTI (session ID) for
// authenticated requests and the client IP for anonymous requests.
//
// Archetype market lists (origin vs destination countries) are config-driven
// via system_settings geo_personalization.origin_markets and
// geo_personalization.destination_markets (CSV of ISO codes) — never hardcoded
// (ADR-064 Consequences).
//
// Ref: japaconnect-engineering/sections/37-ip-geolocation.md (VisitorContext)
//
//	japaconnect-engineering/adr/ADR-064-geo-aware-personalization.md
package visitor

import (
	"context"
	"encoding/json"
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

// Archetype values (ADR-064).
const (
	ArchetypeOriginVisitor  = "origin_visitor"
	ArchetypeDiaspora       = "diaspora"
	ArchetypeGlobalTraveler = "global_traveler"
)

// VisitorContext is the resolved geo-personalization context for a request.
type VisitorContext struct {
	// ContentCountry is the ISO 3166-1 alpha-2 country code used for
	// personalizing content, feeds, and search results.
	ContentCountry string

	// Source indicates where ContentCountry came from: "query", "preference",
	// "archetype", "geoip", "cached", or "default".
	Source string

	// IsAuthenticated is true when the visitor has a valid session.
	IsAuthenticated bool

	// UserID is the authenticated user's UUID string, empty for anonymous.
	UserID string

	// GeoIPCountry is the raw GeoIP country code (may differ from
	// ContentCountry when the user has an override).
	GeoIPCountry string

	// OriginCountry is the visitor's origin country (ADR-064). No explicit
	// origin_country profile field exists yet, so this falls back to the
	// GeoIP country. When an origin_country custom field is added later,
	// populate it here (profile origin > IP country).
	OriginCountry string

	// CurrentCountry is where the visitor is now (GeoContext.CountryCode).
	CurrentCountry string

	// DestinationCountry is the user's declared relocation destination
	// (user_profiles custom field destination_country). Empty for guests or
	// users who haven't set it.
	DestinationCountry string

	// JapaStage is the user's relocation stage from the japa_status custom
	// field (newly_japa|settled|planning|...). Empty for guests.
	JapaStage string

	// Language is the resolved i18n language (Accept-Language primary tag).
	// Full i18n middleware integration is deferred; this is a best-effort
	// resolution from the Accept-Language header.
	Language string

	// Archetype is the resolved visitor archetype (ADR-064):
	// origin_visitor | diaspora | global_traveler.
	Archetype string

	// Confidence is the resolver's confidence in the archetype (0.0–1.0).
	Confidence float32
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
//   - a read-only sqlc Queries for user preference + custom-field lookups
//   - a Redis client for caching
//   - a settings reader for the geo_personalization.enabled killswitch and
//     the config-driven origin/destination market lists
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
		GeoIPCountry:   geoCtx.CountryCode,
		CurrentCountry: geoCtx.CountryCode,
		OriginCountry:  geoCtx.CountryCode, // profile origin > IP; no origin field yet
		Language:       resolveLanguage(r),
	}
	if user != nil {
		vc.IsAuthenticated = true
		vc.UserID = user.UserID.String()
	}

	// Killswitch (ADR-064): when geo_personalization.enabled is false,
	// short-circuit to the system default for every request. The ?geo=
	// override is still honored (it's an explicit user action, not
	// personalization), but preference/archetype/GeoIP sources are skipped.
	if res.settings != nil && t != nil {
		if !res.settings.GetBoolOr(r.Context(), t.ID, "geo_personalization.enabled", true) {
			if q := normalizeCountry(r.URL.Query().Get("geo")); q != "" {
				vc.ContentCountry = q
				vc.Source = "query"
			} else {
				vc.ContentCountry = "NG"
				vc.Source = "default"
			}
			return vc
		}
	}

	// 1. Explicit ?geo= query parameter — always wins when present.
	if q := normalizeCountry(r.URL.Query().Get("geo")); q != "" {
		vc.ContentCountry = q
		vc.Source = "query"
		return vc
	}

	// Determine cache key and auth state.
	// ADR-064: cache key is jc:visitor:ctx:{session_id} for authenticated
	// users (using the JWT JTI as session ID) and jc:visitor:ctx:ip:{ip}
	// for anonymous visitors.
	var cacheKey string
	if user != nil {
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
			if cv, ok := unmarshalCache(cached); ok {
				// Preserve auth state (not cached) and language (request-scoped).
				cv.IsAuthenticated = vc.IsAuthenticated
				cv.UserID = vc.UserID
				cv.Language = vc.Language
				return cv
			}
			// Stale/old-format entry falls through to re-resolution below.
		}
	}

	// 2. Authenticated user's content_country_preference (explicit override).
	if user != nil && t != nil {
		if pref, err := res.read.GetUserContentCountryPreference(r.Context(),
			gen.GetUserContentCountryPreferenceParams{
				TenantID: t.ID, UserID: user.UserID,
			}); err == nil && pref != nil && *pref != "" {
			vc.ContentCountry = strings.ToUpper(*pref)
			vc.Source = "preference"
			// Still resolve archetype for analytics/surfacing, using profile
			// fields + GeoIP, but the preference wins for ContentCountry.
			res.enrichArchetype(r.Context(), &vc, user, t, "")
			res.cache(r.Context(), cacheKey, vc)
			return vc
		}
	}

	// 3. Archetype resolution — synthesizes ContentCountry from the visitor's
	//    signals (ADR-064). For authenticated users this loads japa_status +
	//    destination_country custom fields. For guests it classifies from
	//    GeoIP alone.
	res.enrichArchetype(r.Context(), &vc, user, t, cacheKey)
	return vc
}

// enrichArchetype loads the user's geo profile fields (when authenticated),
// resolves the archetype, and sets ContentCountry from the archetype when no
// explicit override is present. When cacheKey is non-empty the resolved
// context is cached.
func (res *Resolver) enrichArchetype(ctx context.Context, vc *VisitorContext, user *auth.AuthenticatedUser, t *tenant.Tenant, cacheKey string) {
	// Load config-driven market lists (ADR-064: config, not hardcoded).
	originMarkets := res.marketSet(ctx, t, "geo_personalization.origin_markets", "NG,GH,ZA,KE,EG")
	destMarkets := res.marketSet(ctx, t, "geo_personalization.destination_markets", "GB,CA,US,DE,AU,AE")

	// Load profile signals for authenticated users.
	if user != nil && t != nil {
		fields, err := res.read.GetUserGeoProfileFields(ctx, gen.GetUserGeoProfileFieldsParams{
			TenantID: t.ID, UserID: user.UserID,
		})
		if err == nil {
			vc.JapaStage = strings.ToLower(strings.TrimSpace(fields.JapaStatus))
			vc.DestinationCountry = normalizeCountry(fields.DestinationCountry)
		}
	}

	// Resolve archetype (ADR-064). CurrentCountry (GeoIP) is the primary
	// signal; japa_status and destination_country refine it.
	archetype, contentCountry, confidence := resolveArchetype(
		vc.CurrentCountry, vc.JapaStage, vc.DestinationCountry,
		originMarkets, destMarkets,
	)
	vc.Archetype = archetype
	vc.Confidence = confidence
	vc.ContentCountry = contentCountry

	// If archetype resolution produced no ContentCountry (no GeoIP), fall back
	// to the system default rather than serving an empty country downstream.
	if vc.ContentCountry == "" {
		vc.ContentCountry = "NG"
	}

	if vc.Source == "" {
		vc.Source = "archetype"
	}

	if cacheKey != "" {
		res.cache(ctx, cacheKey, *vc)
	}
}

// resolveArchetype classifies a visitor into one of the three ADR-064
// archetypes and computes the archetype-resolved ContentCountry. It is a pure
// function of the visitor signals + the config-driven market sets so it can
// be unit-tested without a database or Redis.
//
// Classification (ADR-064):
//   - origin_visitor  — current country is an origin market and not settled.
//     ContentCountry = destination if set, else current (origin).
//   - diaspora       — current country is a destination market and/or
//     japa_status == "settled". ContentCountry = current country.
//   - global_traveler — neither pattern. ContentCountry = current, low weight.
func resolveArchetype(current, stage, destination string, originMarkets, destMarkets map[string]bool) (archetype, contentCountry string, confidence float32) {
	isOrigin := current != "" && originMarkets[current]
	isDest := current != "" && destMarkets[current]
	settled := stage == "settled"

	switch {
	case isOrigin && !settled:
		archetype = ArchetypeOriginVisitor
		confidence = 0.8
		if destination != "" {
			contentCountry = destination
		} else {
			contentCountry = current
		}
	case isDest || settled:
		archetype = ArchetypeDiaspora
		confidence = 0.85
		if isDest && settled {
			confidence = 0.95
		}
		contentCountry = current
	default:
		archetype = ArchetypeGlobalTraveler
		confidence = 0.5
		contentCountry = current
	}
	return archetype, contentCountry, confidence
}

// marketSet reads a CSV system_setting and returns the set of uppercase ISO
// codes. The fallback is used when the setting is unset or the resolver has
// no settings reader (e.g. tests).
func (res *Resolver) marketSet(ctx context.Context, t *tenant.Tenant, key, fallback string) map[string]bool {
	out := make(map[string]bool)
	raw := fallback
	if res.settings != nil && t != nil {
		raw = res.settings.GetStringOr(ctx, t.ID, key, fallback)
	}
	for _, c := range strings.Split(raw, ",") {
		if c = normalizeCountry(c); c != "" {
			out[c] = true
		}
	}
	return out
}

// cache writes the resolved context to Valkey (best-effort, never blocks).
func (res *Resolver) cache(ctx context.Context, key string, vc VisitorContext) {
	if res.rdb == nil {
		return
	}
	if b, err := json.Marshal(vc); err == nil {
		_ = res.rdb.Set(ctx, key, b, cacheTTL).Err()
	}
}

// unmarshalCache decodes a JSON-cached VisitorContext. Returns ok=false for
// old-format (country|source) entries so the caller re-resolves.
func unmarshalCache(s string) (VisitorContext, bool) {
	if len(s) == 0 || s[0] != '{' {
		return VisitorContext{}, false
	}
	var vc VisitorContext
	if err := json.Unmarshal([]byte(s), &vc); err != nil {
		return VisitorContext{}, false
	}
	if vc.ContentCountry == "" {
		vc.ContentCountry = "NG"
		vc.Source = "default"
	}
	return vc, true
}

// normalizeCountry trims, uppercases, and validates a 2-letter ISO code.
func normalizeCountry(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if len(c) != 2 {
		return ""
	}
	return c
}

// resolveLanguage extracts the primary 2-letter language tag from the
// Accept-Language header. Full i18n middleware integration is deferred.
func resolveLanguage(r *http.Request) string {
	accept := r.Header.Get("Accept-Language")
	if accept == "" {
		return "en"
	}
	// "en-US,en;q=0.9" → "en"
	if i := strings.IndexByte(accept, ','); i >= 0 {
		accept = accept[:i]
	}
	if i := strings.IndexByte(accept, '-'); i >= 0 {
		accept = accept[:i]
	}
	accept = strings.ToLower(strings.TrimSpace(accept))
	if accept == "" {
		return "en"
	}
	return accept
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
