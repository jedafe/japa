// Package recommendations implements the unified geo-aware recommendation
// engine (ADR-064a). It combines the VisitorContext (archetype, japa_stage,
// origin/destination/current country) with a config-driven stage_tool_matrix
// to produce ranked tool, feature, content, and space recommendations.
//
// The endpoint GET /api/v1/recommendations is public (OptionalAuth) — it
// works for both guests (using GeoIP-based VisitorContext) and authenticated
// users (using profile-enriched VisitorContext). Results are cached in
// Valkey at jc:recommendations:{session_id} or jc:recommendations:ip:{ip}
// with a 15-minute TTL.
//
// Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md
package recommendations

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// cacheTTL is the Valkey cache TTL for recommendation results.
const cacheTTL = 15 * time.Minute

// settingsReader is the subset of config.Settings the recommendation engine needs.
type settingsReader interface {
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback string) string
}

// Handlers holds dependencies for recommendation HTTP handlers.
type Handlers struct {
	settings settingsReader
	cache    valkeycompat.Cmdable // nil disables caching (tests)
}

// NewHandlers creates Handlers for the recommendation engine.
func NewHandlers(settings *config.Settings, cache valkeycompat.Cmdable) *Handlers {
	return &Handlers{settings: settings, cache: cache}
}

// RecommendationItem is a single recommended tool, feature, or content item.
type RecommendationItem struct {
	Slug   string `json:"slug"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

// RecommendationResponse is the response shape for GET /api/v1/recommendations.
type RecommendationResponse struct {
	Tools    []RecommendationItem `json:"tools"`
	Features []RecommendationItem `json:"features"`
	Spaces   []RecommendationItem `json:"spaces"`
	Context  VisitorContextSummary `json:"context"`
}

// VisitorContextSummary is a projection of the VisitorContext for the response.
type VisitorContextSummary struct {
	Archetype       string `json:"archetype"`
	ContentCountry  string `json:"content_country"`
	OriginCountry   string `json:"origin_country"`
	CurrentCountry  string `json:"current_country"`
	DestCountry     string `json:"destination_country"`
	JapaStage       string `json:"japa_stage"`
	CurrentCity     string `json:"current_city"`
}

// toolLabelMap maps tool/feature slugs to human-readable labels. These are
// fallback values used when the system_settings geo_personalization.tool_labels
// config is not set. The config is the primary source (Rule 3); this map
// ensures the handler degrades gracefully when the setting is missing.
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
	"japa-university":           "Japa University",
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

// toolURLMap maps tool/feature slugs to their frontend URLs. These are
// fallback values used when the system_settings geo_personalization.tool_urls
// config is not set. The config is the primary source (Rule 3).
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
	"japa-university":           "/university",
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

// GetRecommendations handles GET /api/v1/recommendations (ADR-064a).
// Returns ranked tool, feature, and space recommendations based on the
// VisitorContext. Works for both guests (GeoIP-based) and authenticated
// users (profile-enriched). Results are cached in Valkey for 15 minutes.
//
// Query params:
//   limit — max number of tools to return (default 10, max 20)
func (h *Handlers) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": map[string]string{"code": "tenant_unavailable", "message": "No tenant available"},
		})
		return
	}

	vc := visitor.FromContext(r.Context())

	// Parse limit query param (default 10, max 20).
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > 20 {
				n = 20
			}
			limit = n
		}
	}

	// Build cache key (includes limit so different limits don't collide).
	var cacheKey string
	if user := auth.FromContext(r.Context()); user != nil {
		if user.SessionID != "" {
			cacheKey = valkey.MustKey("recommendations:%s:%d", user.SessionID, limit)
		} else {
			cacheKey = valkey.MustKey("recommendations:user:%s:%d", user.UserID.String(), limit)
		}
	} else {
		cacheKey = valkey.MustKey("recommendations:ip:%s:%d", clientip.FromRequest(r), limit)
	}

	// Try cache.
	if h.cache != nil {
		if cached, err := h.cache.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			if cached[0] == '{' {
				var resp RecommendationResponse
				if err := json.Unmarshal([]byte(cached), &resp); err == nil {
					writeJSON(w, http.StatusOK, map[string]any{"data": resp})
					return
				}
			}
		}
	}

	// Load config-driven tool labels and URLs (Rule 3).
	labels := h.loadToolMap(r.Context(), t.ID, "geo_personalization.tool_labels")
	urls := h.loadToolMap(r.Context(), t.ID, "geo_personalization.tool_urls")

	// Resolve tool recommendations from the stage_tool_matrix config.
	toolSlugs := h.resolveToolSlugs(r.Context(), t.ID, vc)
	if len(toolSlugs) > limit {
		toolSlugs = toolSlugs[:limit]
	}
	tools := make([]RecommendationItem, 0, len(toolSlugs))
	for _, slug := range toolSlugs {
		tools = append(tools, RecommendationItem{
			Slug:   slug,
			Label:  toolLabel(slug, labels),
			URL:    toolURL(slug, urls),
			Reason: h.reasonForSlug(slug, vc),
		})
	}

	// Build response.
	resp := RecommendationResponse{
		Tools: tools,
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

	// Cache the response (best-effort).
	if h.cache != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = h.cache.Set(r.Context(), cacheKey, b, cacheTTL).Err()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// loadToolMap loads a JSON slug→string map from system_settings. Returns
// nil when the setting is missing or invalid (callers fall back to the
// compiled-in defaults). Used for tool_labels and tool_urls (Rule 3).
func (h *Handlers) loadToolMap(ctx context.Context, tenantID pgtype.UUID, key string) map[string]string {
	if h.settings == nil {
		return nil
	}
	raw := h.settings.GetStringOr(ctx, tenantID, key, "")
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
func (h *Handlers) resolveToolSlugs(ctx context.Context, tenantID pgtype.UUID, vc visitor.VisitorContext) []string {
	raw := ""
	if h.settings != nil {
		raw = h.settings.GetStringOr(ctx, tenantID, "geo_personalization.stage_tool_matrix", "")
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

	// Try keys in priority order: exact → archetype:* → *:stage → *:*
	keys := []string{
		fmt.Sprintf("%s:%s", vc.Archetype, stage),
		fmt.Sprintf("%s:*", vc.Archetype),
		fmt.Sprintf("*:%s", stage),
		"*:*",
	}

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
func defaultToolSlugs(vc visitor.VisitorContext) []string {
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
		return []string{"destination-wiki", "community-locator", "japa-university"}
	}
}

// reasonForSlug returns a human-readable reason string for why a tool is
// recommended given the VisitorContext.
func (h *Handlers) reasonForSlug(slug string, vc visitor.VisitorContext) string {
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

// toolLabel returns the display label for a tool slug. Checks the config-driven
// map first (Rule 3), then falls back to the compiled-in defaults.
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

// toolURL returns the frontend URL for a tool slug. Checks the config-driven
// map first (Rule 3), then falls back to the compiled-in defaults.
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

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("recommendations: json encode error", "error", err)
	}
}
