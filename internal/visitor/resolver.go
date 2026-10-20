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
	"hash/fnv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/valkey"
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
//
//nolint:revive // exported: stutter naming is intentional
type VisitorContext struct {
	// ContentCountry is the ISO 3166-1 alpha-2 country code used for
	// personalizing content, feeds, and search results.
	ContentCountry string `json:"content_country"`

	// ContentRegion is the UN region of the resolved ContentCountry (e.g.
	// "Africa", "Asia"), used for the region-level feed fallback.
	ContentRegion string `json:"content_region"`

	// Source indicates where ContentCountry came from: "query", "preference",
	// "archetype", "geoip", "cached", or "default".
	Source string `json:"source"`

	// IsAuthenticated is true when the visitor has a valid session.
	IsAuthenticated bool `json:"is_authenticated"`

	// UserID is the authenticated user's UUID string, empty for anonymous.
	UserID string `json:"user_id"`

	// GeoIPCountry is the raw GeoIP country code (may differ from
	// ContentCountry when the user has an override).
	GeoIPCountry string `json:"geoip_country"`

	// OriginCountry is the visitor's origin/home country (ADR-064 + ADR-064a).
	// Populated from the origin_country custom field when set; falls back to
	// GeoIP country when the field is empty (guests or users who haven't set it).
	OriginCountry string `json:"origin_country"`

	// CurrentCountry is where the visitor is now. When the user has set
	// current_geo_space_id, this is resolved from the Space hierarchy
	// (ADR-064a). Otherwise falls back to GeoContext.CountryCode.
	CurrentCountry string `json:"current_country"`

	// DestinationCountry is the user's declared relocation destination
	// (user_profiles custom field destination_country). Empty for guests or
	// users who haven't set it.
	DestinationCountry string `json:"destination_country"`

	// JapaStage is the user's relocation stage from the japa_status custom
	// field (newly_japa|settled|planning|...). Empty for guests.
	JapaStage string `json:"japa_stage"`

	// JourneyStage is a normalized relocation stage aligned with §87 Japa
	// Stage Profiles (considering|planning|preparing|applied|approved|relocated|settled).
	// It is derived from JapaStage for feed ranking and UI.
	JourneyStage string `json:"journey_stage"`

	// VisaRoute is the user's intended visa route from the visa_route custom
	// field (student|work|business|...). Empty when unset.
	VisaRoute string `json:"visa_route"`

	// Purpose is the high-level travel purpose (study|work|business|relocation
	// |vacation|tourism). It may be set by the Japa Readiness Assessment or
	// the guest onboarding selector.
	Purpose string `json:"purpose"`

	// JapaIntent is the member's ADR-122 journey class (relocation|study|work
	// |family|visit|business) resolved from the user_profiles.japa_intent
	// column, or derived from Purpose when the column is unset. It is the
	// canonical personalization axis for nav/surface adaptation (ADR-128).
	JapaIntent string `json:"japa_intent"`

	// TravelDate is the user's planned travel/departure date in YYYY-MM-DD
	// format, or empty. Used to shift journey-stage urgency.
	TravelDate string `json:"travel_date"`

	// RoadmapSlugs are content slugs from the user's latest Japa Readiness
	// Assessment roadmap (§71). Empty if the user has not taken an assessment.
	RoadmapSlugs []string `json:"roadmap_slugs"`

	// WeakestDimensions are the lowest-scoring readiness dimensions from the
	// latest assessment (e.g. documents, finance). Empty if unset.
	WeakestDimensions []string `json:"weakest_dimensions"`

	// AssessmentThreshold is the overall readiness threshold label from the
	// latest Japa Readiness Assessment (e.g. "Japa Ready").
	AssessmentThreshold string `json:"assessment_threshold"`

	// Language is the resolved i18n language (Accept-Language primary tag).
	// Full i18n middleware integration is deferred; this is a best-effort
	// resolution from the Accept-Language header.
	Language string `json:"language"`

	// Archetype is the resolved visitor archetype (ADR-064):
	// origin_visitor | diaspora | global_traveler.
	Archetype string `json:"archetype"`

	// Confidence is the resolver's confidence in the archetype (0.0–1.0).
	Confidence float32 `json:"confidence"`

	// CurrentCity is the visitor's current city name (ADR-064a). Populated
	// from the user's current_geo_space_id Space name when set, or from
	// GeoIP city when available. Empty when neither is available.
	CurrentCity string `json:"current_city"`

	// OriginCity is the visitor's origin city name (ADR-064a). Populated
	// from profile data when available. Empty for guests or when not set.
	OriginCity string `json:"origin_city"`

	// DestCity is the visitor's destination city name (ADR-064a). Populated
	// from profile data when available. Empty for guests or when not set.
	DestCity string `json:"dest_city"`

	// CurrentGeoSpaceID is the UUID of the user's current geo Space
	// (ADR-064a). Used for city-level filtering (community locator, local
	// events). Empty for guests or users who haven't set it.
	CurrentGeoSpaceID string `json:"current_geo_space_id"`
}

// FromContext extracts the VisitorContext from the request context.
// Returns a zero-value context with ContentCountry="NG" when none is set.
func FromContext(ctx context.Context) VisitorContext {
	vc, ok := ctx.Value(ContextKey{}).(VisitorContext)
	if !ok {
		return VisitorContext{
			ContentCountry:    "NG",
			ContentRegion:     "Africa",
			Source:            "default",
			RoadmapSlugs:      []string{},
			WeakestDimensions: []string{},
		}
	}

	return vc
}

// cacheTTL is the Valkey cache TTL for resolved visitor contexts (ADR-064).
const cacheTTL = 30 * time.Minute

// Default market lists for archetype resolution when the settings service is
// unavailable (e.g. tests). The canonical values are seeded in
// system_settings via migrations 000182 and 000382.
const (
	defaultOriginMarkets      = "NG,GH,ZA,KE,EG"
	defaultDestinationMarkets = "GB,CA,US,DE,AU,AE"
)

// Resolver is the VisitorContext resolver middleware. It depends on:
//   - a read-only sqlc Queries for user preference + custom-field lookups
//   - a Redis client for caching
//   - a settings reader for the geo_personalization.enabled killswitch and
//     the config-driven origin/destination market lists
//
// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability (the
// geo_personalization.enabled killswitch). *features.Checker satisfies it.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

type Resolver struct {
	read     *gen.Queries
	rdb      valkeycompat.Cmdable
	settings *config.Settings
	checker  FeatureChecker
}

// NewResolver creates a VisitorContext resolver.
func NewResolver(read *gen.Queries, rdb valkeycompat.Cmdable, settings *config.Settings) *Resolver {
	return &Resolver{read: read, rdb: rdb, settings: settings}
}

// SetFeatureChecker injects the feature flag checker so the
// geo_personalization.enabled killswitch reads feature_flags instead of
// system_settings. Falls back to system_settings when nil.
func (res *Resolver) SetFeatureChecker(c FeatureChecker) { res.checker = c }

// geoPersonalizationEnabled resolves the geo_personalization.enabled feature
// flag; system_settings is consulted only as a fallback when no checker is
// wired.
func (res *Resolver) geoPersonalizationEnabled(ctx context.Context) bool {
	if res.checker != nil {
		return res.checker.IsEnabled(ctx, "geo_personalization.enabled")
	}

	if res.settings == nil {
		return true
	}

	return res.settings.GetBoolOr(ctx, "geo_personalization.enabled", true)
}

// Middleware returns an HTTP middleware that resolves the VisitorContext for
// every request and adds it to the request context. It must be wired AFTER
// the GeoIP middleware and AFTER the auth middleware.
func (res *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vc := res.resolve(r)
		res.enrichRegion(r.Context(), &vc)

		ctx := context.WithValue(r.Context(), ContextKey{}, vc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolve determines the VisitorContext for a single request.
func (res *Resolver) resolve(r *http.Request) VisitorContext {
	geoCtx := geoip.FromContext(r.Context())
	user := auth.FromContext(r.Context())

	vc := VisitorContext{
		GeoIPCountry:      geoCtx.CountryCode,
		CurrentCountry:    geoCtx.CountryCode, // default: GeoIP; enriched from profile below
		CurrentCity:       geoCtx.City,        // ADR-064a: GeoIP city as initial value
		Language:          res.clampLanguage(r.Context(), resolveLanguage(r)),
		RoadmapSlugs:      []string{},
		WeakestDimensions: []string{},
	}
	if user != nil {
		vc.IsAuthenticated = true
		vc.UserID = user.UserID.String()
	}

	// Killswitch (ADR-064 §6): when geo_personalization.enabled is false,
	// "all surfaces fall back to global ranking (current behavior)." The
	// ?geo= override is still honored (it's an explicit user action, not
	// personalization), but preference/archetype/GeoIP sources are skipped.
	// ContentCountry is left empty (not "NG") so downstream consumers
	// (trending, recommendations, search) treat it as global — empty/NULL
	// in the trending SQL means "no geo filter", while "NG" would filter
	// to Nigeria only (BUG-VIS-001).
	if !res.geoPersonalizationEnabled(r.Context()) {
		if q := normalizeCountry(r.URL.Query().Get("geo")); q != "" {
			vc.ContentCountry = q
			vc.Source = "query"
		} else {
			vc.ContentCountry = ""
			vc.Source = "default"
		}

		return vc
	}

	// 1. Explicit ?geo= query parameter — always wins when present.
	if q := normalizeCountry(r.URL.Query().Get("geo")); q != "" {
		vc.ContentCountry = q
		vc.Source = "query"

		return vc
	}

	// 1b. Guest journey cookie / query parameters (journey-aware
	// recommendations). Allows anonymous users to set origin, destination,
	// stage, and visa route without logging in. Auth/profile resolution below
	// may override these values for authenticated users.
	res.enrichGuestJourney(r, &vc)

	// Determine cache key and auth state.
	// ADR-064: cache key is jc:visitor:ctx:{session_id} for authenticated
	// users (using the JWT JTI as session ID) and jc:visitor:ctx:ip:{ip}
	// for anonymous visitors.
	var cacheKey string

	if user != nil {
		if user.SessionID != "" {
			cacheKey = valkey.MustKey("visitor:ctx:%s", user.SessionID)
		} else {
			// Fallback for tokens without JTI (legacy/compat).
			cacheKey = valkey.MustKey("visitor:ctx:user:%s", user.UserID.String())
		}
	} else {
		cacheKey = valkey.MustKey("visitor:ctx:ip:%s", clientip.FromRequest(r))
	}

	// Fold the geo config fingerprint into the cache key so market-list and
	// related setting changes self-invalidate cached contexts instead of
	// waiting out the 30-minute TTL.
	if cacheKey != "" {
		cacheKey += ":" + res.geoConfigFingerprint(r.Context())
	}

	// Try cache first. cacheKey is empty when the anonymous visitor has no
	// platform context (BUG-V001: we skip caching rather than risk cross-platform
	// bleed on an unscoped key).
	if res.rdb != nil && cacheKey != "" {
		if cached, err := res.rdb.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			if cv, ok := res.unmarshalCache(r.Context(), cached); ok {
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
	if user != nil {
		if pref, err := res.read.GetUserContentCountryPreference(r.Context(),

			user.UserID,
		); err == nil && pref != nil && *pref != "" {
			vc.ContentCountry = strings.ToUpper(*pref)
			vc.Source = "preference"
			// Still resolve archetype for analytics/surfacing, using profile
			// fields + GeoIP, but the preference wins for ContentCountry.
			res.enrichArchetype(r.Context(), &vc, user, "")
			res.cache(r.Context(), cacheKey, vc)

			return vc
		}
	}

	// 3. Archetype resolution — synthesizes ContentCountry from the visitor's
	//    signals (ADR-064). For authenticated users this loads japa_status +
	//    destination_country custom fields. For guests it classifies from
	//    GeoIP alone.
	res.enrichArchetype(r.Context(), &vc, user, cacheKey)

	return vc
}

// ResolveForUser builds a VisitorContext for an authenticated user without an
// HTTP request — used by request-less consumers like the §122 sweep job.
// It runs the same member path as resolve(): geo killswitch, the user's
// content_country_preference override, then profile/geo-space/assessment
// enrichment via enrichArchetype. Request-scoped signals that don't exist
// outside HTTP (GeoIP, ?geo= override, journey cookie, language) resolve to
// their neutral defaults — profile fields are the source of truth here.
func (res *Resolver) ResolveForUser(ctx context.Context, userID pgtype.UUID) VisitorContext {
	vc := VisitorContext{
		IsAuthenticated:   true,
		UserID:            userID.String(),
		RoadmapSlugs:      []string{},
		WeakestDimensions: []string{},
	}
	if res == nil || !userID.Valid {
		return vc
	}

	if !res.geoPersonalizationEnabled(ctx) {
		vc.Source = "default"
		return vc
	}

	user := &auth.AuthenticatedUser{UserID: userID}

	// User's explicit content-country preference wins when set (mirrors
	// resolve() step 2).
	if pref, err := res.read.GetUserContentCountryPreference(ctx, userID); err == nil && pref != nil && *pref != "" {
		vc.ContentCountry = strings.ToUpper(*pref)
		vc.Source = "preference"
		res.enrichArchetype(ctx, &vc, user, "")

		return vc
	}

	res.enrichArchetype(ctx, &vc, user, "")

	return vc
}

// enrichArchetype loads the user's geo profile fields (when authenticated),
// resolves the archetype, and sets ContentCountry from the archetype when no
// explicit override is present. When cacheKey is non-empty the resolved
// context is cached.
func (res *Resolver) enrichArchetype(ctx context.Context, vc *VisitorContext, user *auth.AuthenticatedUser, cacheKey string) {
	// Load config-driven market lists (ADR-064: config, not hardcoded).
	originMarkets := res.marketSet(ctx, "geo_personalization.origin_markets", defaultOriginMarkets)
	destMarkets := res.marketSet(ctx, "geo_personalization.destination_markets", defaultDestinationMarkets)

	// Load profile signals for authenticated users.
	if user != nil {
		fields, err := res.read.GetUserGeoProfileFields(ctx,
			user.UserID,
		)
		if err == nil {
			vc.JapaStage = strings.ToLower(strings.TrimSpace(fields.JapaStatus))
			vc.JourneyStage = normalizeJourneyStage(vc.JapaStage)
			vc.VisaRoute = normalizeVisaRoute(fields.VisaRoute)
			vc.Purpose = normalizePurpose(fields.Purpose)
			vc.JapaIntent = NormalizeIntent(fields.JapaIntent)
			vc.TravelDate = strings.TrimSpace(fields.PlannedTravelDate)
			vc.DestinationCountry = normalizeCountry(fields.DestinationCountry)
			// ADR-064a: Use origin_country from profile when set; falls back
			// to GeoIP country (already set in vc.OriginCountry).
			if oc := normalizeCountry(fields.OriginCountry); oc != "" {
				vc.OriginCountry = oc
			}
		}

		// Journey-aware recommendations: load the latest Japa Readiness
		// Assessment (§71) and overlay its signals on top of profile fields.
		res.enrichFromAssessment(ctx, vc, user.UserID)
		// ADR-064a: Resolve the user's current_geo_space_id to extract
		// city-level data and override CurrentCountry from the Space hierarchy
		// (more reliable than GeoIP for VPN/travel scenarios). Gated on the
		// geo_personalization.city_level_enabled killswitch (ADR-064a §6) —
		// when false, city-level enrichment is skipped entirely.
		if res.getBoolSetting(ctx, "geo_personalization.city_level_enabled", true) {
			geoSpace, err := res.read.GetUserCurrentGeoSpace(ctx,
				user.UserID,
			)
			if err == nil && geoSpace.CurrentGeoSpaceID.Valid {
				vc.CurrentGeoSpaceID = geoSpace.CurrentGeoSpaceID.String()
				// Override CurrentCountry from the Space hierarchy when available.
				if geoSpace.CountryGeoCode != nil && *geoSpace.CountryGeoCode != "" {
					vc.CurrentCountry = strings.ToUpper(*geoSpace.CountryGeoCode)
				}
				// Set CurrentCity from the Space name when it's a city-level
				// space (BUG-VIS-002: the SQL query only returns city_name for
				// city-level spaces; state/country names are NOT treated as cities).
				if geoSpace.CityName != "" {
					vc.CurrentCity = geoSpace.CityName
					// Best-effort origin/destination city: if the user is currently
					// in a city that matches their origin or destination country,
					// mirror the city name. True origin/destination city fields are
					// not yet seeded as custom fields, so this is a partial fill.
					if geoSpace.CountryGeoCode != nil && *geoSpace.CountryGeoCode != "" {
						country := strings.ToUpper(*geoSpace.CountryGeoCode)
						if country == vc.OriginCountry {
							vc.OriginCity = geoSpace.CityName
						}

						if country == vc.DestinationCountry {
							vc.DestCity = geoSpace.CityName
						}
					}
				}
			}
		}
	}

	// ADR-064a: OriginCountry falls back to the GeoIP country when no
	// declared source supplied one (profile field, readiness assessment, or
	// guest journey context — applied in that precedence order above/before).
	if vc.OriginCountry == "" {
		vc.OriginCountry = vc.GeoIPCountry
	}

	// ADR-128: reconcile the two journey vocabularies so consumers key on a
	// single value. The declared japa_intent column wins; an empty intent
	// derives from Purpose (vacation/tourism → visit); an empty Purpose
	// derives from intent (visit → tourism). Covers guests too — their
	// journey_purpose cookie/param is applied before this runs.
	if vc.JapaIntent == "" {
		vc.JapaIntent = NormalizeIntent(vc.Purpose)
	}

	if vc.Purpose == "" {
		vc.Purpose = PurposeForIntent(vc.JapaIntent)
	}

	// Resolve archetype (ADR-064). CurrentCountry is the primary signal
	// (now from Space hierarchy when set, else GeoIP); japa_status and
	// destination_country refine it. OriginCountry (profile field, guest
	// journey context, or GeoIP fallback) is also a signal: a member whose
	// home country is an origin market stays an origin_visitor even when
	// their current location is not an origin market.
	archetype, contentCountry, confidence := resolveArchetype(
		vc.CurrentCountry, vc.JapaStage, vc.DestinationCountry, vc.OriginCountry,
		originMarkets, destMarkets,
	)
	vc.Archetype = archetype
	vc.Confidence = confidence
	// Preserve explicit user overrides (?geo=, content_country_preference).
	// Archetype classification still runs for the signals it carries, but
	// the resolved ContentCountry must not overwrite an explicit choice.
	if vc.Source == "" {
		vc.ContentCountry = contentCountry
	}

	// If archetype resolution produced no ContentCountry (no GeoIP), fall back
	// to the configured system default rather than serving an empty country
	// downstream.
	if vc.ContentCountry == "" {
		vc.ContentCountry = res.defaultCountry(ctx)
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
//   - origin_visitor  — current country is an origin market and not settled;
//     OR the declared origin (profile field, guest journey, or GeoIP) is an
//     origin market while the visitor is neither in a destination market nor
//     settled — e.g. a Nigerian in transit through a non-market country.
//     ContentCountry = destination if set, else the origin market country.
//   - diaspora       — current country is a destination market and/or
//     japa_status == "settled". ContentCountry = current country.
//   - global_traveler — neither pattern. ContentCountry = current, low weight.
func resolveArchetype(current, stage, destination, origin string, originMarkets, destMarkets map[string]bool) (archetype, contentCountry string, confidence float32) {
	isOrigin := current != "" && originMarkets[current]
	isDest := current != "" && destMarkets[current]
	settled := stage == "settled"
	fromOriginMarket := origin != "" && originMarkets[origin]

	switch {
	case isOrigin && !settled:
		archetype = ArchetypeOriginVisitor
		confidence = 0.8

		if destination != "" {
			contentCountry = destination
		} else if fromOriginMarket {
			contentCountry = origin
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
	case fromOriginMarket:
		archetype = ArchetypeOriginVisitor
		confidence = 0.85

		if destination != "" {
			contentCountry = destination
		} else {
			contentCountry = origin
		}
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
func (res *Resolver) marketSet(ctx context.Context, key, fallback string) map[string]bool {
	out := make(map[string]bool)

	raw := fallback
	if res.settings != nil {
		raw = res.settings.GetStringOr(ctx, key, fallback)
	}

	for _, c := range strings.Split(raw, ",") {
		if c = normalizeCountry(c); c != "" {
			out[c] = true
		}
	}

	return out
}

// getBoolSetting reads a bool system_setting with a fallback. Returns the
// fallback when the resolver has no settings reader (e.g. tests).
func (res *Resolver) getBoolSetting(ctx context.Context, key string, fallback bool) bool {
	if res.settings == nil {
		return fallback
	}

	return res.settings.GetBoolOr(ctx, key, fallback)
}

// defaultCountry returns the configured fallback country code for visitors
// with no resolvable geo signal. Falls back to "NG" when no setting.
func (res *Resolver) defaultCountry(ctx context.Context) string {
	fallback := "NG"
	if res.settings != nil {
		return res.settings.GetStringOr(ctx, "geo_personalization.default_country", fallback)
	}

	return fallback
}

// geoConfigFingerprint returns a short hash of the geo-personalization
// settings that affect resolution (archetype market lists, city-level
// killswitch, default country). It is appended to the visitor-context cache
// key so a config change yields a different key — stale entries simply
// expire unread. Reads are served by the settings L1 cache, so this costs
// a few map lookups per request.
func (res *Resolver) geoConfigFingerprint(ctx context.Context) string {
	if res.settings == nil {
		return "nocfg"
	}

	h := fnv.New32a()

	for _, key := range []string{
		"geo_personalization.origin_markets",
		"geo_personalization.destination_markets",
		"geo_personalization.city_level_enabled",
		"geo_personalization.default_country",
	} {
		v, err := res.settings.GetSetting(ctx, key)
		if err != nil {
			v = "?"
		}

		_, _ = h.Write([]byte(v))
	}

	return strconv.FormatUint(uint64(h.Sum32()), 36)
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
func (res *Resolver) unmarshalCache(ctx context.Context, s string) (VisitorContext, bool) {
	if len(s) == 0 || s[0] != '{' {
		return VisitorContext{}, false
	}

	var vc VisitorContext
	if err := json.Unmarshal([]byte(s), &vc); err != nil {
		return VisitorContext{}, false
	}

	if vc.ContentCountry == "" {
		vc.ContentCountry = res.defaultCountry(ctx)
		vc.Source = "default"
	}

	if vc.RoadmapSlugs == nil {
		vc.RoadmapSlugs = []string{}
	}

	if vc.WeakestDimensions == nil {
		vc.WeakestDimensions = []string{}
	}

	return vc, true
}

// NormalizeCountry trims, uppercases, and validates a 2-letter ISO code.
func NormalizeCountry(c string) string {
	return normalizeCountry(c)
}

// normalizeCountry trims, uppercases, and validates a 2-letter ISO code.
func normalizeCountry(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if len(c) != 2 || c[0] < 'A' || c[0] > 'Z' || c[1] < 'A' || c[1] > 'Z' {
		return ""
	}

	return c
}

// enrichRegion populates ContentRegion from the resolved ContentCountry by
// looking up the matching primary country space. Falls back to empty.
func (res *Resolver) enrichRegion(ctx context.Context, vc *VisitorContext) {
	if vc.ContentCountry == "" {
		return
	}

	code := strings.ToUpper(vc.ContentCountry)

	row, err := res.read.GetSpaceRegionByGeoCode(ctx,
		&code,
	)
	if err != nil || row == nil || *row == "" {
		return
	}

	vc.ContentRegion = *row
}

// storedJourneyContext is the JSON shape persisted in Valkey for guest
// journey cookies (jc:visitor:journey:{token}). It intentionally mirrors the
// user-provided fields from POST /api/v1/feed/context.
type storedJourneyContext struct {
	OriginCountry      string `json:"origin_country"`
	DestinationCountry string `json:"destination_country"`
	JapaStage          string `json:"japa_stage"`
	JapaIntent         string `json:"japa_intent"`
	VisaRoute          string `json:"visa_route"`
	Purpose            string `json:"purpose"`
	TravelDate         string `json:"travel_date"`
}

// enrichGuestJourney reads the jc_journey_token cookie and/or ?journey_*
// query parameters and overlays the values onto the VisitorContext for
// anonymous users. Authenticated users are not skipped here — profile fields
// are preferred in enrichArchetype, but a cookie/query can still seed defaults
// when a profile field is empty.
func (res *Resolver) enrichGuestJourney(r *http.Request, vc *VisitorContext) {
	var stored storedJourneyContext

	loaded := false

	// 1. Try the cookie-backed Valkey store.
	if res.rdb != nil {
		if token, err := r.Cookie("jc_journey_token"); err == nil && token.Value != "" {
			key := valkey.MustKey("visitor:journey:%s", token.Value)
			if raw, err := res.rdb.Get(r.Context(), key).Result(); err == nil && raw != "" {
				if err := json.Unmarshal([]byte(raw), &stored); err == nil {
					loaded = true
				}
			}
		}
	}

	// 2. Query parameters override or supplement the cookie.
	q := r.URL.Query()
	if v := q.Get("journey_origin"); v != "" {
		stored.OriginCountry = v
		loaded = true
	}

	if v := q.Get("journey_destination"); v != "" {
		stored.DestinationCountry = v
		loaded = true
	}

	if v := q.Get("journey_stage"); v != "" {
		stored.JapaStage = v
		loaded = true
	}

	if v := q.Get("journey_intent"); v != "" {
		stored.JapaIntent = v
		loaded = true
	}

	if v := q.Get("journey_visa"); v != "" {
		stored.VisaRoute = v
		loaded = true
	}

	if v := q.Get("journey_purpose"); v != "" {
		stored.Purpose = v
		loaded = true
	}

	if v := q.Get("journey_travel_date"); v != "" {
		v = strings.TrimSpace(v)
		if _, err := time.Parse("2006-01-02", v); err == nil {
			stored.TravelDate = v
			loaded = true
		}
	}

	if !loaded {
		return
	}

	if stored.OriginCountry != "" {
		vc.OriginCountry = normalizeCountry(stored.OriginCountry)
	}

	if stored.DestinationCountry != "" {
		vc.DestinationCountry = normalizeCountry(stored.DestinationCountry)
	}

	if stored.JapaStage != "" {
		vc.JapaStage = strings.ToLower(strings.TrimSpace(stored.JapaStage))
		vc.JourneyStage = normalizeJourneyStage(vc.JapaStage)
	}

	if stored.JapaIntent != "" {
		vc.JapaIntent = NormalizeIntent(stored.JapaIntent)
	}

	if stored.VisaRoute != "" {
		vc.VisaRoute = normalizeVisaRoute(stored.VisaRoute)
	}

	if stored.Purpose != "" {
		vc.Purpose = normalizePurpose(stored.Purpose)
	}

	if stored.TravelDate != "" {
		v := strings.TrimSpace(stored.TravelDate)
		if _, err := time.Parse("2006-01-02", v); err == nil {
			vc.TravelDate = v
		}
	}
}

// enrichFromAssessment loads the latest Japa Readiness Assessment result for
// an authenticated user and overlays origin/destination/visa route, roadmap
// slugs, and weakest dimensions onto the VisitorContext.
func (res *Resolver) enrichFromAssessment(ctx context.Context, vc *VisitorContext, userID pgtype.UUID) {
	if res.read == nil {
		return
	}

	row, err := res.read.GetLatestAssessmentJourneyContext(ctx,

		userID,
	)
	if err != nil {
		return
	}

	// Threshold label is surfaced as-is for UI copy.
	vc.AssessmentThreshold = strings.TrimSpace(row.ThresholdLabel)

	// Roadmap content is an array of RoadmapStep with ContentSlug fields.
	vc.RoadmapSlugs = extractRoadmapSlugs(row.RoadmapContent)

	// Dimension scores are {dimension: score}. Weak dimensions inform which
	// journey-stage content to boost.
	vc.WeakestDimensions = extractWeakestDimensions(row.DimensionScores)

	// Submission field_values may contain origin, destination, visa_type, etc.
	// Overlay them only when the profile has not already set a value.
	fields := parseAssessmentSubmission(row.SubmissionValues)
	fields.VisaType = normalizeVisaRoute(fields.VisaType)
	fields.JapaStage = strings.ToLower(strings.TrimSpace(fields.JapaStage))
	fields.Purpose = normalizePurpose(fields.Purpose)

	if fields.Origin != "" && vc.OriginCountry == "" {
		vc.OriginCountry = normalizeCountry(fields.Origin)
	}

	if fields.Destination != "" && vc.DestinationCountry == "" {
		vc.DestinationCountry = normalizeCountry(fields.Destination)
	}

	if fields.VisaType != "" && vc.VisaRoute == "" {
		vc.VisaRoute = normalizeVisaRoute(fields.VisaType)
	}

	if fields.TravelDate != "" && vc.TravelDate == "" {
		vc.TravelDate = fields.TravelDate
	}

	if fields.Purpose != "" && vc.Purpose == "" {
		vc.Purpose = fields.Purpose
	}

	if fields.JapaStage != "" {
		vc.JapaStage = strings.ToLower(strings.TrimSpace(fields.JapaStage))
		vc.JourneyStage = normalizeJourneyStage(vc.JapaStage)
	}
}

type assessmentSubmissionFields struct {
	Origin      string `json:"origin_country"`
	Destination string `json:"destination_country"`
	VisaType    string `json:"visa_type"`
	JapaStage   string `json:"japa_status"`
	Purpose     string `json:"purpose"`
	TravelDate  string `json:"travel_date"`
}

// parseAssessmentSubmission extracts known journey fields from a form
// submission's field_values JSONB. It ignores unknown keys and returns empty
// values for missing or invalid fields.
func parseAssessmentSubmission(b []byte) assessmentSubmissionFields {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil || raw == nil {
		return assessmentSubmissionFields{}
	}

	// The assessment form stores field values under the custom field key,
	// but some forms may use nested objects or arrays. Try a few common
	// patterns and fall back to the raw string value.
	getStr := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				switch val := v.(type) {
				case string:
					return val
				case map[string]any:
					if s, ok := val["value"].(string); ok {
						return s
					}
					// Some clients send {id: "...", label: "..."} for selects.
					if s, ok := val["id"].(string); ok {
						return s
					}
				case []any:
					// Multi-select: join non-empty values with a comma.
					var parts []string

					for _, el := range val {
						switch e := el.(type) {
						case string:
							if e != "" {
								parts = append(parts, e)
							}
						case map[string]any:
							if s, ok := e["value"].(string); ok && s != "" {
								parts = append(parts, s)
							} else if s, ok := e["id"].(string); ok && s != "" {
								parts = append(parts, s)
							}
						}
					}

					return strings.Join(parts, ",")
				case float64:
					// Numeric values (e.g. year, age).
					return strconv.FormatFloat(val, 'f', -1, 64)
				case int:
					return strconv.Itoa(val)
				}
			}
		}

		return ""
	}

	return assessmentSubmissionFields{
		Origin:      getStr("origin_country"),
		Destination: getStr("destination_country"),
		VisaType:    getStr("visa_type", "visa_route"),
		JapaStage:   getStr("japa_status", "journey_stage"),
		Purpose:     getStr("purpose", "travel_purpose"),
		TravelDate:  getStr("travel_date", "planned_travel_date"),
	}
}

// roadmapStep mirrors the RoadmapStep type in internal/assessment/service.go.
type roadmapStep struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ContentSlug string `json:"content_slug"`
	Dimension   string `json:"dimension"`
	Priority    int    `json:"priority"`
}

// extractRoadmapSlugs returns the content_slug values from a RoadmapStep array.
func extractRoadmapSlugs(b []byte) []string {
	if len(b) == 0 {
		return []string{}
	}

	var steps []roadmapStep
	if err := json.Unmarshal(b, &steps); err != nil {
		return []string{}
	}

	slugs := []string{}

	for _, s := range steps {
		if s.ContentSlug != "" {
			slugs = append(slugs, s.ContentSlug)
		}
	}

	return slugs
}

// extractWeakestDimensions returns the up to three dimensions with the lowest
// scores from an assessment result's dimension_scores JSONB.
func extractWeakestDimensions(b []byte) []string {
	if len(b) == 0 {
		return []string{}
	}

	var scores map[string]int
	if err := json.Unmarshal(b, &scores); err != nil {
		return []string{}
	}

	type dim struct {
		name  string
		score int
	}

	var dims []dim
	for n, s := range scores {
		dims = append(dims, dim{name: n, score: s})
	}

	sort.Slice(dims, func(i, j int) bool { return dims[i].score < dims[j].score })

	out := []string{}
	for i := 0; i < len(dims) && i < 3; i++ {
		out = append(out, dims[i].name)
	}

	return out
}

// NormalizeJourneyStage maps the legacy japa_status values (and any assessment
// form values) to the canonical §87 Japa Stage state machine.
func NormalizeJourneyStage(s string) string {
	return normalizeJourneyStage(s)
}

// normalizeJourneyStage maps the legacy japa_status values (and any assessment
// form values) to the canonical §87 Japa Stage state machine.
func normalizeJourneyStage(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "considering":
		return "considering"
	case "planning":
		return "planning"
	case "preparing", "newly_japa":
		return "preparing"
	case "applied", "applying":
		return "applied"
	case "approved", "visa_approved":
		return "approved"
	case "relocated", "traveling", "travelling", "settling_in":
		return "relocated"
	case "settled":
		return "settled"
	// Trip-class terminal stages (ADR-122): both are canonical values and
	// must pass through — dropping them silently discards the stage signal,
	// and mapping "returned" to "settled" mislabels a returned visitor.
	case "traveled":
		return "traveled"
	case "returned":
		return "returned"
	}

	return ""
}

// NormalizePurpose normalises free-text travel purpose values.
func NormalizePurpose(s string) string {
	return normalizePurpose(s)
}

// normalizePurpose normalises free-text travel purpose values.
func normalizePurpose(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "study":
		return "study"
	case "work", "employment":
		return "work"
	case "business":
		return "business"
	case "relocation", "relocate":
		return "relocation"
	case "vacation", "holiday":
		return "vacation"
	case "tourism", "tourist":
		return "tourism"
	case "family", "family reunion":
		return "family"
	}

	if s != "" {
		return "other"
	}

	return ""
}

// NormalizeVisaRoute normalises free-text visa route values into the canonical
// set used by the content journey metadata.
func NormalizeVisaRoute(s string) string {
	return normalizeVisaRoute(s)
}

// normalizeVisaRoute normalises free-text visa route values into the canonical
// set used by the content journey metadata.
func normalizeVisaRoute(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "student", "study", "student visa":
		return "student"
	case "work", "work visa", "employment":
		return "work"
	case "business", "business visa":
		return "business"
	case "tourist", "tourism", "tourist visa", "visitor":
		return "tourist"
	case "spouse", "partner", "spousal":
		return "spouse"
	case "family", "family reunion", "family visa":
		return "family"
	case "investment", "investor", "investor visa":
		return "investment"
	case "transit", "transit visa":
		return "transit"
	}

	if s != "" {
		return "other"
	}

	return ""
}

// clampLanguage ensures the resolved language is in the enabled list,
// falling back to the default and then "en".
func (res *Resolver) clampLanguage(ctx context.Context, lang string) string {
	if res.settings == nil {
		return lang
	}

	enabledRaw := res.settings.GetStringOr(ctx, "i18n.enabled_languages", "en,fr")
	enabled := make(map[string]bool)

	for _, l := range strings.Split(enabledRaw, ",") {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			enabled[l] = true
		}
	}

	if enabled[lang] {
		return lang
	}

	defaultLang := strings.ToLower(strings.TrimSpace(res.settings.GetStringOr(ctx, "i18n.default_language", "en")))
	if enabled[defaultLang] {
		return defaultLang
	}
	// Final fallback if the configured default is also missing from enabled.
	for l := range enabled {
		return l
	}

	return "en"
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
