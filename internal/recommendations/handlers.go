// Package recommendations implements the unified geo-aware recommendation
// engine (ADR-064a). It combines the VisitorContext (archetype, japa_stage,
// origin/destination/current country) with a config-driven stage_tool_matrix
// to produce ranked tool, feature, content, and space recommendations.
// // The endpoint GET /api/v1/recommendations is public (OptionalAuth) — it
// works for both guests (using GeoIP-based VisitorContext) and authenticated
// users (using profile-enriched VisitorContext). Results are cached in
// Valkey at jc:recommendations:{session_id} or jc:recommendations:ip:{ip}
// with a configurable TTL (default 15 minutes).
// // Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md
package recommendations

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// defaultCacheTTL is the fallback Valkey cache TTL (seconds) for recommendation
// results when geo_personalization.recommendations_cache_ttl_seconds is unset.
const defaultCacheTTL = 15 * time.Minute

// defaultLimit is the fallback for the ?limit query param.
const defaultLimit = 10

// defaultMaxLimit is the fallback cap for the ?limit query param when
// geo_personalization.recommendations_max_limit is unset.
const defaultMaxLimit = 20

// defaultTrendingWindow is the fallback trending-snapshot window used when
// geo_personalization.recommendations_trending_window is unset (Rule 3).
const defaultTrendingWindow = "7d"

// settingsReader is the subset of config.Settings the recommendation engine needs.
type settingsReader interface {
	GetStringOr(ctx context.Context, key, fallback string) string
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// TrendingReader is the subset of the trending store the recommendation
// engine consumes to source content and space recommendations (ADR-064a §5).
// A nil trending reader disables content/spaces population (graceful
// degradation — the response still returns tools and features).
type TrendingReader interface {
	GetTrendingContent(ctx context.Context, window, geoCode string, limit int32) ([]gen.GetTrendingContentRow, error)
	GetTrendingSpaces(ctx context.Context, window, geoCode string, limit int32) ([]gen.GetTrendingSpacesRow, error)
	GetSpaceLevels(ctx context.Context, spaceIDs []pgtype.UUID) (map[[16]byte]string, error)
}

// Handlers holds dependencies for recommendation HTTP handlers.
type Handlers struct {
	settings settingsReader
	cache    valkeycompat.Cmdable // nil disables caching (tests)
	trending TrendingReader       // nil disables content/spaces sourcing (tests)
}

// NewHandlers creates Handlers for the recommendation engine. The trending
// reader may be nil to disable content/spaces sourcing (e.g. in tests); in
// production it should be the *trending.Store instance.
func NewHandlers(settings *config.Settings, cache valkeycompat.Cmdable, trending TrendingReader) *Handlers {
	return &Handlers{settings: settings, cache: cache, trending: trending}
}

// RecommendationItem is a single recommended tool or feature.
type RecommendationItem struct {
	Slug   string `json:"slug"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

// RecommendationContentItem is a single recommended content item sourced from
// the trending content snapshots (ADR-064a §5, Section 53).
type RecommendationContentItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
}

// RecommendationSpaceItem is a single recommended space sourced from the
// trending space snapshots (ADR-064a §5, Section 53).
type RecommendationSpaceItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Level string `json:"level"`
}

// RecommendationResponse is the response shape for GET /api/v1/recommendations.
type RecommendationResponse struct {
	Tools    []RecommendationItem        `json:"tools"`
	Features []RecommendationItem        `json:"features"`
	Content  []RecommendationContentItem `json:"content"`
	Spaces   []RecommendationSpaceItem   `json:"spaces"`
	Context  VisitorContextSummary       `json:"context"`
}

// VisitorContextSummary is a projection of the VisitorContext for the response.
type VisitorContextSummary struct {
	Archetype      string `json:"archetype"`
	ContentCountry string `json:"content_country"`
	OriginCountry  string `json:"origin_country"`
	CurrentCountry string `json:"current_country"`
	DestCountry    string `json:"destination_country"`
	JapaStage      string `json:"japa_stage"`
	CurrentCity    string `json:"current_city"`
}

// #nosec G101 // public fallback display labels; the canonical source is the
// geo_personalization.tool_labels system_settings value (Rule 3). This map only
// provides human-readable labels when the setting is missing or the service is
// not wired.
var toolLabelMap = map[string]string{
	"visa-route-comparison":     "Visa Route Comparison",
	"embassy-alerter":           "Embassy Appointment Alerter",
	"processing-time-tracker":   "Processing Time Tracker",
	"credential-checker":        "Credential Checker",
	"cv-formatter":              "CV Formatter",
	"housing-guide":             "Housing Guide",
	"attestation-guide":         "Attestation Guide",
	"language-test-hub":         "Language Test Hub",
	"tax-calculator":            "Tax Calculator",
	"community-locator":         "Community Locator",
	"destination-wiki":          "Destination Wiki",
	"cost-of-living-comparison": "Cost of Living Comparison",
	"arrival-checklist":         "Arrival Checklist",
	"banking-guide":             "Banking Guide",
	"mutual-aid-board":          "Mutual Aid Board",
	"service-directory":         "Service Directory",
	"japa-academy":              "Japa Academy",
	"story-wall":                "Community Story Wall",
	"scam-alert":                "Scam Alert",
	"planning-groups":           "Planning Groups",
	"visa-timeline":             "Visa Timeline Tracker",
	"community-calendar":        "Community Calendar",
	"japa-stage":                "Japa Stage Profiles",
	"ask-an-expat":              "Ask an Expat AMA",
	"achievement-feed":          "Achievement Feed",
	"mentorship":                "Mentorship Matching",
}

// #nosec G101 // public fallback URLs; the canonical source is the
// geo_personalization.tool_urls system_settings value (Rule 3). This map only
// provides URLs when the setting is missing or the service is not wired.
var toolURLMap = map[string]string{
	"visa-route-comparison":     "/tools/visa-route-comparison",
	"embassy-alerter":           "/tools/embassy-alerter",
	"processing-time-tracker":   "/tools/processing-time-tracker",
	"credential-checker":        "/tools/credential-checker",
	"cv-formatter":              "/tools/cv-formatter",
	"housing-guide":             "/tools/housing-guide",
	"attestation-guide":         "/tools/attestation-guide",
	"language-test-hub":         "/tools/language-test-hub",
	"tax-calculator":            "/tools/tax-calculator",
	"community-locator":         "/tools/community-locator",
	"destination-wiki":          "/wiki",
	"cost-of-living-comparison": "/tools/cost-of-living",
	"arrival-checklist":         "/tools/arrival-checklist",
	"banking-guide":             "/tools/banking-guide",
	"mutual-aid-board":          "/mutual-aid",
	"service-directory":         "/services",
	"japa-academy":              "/university",
	"story-wall":                "/stories",
	"scam-alert":                "/scam-alerts",
	"planning-groups":           "/groups",
	"visa-timeline":             "/tools/visa-timeline",
	"community-calendar":        "/calendar",
	"japa-stage":                "/japa-stage",
	"ask-an-expat":              "/ama",
	"achievement-feed":          "/achievements",
	"mentorship":                "/mentorship",
}

// featureSlugs classifies which matrix slugs are community/platform "features"
// (§§79–89, §98, §102) rather than functional "tools" (§§90–97, §§99–101) or
// platform utilities. The stage_tool_matrix mixes both; ADR-064a §5 requires
// the response to split them into separate `tools` and `features` arrays.
// Slugs not in this set are treated as tools.
var featureSlugs = map[string]bool{
	"story-wall":         true, // §79 Community Story Wall
	"scam-alert":         true, // §81 Scam Alert Board
	"planning-groups":    true, // §82 Japa Planning Groups
	"visa-timeline":      true, // §83 Visa Timeline Tracker (community-sourced)
	"community-calendar": true, // §86 Community Calendar
	"japa-stage":         true, // §87 Japa Stage Profiles
	"ask-an-expat":       true, // §88 Ask-an-Expat AMA
	"achievement-feed":   true, // §89 Achievement Feed
	"mutual-aid-board":   true, // §85 Mutual Aid Board
	"service-directory":  true, // §98 Service Directory
	"destination-wiki":   true, // §80 Destination Wiki
	"mentorship":         true, // §102 Mentorship Matching
	"japa-academy":       true, // §77 Japa Academy (platform feature)
}

// GetRecommendations handles GET /api/v1/recommendations (ADR-064a).
// Returns ranked tool, feature, content, and space recommendations based on
// the VisitorContext. Works for both guests (GeoIP-based) and authenticated
// users (profile-enriched). Results are cached in Valkey for a configurable
// TTL (default 15 minutes).
// // Query params:
// //	limit — max number of tools/features to return (default 10, max configurable).
func (h *Handlers) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	vc := visitor.FromContext(r.Context())

	limit := h.resolveLimit(r.Context(), r.URL.Query().Get("limit"))

	// Build cache key. The key does NOT include the limit — the full result
	// set is cached and sliced to the requested limit after a cache hit. This
	// keeps the key aligned with the Valkey catalog pattern
	// (jc:recommendations:{session_id} / jc:recommendations:ip:{}:{ip})
	// and ensures invalidateVisitorCache (users/handlers.go) can evict the
	// exact key after a profile/stage/location update. On hit, the cached
	// VisitorContextSummary is compared to the current VisitorContext; if the
	// context changed (?geo=, journey cookie, location) the cached response is
	// treated as a miss even when the key matches.
	var cacheKey string

	if user := auth.FromContext(r.Context()); user != nil {
		if user.SessionID != "" {
			cacheKey = valkey.MustKey("recommendations:%s", user.SessionID)
		} else {
			cacheKey = valkey.MustKey("recommendations:user:%s", user.UserID.String())
		}
	} else {
		// IP-scoped cache key for anonymous guests.
		cacheKey = valkey.MustKey("recommendations:ip:%s", clientip.FromRequest(r))
	}

	// Try cache. On hit, slice tools/features to the requested limit and
	// return. The cached payload is the full unbounded result set.
	if h.serveFromCache(r.Context(), cacheKey, limit, w, vc) {
		return
	}

	// Load config-driven tool labels and URLs (Rule 3).
	labels := h.loadToolMap(r.Context(), "geo_personalization.tool_labels")
	urls := h.loadToolMap(r.Context(), "geo_personalization.tool_urls")

	// Resolve tool/feature recommendations from the stage_tool_matrix config.
	toolSlugs := h.resolveToolSlugs(r.Context(), vc)
	toolSlugs, featureSlugsList := classifySlugs(toolSlugs)
	tools := buildItems(toolSlugs, labels, urls, vc, h)
	features := buildItems(featureSlugsList, labels, urls, vc, h)

	// Source content and space recommendations from the trending store
	// (ADR-064a §5, Section 53). Skipped when no trending reader is wired
	// (graceful degradation — empty arrays, never null).
	content := h.buildContentItems(r.Context(), vc, toInt32(limit))
	spaces := h.buildSpaceItems(r.Context(), vc, toInt32(limit))

	// Build response.
	resp := RecommendationResponse{
		Tools:    tools,
		Features: features,
		Content:  content,
		Spaces:   spaces,
		Context: VisitorContextSummary{
			Archetype:      vc.Archetype,
			ContentCountry: vc.ContentCountry,
			OriginCountry:  vc.OriginCountry,
			CurrentCountry: vc.CurrentCountry,
			DestCountry:    vc.DestinationCountry,
			JapaStage:      vc.JapaStage,
			CurrentCity:    vc.CurrentCity,
		},
	}

	// Cache the full response (best-effort). The cached payload is the
	// unbounded set; the limit is applied on read so one cache entry serves
	// every limit value.
	if h.cache != nil {
		if b, err := json.Marshal(resp); err == nil {
			ttl := h.resolveCacheTTL(r.Context())
			_ = h.cache.Set(r.Context(), cacheKey, b, ttl).Err()
		}
	}

	// Apply the limit to the response we return to this caller.
	resp.Tools = sliceItems(resp.Tools, limit)
	resp.Features = sliceItems(resp.Features, limit)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// toInt32 converts an int to int32 without overflow. It clamps to
// math.MaxInt32 / math.MinInt32 so callers can use it for sqlc parameters
// without triggering integer-overflow linters (Rule 15).
func toInt32(v int) int32 {
	if v > math.MaxInt32 {
		v = math.MaxInt32
	}

	if v < math.MinInt32 {
		v = math.MinInt32
	}

	return int32(v) // #nosec G115 // bounded by the explicit clamps above
}

// resolveLimit parses the ?limit query param and clamps it to the configured
// max (geo_personalization.recommendations_max_limit, fallback 20). The max
// is config-driven per Rule 3 (BUG-007 fix).
func (h *Handlers) resolveLimit(ctx context.Context, raw string) int {
	limit := defaultLimit

	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	maxLimit := defaultMaxLimit
	if h.settings != nil {
		maxLimit = h.settings.GetIntOr(ctx, "geo_personalization.recommendations_max_limit", defaultMaxLimit)
	}

	if maxLimit < 1 {
		maxLimit = defaultMaxLimit
	}

	if limit > maxLimit {
		limit = maxLimit
	}

	return limit
}

// resolveCacheTTL returns the configured cache TTL
// (geo_personalization.recommendations_cache_ttl_seconds, fallback 900s/15min).
// Config-driven per Rule 3 (BUG-007 fix).
func (h *Handlers) resolveCacheTTL(ctx context.Context) time.Duration {
	if h.settings == nil {
		return defaultCacheTTL
	}

	secs := h.settings.GetIntOr(ctx, "geo_personalization.recommendations_cache_ttl_seconds", int(defaultCacheTTL/time.Second))
	if secs < 1 {
		return defaultCacheTTL
	}

	return time.Duration(secs) * time.Second
}

// resolveTrendingWindow returns the configured trending-snapshot window
// (geo_personalization.recommendations_trending_window, fallback "7d").
// Config-driven per Rule 3 (BUG-004 fix).
func (h *Handlers) resolveTrendingWindow(ctx context.Context) string {
	if h.settings == nil {
		return defaultTrendingWindow
	}

	w := h.settings.GetStringOr(ctx, "geo_personalization.recommendations_trending_window", defaultTrendingWindow)
	if w == "" {
		return defaultTrendingWindow
	}

	return w
}

// classifySlugs splits a list of matrix slugs into tools and features using
// the featureSlugs set (ADR-064a §5). Order is preserved within each group.
func classifySlugs(slugs []string) (tools, features []string) {
	tools = make([]string, 0, len(slugs))
	features = make([]string, 0, len(slugs))

	for _, s := range slugs {
		if featureSlugs[s] {
			features = append(features, s)
		} else {
			tools = append(tools, s)
		}
	}

	return tools, features
}

// buildItems converts a list of slugs into RecommendationItem values using the
// config-driven (or fallback) labels and URLs and the per-archetype reason.
func buildItems(slugs []string, labels, urls map[string]string, vc visitor.VisitorContext, h *Handlers) []RecommendationItem {
	items := make([]RecommendationItem, 0, len(slugs))
	for _, slug := range slugs {
		items = append(items, RecommendationItem{
			Slug:   slug,
			Label:  toolLabel(slug, labels),
			URL:    toolURL(slug, urls),
			Reason: h.reasonForSlug(slug, vc),
		})
	}

	return items
}

// buildContentItems sources trending content (Section 53) and re-weights it by
// the VisitorContext's ContentCountry (ADR-064a §5). Returns an empty slice
// (never nil) when the trending reader is unavailable or returns no rows.
func (h *Handlers) buildContentItems(ctx context.Context, vc visitor.VisitorContext, limit int32) []RecommendationContentItem {
	if h.trending == nil {
		return []RecommendationContentItem{}
	}

	rows, err := h.trending.GetTrendingContent(ctx, h.resolveTrendingWindow(ctx), vc.ContentCountry, limit)
	if err != nil {
		slog.Warn("recommendations: trending content fetch failed", "error", err)
		return []RecommendationContentItem{}
	}

	items := make([]RecommendationContentItem, 0, len(rows))

	for _, row := range rows {
		if !row.ID_2.Valid || row.Title == "" {
			continue
		}

		items = append(items, RecommendationContentItem{
			ID:          row.ID_2.String(),
			Title:       row.Title,
			URL:         contentURL(row.ContentType, row.Slug),
			ContentType: row.ContentType,
		})
	}

	return items
}

// buildSpaceItems sources trending spaces (Section 53) and re-weights them by
// the VisitorContext's ContentCountry (ADR-064a §5). Returns an empty slice
// (never nil) when the trending reader is unavailable or returns no rows.
// Populates the Level field via GetSpaceLevels (BUG-001 fix — ADR-064a §5
// response shape requires `level`).
func (h *Handlers) buildSpaceItems(ctx context.Context, vc visitor.VisitorContext, limit int32) []RecommendationSpaceItem {
	if h.trending == nil {
		return []RecommendationSpaceItem{}
	}

	window := h.resolveTrendingWindow(ctx)

	rows, err := h.trending.GetTrendingSpaces(ctx, window, vc.ContentCountry, limit)
	if err != nil {
		slog.Warn("recommendations: trending spaces fetch failed", "error", err)
		return []RecommendationSpaceItem{}
	}

	// Collect space IDs for the level lookup (BUG-001).
	spaceIDs := make([]pgtype.UUID, 0, len(rows))

	for _, row := range rows {
		if row.ID_2.Valid {
			spaceIDs = append(spaceIDs, row.ID_2)
		}
	}

	levels := map[[16]byte]string{}

	if len(spaceIDs) > 0 {
		if lv, err := h.trending.GetSpaceLevels(ctx, spaceIDs); err == nil {
			levels = lv
		} else {
			slog.Warn("recommendations: space levels fetch failed", "error", err)
		}
	}

	items := make([]RecommendationSpaceItem, 0, len(rows))

	for _, row := range rows {
		if !row.ID_2.Valid {
			continue
		}

		items = append(items, RecommendationSpaceItem{
			ID:    row.ID_2.String(),
			Name:  row.Name,
			Slug:  row.Slug,
			Level: levels[row.ID_2.Bytes], // BUG-001: populated from GetSpaceLevels
		})
	}

	return items
}

// sliceItems returns v truncated to at most n elements. Used to apply the
// ?limit param to a cached (full) response on read.
func sliceItems[T any](v []T, n int) []T {
	if n >= 0 && len(v) > n {
		return v[:n]
	}

	return v
}

// contextMatches returns true when the cached VisitorContextSummary matches
// the current request's VisitorContext. This prevents stale recommendations
// when ?geo=, journey cookie, or profile/location changes shift the context
// while the cache key (session/IP) stays the same.
func contextMatches(summary VisitorContextSummary, vc visitor.VisitorContext) bool {
	return summary.Archetype == vc.Archetype &&
		summary.ContentCountry == vc.ContentCountry &&
		summary.OriginCountry == vc.OriginCountry &&
		summary.CurrentCountry == vc.CurrentCountry &&
		summary.DestCountry == vc.DestinationCountry &&
		summary.JapaStage == vc.JapaStage &&
		summary.CurrentCity == vc.CurrentCity
}

// serveFromCache attempts to read a cached RecommendationResponse from Valkey
// and write it to w (with tools/features sliced to limit). Returns true when a
// cached response was served, false on miss/error (caller proceeds to compute).
func (h *Handlers) serveFromCache(ctx context.Context, key string, limit int, w http.ResponseWriter, vc visitor.VisitorContext) bool {
	if h.cache == nil {
		return false
	}

	cached, err := h.cache.Get(ctx, key).Result()
	if err != nil || cached == "" || cached[0] != '{' {
		return false
	}

	var resp RecommendationResponse
	if err := json.Unmarshal([]byte(cached), &resp); err != nil {
		return false
	}

	// ADR-064a: cached recommendations must not be served when the
	// VisitorContext changed, even if the cache key (session/IP) is the same.
	if !contextMatches(resp.Context, vc) {
		return false
	}

	resp.Tools = sliceItems(resp.Tools, limit)
	resp.Features = sliceItems(resp.Features, limit)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})

	return true
}

// contentURL builds the frontend URL for a trending content item. Blog posts
// use /blog/{slug}, forum threads use /forum/{slug}, everything else uses
// /content/{slug} (matches apps/main OverviewView.contentUrl).
func contentURL(contentType, slug string) string {
	switch contentType {
	case "blog_post":
		return "/blog/" + slug
	case "forum_thread":
		return "/forum/" + slug
	default:
		return "/content/" + slug
	}
}

// loadToolMap loads a JSON slug→string map from system_settings. Returns
// nil when the setting is missing or invalid (callers fall back to the
// compiled-in defaults). Used for tool_labels and tool_urls (Rule 3).
func (h *Handlers) loadToolMap(ctx context.Context, key string) map[string]string {
	if h.settings == nil {
		return nil
	}

	raw := h.settings.GetStringOr(ctx, key, "")
	if raw == "" {
		return nil
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		slog.Warn("recommendations: invalid tool map JSON", "key", key, "error", err)
		return nil
	}

	return m
}

// resolveToolSlugs reads the stage_tool_matrix system setting and resolves
// the recommended tool slugs for the given VisitorContext. The matrix is a
// JSON object mapping "{archetype}:{japa_stage}" keys to arrays of tool slugs.
// Resolution order: exact match → "{archetype}:*" → "*:{japa_stage}" → "*:*".
func (h *Handlers) resolveToolSlugs(ctx context.Context, vc visitor.VisitorContext) []string {
	raw := ""
	if h.settings != nil {
		raw = h.settings.GetStringOr(ctx, "geo_personalization.stage_tool_matrix", "")
	}

	if raw == "" {
		return defaultToolSlugs(vc)
	}

	// Parse the JSON matrix.
	var matrix map[string][]string
	if err := json.Unmarshal([]byte(raw), &matrix); err != nil {
		slog.Warn("recommendations: invalid stage_tool_matrix JSON", "error", err)
		return defaultToolSlugs(vc)
	}

	stage := vc.JapaStage
	if stage == "" {
		stage = "unknown"
	}

	// Try keys in priority order (ADR-128): intent-scoped keys resolve first
	// — the japa_intent journey class is the canonical personalization axis —
	// then archetype keys, then stage-only and full wildcards. Seed data
	// predating ADR-128 uses archetype keys, so both keyspaces are honored.
	keys := []string{}
	if vc.JapaIntent != "" {
		keys = append(keys, vc.JapaIntent+":"+stage, vc.JapaIntent+":*")
	}

	if vc.Archetype != "" {
		keys = append(keys, vc.Archetype+":"+stage, vc.Archetype+":*")
	}

	keys = append(keys, "*:"+stage, "*:*")

	seen := make(map[string]bool)

	var result []string

	for _, key := range keys {
		if slugs, ok := matrix[key]; ok {
			for _, s := range slugs {
				if !seen[s] {
					seen[s] = true

					result = append(result, s)
				}
			}
		}
	}

	if len(result) == 0 {
		return defaultToolSlugs(vc)
	}

	return result
}

// defaultToolSlugs returns a sensible default when the config matrix is
// missing or invalid. Mirrors the fallback values in the migration seed.
// Intent takes precedence over archetype (ADR-128).
func defaultToolSlugs(vc visitor.VisitorContext) []string {
	switch vc.JapaIntent {
	case "visit":
		return []string{"destination-wiki", "embassy-alerter", "scam-alert", "community-locator"}
	case "business":
		return []string{"service-directory", "destination-wiki", "tax-calculator", "community-locator"}
	case "study":
		return []string{"credential-checker", "language-test-hub", "japa-academy", "visa-route-comparison"}
	case "work":
		return []string{"cv-formatter", "credential-checker", "visa-route-comparison", "community-locator"}
	}

	switch {
	case vc.Archetype == visitor.ArchetypeOriginVisitor && vc.JapaStage == "planning":
		return []string{"visa-route-comparison", "embassy-alerter", "processing-time-tracker", "credential-checker"}
	case vc.Archetype == visitor.ArchetypeOriginVisitor && (vc.JapaStage == "newly_japa" || vc.JapaStage == "settling_in"):
		return []string{"arrival-checklist", "housing-guide", "banking-guide", "language-test-hub"}
	case vc.Archetype == visitor.ArchetypeDiaspora && vc.JapaStage == "settled":
		return []string{"tax-calculator", "attestation-guide", "community-locator", "mutual-aid-board"}
	case vc.Archetype == visitor.ArchetypeDiaspora:
		return []string{"community-locator", "destination-wiki", "service-directory"}
	default:
		return []string{"destination-wiki", "community-locator", "japa-academy"}
	}
}

// reasonForSlug returns a human-readable reason string for why a tool is
// recommended given the VisitorContext. The reason is tool-specific (BUG-003
// fix): visa/embassy/processing tools are "planning", arrival/housing/banking
// tools are "settling_in", tax/attestation/credential tools are "settled",
// community/social tools are "community", and everything else falls back to
// the archetype-level reason.
func (h *Handlers) reasonForSlug(slug string, vc visitor.VisitorContext) string {
	// Tool-specific reasons keyed on the slug's functional category.
	switch slug {
	case "visa-route-comparison", "embassy-alerter", "processing-time-tracker":
		return "planning"
	case "arrival-checklist", "housing-guide", "banking-guide", "language-test-hub":
		return "settling_in"
	case "tax-calculator", "attestation-guide", "credential-checker", "cv-formatter":
		return "settled"
	case "community-locator", "destination-wiki", "service-directory", "mutual-aid-board",
		"story-wall", "scam-alert", "planning-groups", "visa-timeline",
		"community-calendar", "japa-stage", "ask-an-expat", "achievement-feed",
		"mentorship", "japa-academy":
		return "community"
	case "cost-of-living-comparison":
		return "exploring"
	default:
		// Fallback: archetype-level reason for unknown slugs.
		return h.archetypeReason(vc)
	}
}

// archetypeReason returns the archetype-level reason used as a fallback when
// a slug doesn't map to a specific category.
func (h *Handlers) archetypeReason(vc visitor.VisitorContext) string {
	switch vc.Archetype {
	case visitor.ArchetypeOriginVisitor:
		if vc.JapaStage == "planning" {
			return "planning"
		}

		if vc.JapaStage == "settling_in" || vc.JapaStage == "newly_japa" {
			return "settling_in"
		}

		return "preparing"
	case visitor.ArchetypeDiaspora:
		if vc.JapaStage == "settled" {
			return "settled"
		}

		return "community"
	default:
		return "exploring"
	}
}

// toolLabel returns the display label for a tool slug. The config-driven
// geo_personalization.tool_labels setting is the source of truth (Rule 3); when
// that is missing, the public toolLabelMap fallback is used before humanizing.
func toolLabel(slug string, configLabels map[string]string) string {
	if configLabels != nil {
		if label, ok := configLabels[slug]; ok && label != "" {
			return label
		}
	}

	if label, ok := toolLabelMap[slug]; ok {
		return label
	}

	return strings.ReplaceAll(slug, "-", " ")
}

// toolURL returns the frontend URL for a tool slug. The config-driven
// geo_personalization.tool_urls setting is the source of truth (Rule 3); when
// that is missing, the public toolURLMap fallback is used before /tools/{slug}.
func toolURL(slug string, configURLs map[string]string) string {
	if configURLs != nil {
		if url, ok := configURLs[slug]; ok && url != "" {
			return url
		}
	}

	if url, ok := toolURLMap[slug]; ok {
		return url
	}

	return "/tools/" + slug
}
