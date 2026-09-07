package feed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/tenant"
	vkcache "github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/valkey-io/valkey-go"
)

const defaultPageSize int32 = 20

// exploreCacheTTL is the TTL for jc:explore:{tenant}:{geo}:{page} (Section 53).
const exploreCacheTTL = 15 * time.Minute

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

// customFieldProvider resolves custom field definitions by key. The concrete
// type is *internal/customfields.Service; the interface keeps the feed package
// from importing a large dependency tree.
type customFieldProvider interface {
	GetCustomFieldByKey(ctx context.Context, key string, tenantID pgtype.UUID) (gen.GetCustomFieldByKeyRow, error)
}

// contentFieldResolver resolves {{...}} template literals in content-like
// structs (Title/Excerpt/Body/etc). The concrete type is
// *internal/content.Service; the interface keeps the feed package decoupled
// from the content package.
type contentFieldResolver interface {
	ResolveContentFields(ctx context.Context, v any, rc literals.RenderContext) error
}

// Handlers holds dependencies for feed HTTP handlers.
type Handlers struct {
	service         *Service
	settings        *config.Settings
	cache           valkey.Client
	customFields    customFieldProvider
	contentResolver contentFieldResolver
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// SetSettings injects the settings service for reading feed.card_style.
func (h *Handlers) SetSettings(s *config.Settings) {
	h.settings = s
}

// SetCache injects the Valkey client for explore feed caching (BUG-006).
func (h *Handlers) SetCache(c valkey.Client) {
	h.cache = c
}

// SetCustomFields injects the custom field provider for resolving journey option
// lists from the custom field system (Rule 16).
func (h *Handlers) SetCustomFields(cf customFieldProvider) {
	h.customFields = cf
}

// SetContentResolver injects the content service used to resolve {{...}}
// template literals in feed item fields (§14 global template literals).
func (h *Handlers) SetContentResolver(r contentFieldResolver) {
	h.contentResolver = r
}

// resolveFeedLiterals resolves {{...}} template literals in the returned
// items' Title/Excerpt fields so feed cards render the same resolved text
// as the content pages (Section 14 — global template literals). The content's
// own source/target country fields scope {{Source_*}}/{{Destination_*}}.
// Best-effort: failures are logged, never fatal. Called after the explore
// cache read so request-scoped literals ({{Current_Year}}, geo) stay fresh.
func (h *Handlers) resolveFeedLiterals(r *http.Request, tenantID pgtype.UUID, items []FeedItem) {
	if h.contentResolver == nil || len(items) == 0 {
		return
	}

	rc := literals.NewRequestContext(r, tenantID, "")

	if err := h.contentResolver.ResolveContentFields(r.Context(), items, rc); err != nil {
		slog.Warn("feed: failed to resolve item literals", "error", err, "tenant_id", tenantID.String())
	}
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// optionalUser resolves the authenticated user from the request context, if present.
func (h *Handlers) optionalUser(r *http.Request) *auth.AuthenticatedUser {
	return auth.FromContext(r.Context())
}

// parsePageSize extracts and clamps the page_size query parameter. When the
// query parameter is absent, the default comes from the system_settings key
// `content.posts_per_page` (admin-configurable in the admin/control Settings
// page), falling back to the compiled default. This makes the admin
// "Posts Per Page" setting actually affect feed pagination across the site.
// Ref: conventions/system-settings-catalog.md (content.posts_per_page).
func (h *Handlers) parsePageSize(r *http.Request) int32 {
	defaultSize := int32(defaultPageSize)

	if h.settings != nil {
		if t := tenant.FromContext(r.Context()); t != nil {
			if v := h.settings.GetIntOr(r.Context(), t.ID, "content.posts_per_page", int(defaultPageSize)); v > 0 {
				if v > int(MaxPageSize) {
					v = int(MaxPageSize)
				}

				defaultSize = toInt32(v)
			}
		}
	}

	s := r.URL.Query().Get("page_size")
	if s == "" {
		if defaultSize > MaxPageSize {
			return MaxPageSize
		}

		return defaultSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return toInt32(n)
}

// parsePageOffset extracts the page_offset query parameter.
func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return toInt32(n)
}

// parseSort extracts and validates the sort query parameter.
func parseSort(r *http.Request) string {
	s := r.URL.Query().Get("sort")
	switch s {
	case SortTop, SortRising, SortRelevant:
		return s
	default:
		return SortLatest
	}
}

// ---------------------------------------------------------------------------
// Feed Handlers
// ---------------------------------------------------------------------------

// GetExploreFeed handles GET /api/v1/feed/explore — public, no auth required.
// BUG-006: caches results in Valkey (jc:explore:{tenant}:{geo}:{page}, Section 53, TTL 15min).
// Uses distributed lock stampede protection (BUG-008).
func (h *Handlers) GetExploreFeed(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sort := parseSort(r)

	// Resolve geo code for cache key (ADR-064 VisitorContext).
	geoCode := "global"
	geoRegion := ""

	if vc := visitor.FromContext(r.Context()); vc.ContentCountry != "" {
		geoCode = vc.ContentCountry
		geoRegion = vc.ContentRegion
	}

	page := int(offset) / int(limit)
	cacheKey := vkcache.MustKey("explore:%s:%s:%s:%s:%d", tenantID.String(), geoCode, geoRegion, sort, page)

	vc := visitor.FromContext(r.Context())

	items, err := vkcache.GetOrComputeNative(r.Context(), h.cache, cacheKey, exploreCacheTTL, func(ctx context.Context) ([]FeedItem, error) {
		return h.service.GetFeed(ctx, tenantID, "explore", nil, nil, nil, sort, limit, offset, vc.ContentCountry, vc.ContentRegion)
	})
	if err != nil {
		slog.Error("feed: get explore feed failed", "sort", sort, "geo", geoCode, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get explore feed")

		return
	}

	h.resolveFeedLiterals(r, tenantID, items)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetPrimaryFeed handles GET /api/v1/feed/primary — personalized feed.
func (h *Handlers) GetPrimaryFeed(w http.ResponseWriter, r *http.Request) {
	user := h.optionalUser(r)
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sort := parseSort(r)

	items, err := h.service.GetFeed(r.Context(), tenantID, "primary", &user.UserID, nil, nil, sort, limit, offset, "", "")
	if err != nil {
		slog.Error("feed: get primary feed failed", "user_id", user.UserID, "sort", sort, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get primary feed")

		return
	}

	h.resolveFeedLiterals(r, tenantID, items)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetForYouFeed handles GET /api/v1/feed/for-you — personalized ranked feed.
func (h *Handlers) GetForYouFeed(w http.ResponseWriter, r *http.Request) {
	user := h.optionalUser(r)
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sortMode := parseSort(r)

	vc := visitor.FromContext(r.Context())

	items, err := h.service.GetForYouFeed(r.Context(), tenantID, user.UserID, sortMode, limit, offset, &vc)
	if err != nil {
		slog.Error("feed: get for-you feed failed", "user_id", user.UserID, "sort", sortMode, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get For-You feed")

		return
	}

	h.resolveFeedLiterals(r, tenantID, items)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetRoadmapFeed handles GET /api/v1/feed/roadmap — the next steps from the
// visitor's latest readiness assessment, returned in roadmap order.
// Requires authentication.
//
// The result is re-ranked by the visitor's journey context (destination,
// journey stage, visa route, source country) when any of those signals are
// present, and falls back to roadmap order otherwise.
func (h *Handlers) GetRoadmapFeed(w http.ResponseWriter, r *http.Request) {
	user := h.optionalUser(r)
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := h.parsePageSize(r)

	vc := visitor.FromContext(r.Context())
	offset := parsePageOffset(r)

	items, err := h.service.GetRoadmapFeedWithContext(r.Context(), tenantID, user.UserID, vc.RoadmapSlugs, limit, offset, &vc)
	if err != nil {
		slog.Error("feed: get roadmap feed failed", "user_id", user.UserID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get roadmap feed")

		return
	}

	h.resolveFeedLiterals(r, tenantID, items)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetSpaceFeed handles GET /api/v1/feed/space/{space_id}.
func (h *Handlers) GetSpaceFeed(w http.ResponseWriter, r *http.Request) {
	user := h.optionalUser(r)
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	spaceIDStr := chi.URLParam(r, "space_id")

	var spaceID pgtype.UUID

	if err := spaceID.Scan(spaceIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space ID format")
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sort := parseSort(r)

	items, err := h.service.GetFeed(r.Context(), tenantID, "space", &user.UserID, &spaceID, nil, sort, limit, offset, "", "")
	if err != nil {
		slog.Error("feed: get space feed failed", "space_id", spaceID, "sort", sort, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get space feed")

		return
	}

	h.resolveFeedLiterals(r, tenantID, items)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetProfileFeed handles GET /api/v1/feed/profile/{user_id}. Public per
// Section 32:12 (profile timeline audience = "Anyone"): the viewer is
// optional — guests see all public rows. When the viewer is authenticated,
// mutual user_blocks are excluded per §41 (BUG-002).
func (h *Handlers) GetProfileFeed(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	authorIDStr := chi.URLParam(r, "user_id")

	var authorID pgtype.UUID

	if err := authorID.Scan(authorIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sort := parseSort(r)

	// viewer is optional — nil for guests, populated for authed users so
	// the §41 mutual user_blocks exclusion applies.
	var viewerID *pgtype.UUID
	if user := h.optionalUser(r); user != nil {
		viewerID = &user.UserID
	}

	items, err := h.service.GetFeed(r.Context(), tenantID, "profile", viewerID, nil, &authorID, sort, limit, offset, "", "")
	if err != nil {
		slog.Error("feed: get profile feed failed", "author_id", authorID, "sort", sort, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get profile feed")

		return
	}

	h.resolveFeedLiterals(r, tenantID, items)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetFeedContext handles GET /api/v1/feed/context.
// Returns the resolved VisitorContext for the current request so the main
// app can render the active origin/destination/visa route/stage and allow
// one-tap editing.
func (h *Handlers) GetFeedContext(w http.ResponseWriter, r *http.Request) {
	vc := visitor.FromContext(r.Context())
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": vc})
}

// feedContextRequest is the body for POST /api/v1/feed/context.
type feedContextRequest struct {
	OriginCountry      string `json:"origin_country"`
	DestinationCountry string `json:"destination_country"`
	JapaStage          string `json:"japa_stage"`
	VisaRoute          string `json:"visa_route"`
	Purpose            string `json:"purpose"`
	TravelDate         string `json:"travel_date"`
}

// guestJourneyTTL is the lifetime of a guest journey context in Valkey.
const guestJourneyTTL = 30 * 24 * time.Hour

// SetFeedContext handles POST /api/v1/feed/context.
// Anonymous users can set their journey context (origin, destination, stage,
// visa, purpose, travel date) without creating an account. The context is
// stored in Valkey and a cookie is returned so the middleware can resolve it
// on subsequent requests.
func (h *Handlers) SetFeedContext(w http.ResponseWriter, r *http.Request) {
	var req feedContextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	// Build the stored context. Empty strings are omitted from Valkey.
	stored := map[string]string{}
	if v := visitor.NormalizeCountry(req.OriginCountry); v != "" {
		stored["origin_country"] = v
	}

	if v := visitor.NormalizeCountry(req.DestinationCountry); v != "" {
		stored["destination_country"] = v
	}

	if v := visitor.NormalizeJourneyStage(req.JapaStage); v != "" {
		stored["japa_stage"] = v
	}

	if v := visitor.NormalizeVisaRoute(req.VisaRoute); v != "" {
		stored["visa_route"] = v
	}

	if v := visitor.NormalizePurpose(req.Purpose); v != "" {
		stored["purpose"] = v
	}

	if v := strings.TrimSpace(req.TravelDate); v != "" {
		if _, err := time.Parse("2006-01-02", v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_travel_date", "travel_date must be YYYY-MM-DD")
			return
		}

		stored["travel_date"] = v
	}

	// Generate a random token for the cookie.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		slog.Error("feed: failed to generate journey token", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to save journey context")

		return
	}

	token := hex.EncodeToString(b)

	// Persist to Valkey and clear the visitor context cache so the next
	// request re-resolves with the new journey signals.
	if h.cache != nil {
		if raw, err := json.Marshal(stored); err == nil {
			key := vkcache.MustKey("visitor:journey:%s", token)
			_ = h.cache.Do(r.Context(), h.cache.B().Set().Key(key).Value(string(raw)).Ex(guestJourneyTTL).Build()).Error()
		}

		h.invalidateVisitorContextCache(r)
	}

	// Set the cookie. HttpOnly keeps the token out of XSS reach; the frontend
	// receives the token in the response body for localStorage sync. Secure is
	// set whenever the request came in over HTTPS.
	// #nosec G124 — Secure is true for HTTPS requests; HTTP is intentionally
	// allowed for local dev while SameSite=Lax + HttpOnly are always set.
	http.SetCookie(w, &http.Cookie{
		Name:     "jc_journey_token",
		Value:    token,
		Path:     "/",
		MaxAge:   int(guestJourneyTTL.Seconds()),
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Secure:   r.TLS != nil,
	})

	// Return the current (pre-middleware) context and the new token. The
	// caller should refresh or call GET /feed/context to see the applied
	// journey values because the middleware has already run.
	vc := visitor.FromContext(r.Context())
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": vc, "token": token})
}

// invalidateVisitorContextCache deletes the VisitorContext and recommendation
// cache entries for the current requestor so that a new or updated journey
// is reflected on the next request.
func (h *Handlers) invalidateVisitorContextCache(r *http.Request) {
	if h.cache == nil {
		return
	}

	ctx := r.Context()
	t := tenant.FromContext(ctx)
	user := auth.FromContext(ctx)

	if user != nil {
		if user.SessionID != "" {
			_ = h.cache.Do(ctx, h.cache.B().Del().Key(vkcache.MustKey("visitor:ctx:%s", user.SessionID)).Build()).Error()
			_ = h.cache.Do(ctx, h.cache.B().Del().Key(vkcache.MustKey("recommendations:%s", user.SessionID)).Build()).Error()
		}

		_ = h.cache.Do(ctx, h.cache.B().Del().Key(vkcache.MustKey("visitor:ctx:user:%s", user.UserID.String())).Build()).Error()
		_ = h.cache.Do(ctx, h.cache.B().Del().Key(vkcache.MustKey("recommendations:user:%s", user.UserID.String())).Build()).Error()

		return
	}

	if t != nil {
		ip := clientip.FromRequest(r)
		_ = h.cache.Do(ctx, h.cache.B().Del().Key(vkcache.MustKey("visitor:ctx:ip:%s:%s", t.ID.String(), ip)).Build()).Error()
		_ = h.cache.Do(ctx, h.cache.B().Del().Key(vkcache.MustKey("recommendations:ip:%s:%s", t.ID.String(), ip)).Build()).Error()
	}
}

// GetCardStyle handles GET /api/v1/feed/card-style — public endpoint.
// Returns the active feed card style from system_settings.
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md.
func (h *Handlers) GetCardStyle(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	raw := ""

	if h.settings != nil {
		if val, err := h.settings.GetSetting(r.Context(), t.ID, "feed.card_style"); err == nil {
			raw = val
		}
	}

	cfg := resolveCardStyleConfig(raw)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": cfg,
	})
}

// journeyOption is a single select option for purpose, stage, or visa route.
type journeyOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// parseCustomFieldOptions decodes a JSONB custom field options array into
// journeyOption values. Returns nil for missing or malformed options.
func parseCustomFieldOptions(raw []byte) []journeyOption {
	if len(raw) == 0 {
		return nil
	}

	var opts []journeyOption
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil
	}

	return opts
}

// customFieldOptionKeys maps the journey axes to their system custom field keys.
// Purpose and visa route are sourced from the custom field system (Rule 16).
// Journey stage is the canonical §87 Japa Stage state machine: the legacy
// japa_status custom field mixes non-state-machine values, so using it for
// guest onboarding options causes selections to be dropped or re-mapped on save.
const (
	journeyPurposeCustomFieldKey   = "purpose"
	journeyVisaRouteCustomFieldKey = "visa_route"
)

// journeyStageOptions is the canonical §87 Japa Stage option list. It must stay
// in sync with visitor.NormalizeJourneyStage and internal/japastage.ValidStage.
var journeyStageOptions = []journeyOption{
	{Value: "considering", Label: "Considering"},
	{Value: "planning", Label: "Planning"},
	{Value: "preparing", Label: "Preparing"},
	{Value: "applied", Label: "Applied"},
	{Value: "approved", Label: "Approved"},
	{Value: "relocated", Label: "Relocated"},
	{Value: "settled", Label: "Settled"},
}

// GetJourneyOptions handles GET /api/v1/feed/journey-options — public endpoint.
// Resolves the option lists for purpose, journey stage, and visa route from the
// custom field system so the guest onboarding UI never hardcodes option lists.
// Ref: 2026-08-21-journey-aware-recommendations-design.md §6.1.
func (h *Handlers) GetJourneyOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	t := tenant.FromContext(ctx)
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	if h.customFields == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "journey_options_unavailable", "Journey options are not configured")
		return
	}

	purpose, _ := h.customFields.GetCustomFieldByKey(ctx, journeyPurposeCustomFieldKey, t.ID)
	visa, _ := h.customFields.GetCustomFieldByKey(ctx, journeyVisaRouteCustomFieldKey, t.ID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"purposes":    parseCustomFieldOptions(purpose.Options),
			"stages":      journeyStageOptions,
			"visa_routes": parseCustomFieldOptions(visa.Options),
		},
	})
}
