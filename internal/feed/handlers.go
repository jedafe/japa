package feed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
	vkcache "github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/valkey-io/valkey-go"
)

const defaultPageSize int32 = 20

// exploreCacheTTL is the TTL for jc:explore:{tenant}:{geo}:{page} (Section 53).
const exploreCacheTTL = 15 * time.Minute

// Handlers holds dependencies for feed HTTP handlers.
type Handlers struct {
	service  *Service
	settings *config.Settings
	cache    valkey.Client
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
// Ref: conventions/system-settings-catalog.md (content.posts_per_page)
func (h *Handlers) parsePageSize(r *http.Request) int32 {
	defaultSize := int32(defaultPageSize)
	if h.settings != nil {
		if t := tenant.FromContext(r.Context()); t != nil {
			if v := h.settings.GetIntOr(r.Context(), t.ID, "content.posts_per_page", int(defaultPageSize)); v > 0 {
				defaultSize = int32(v) //nolint:gosec // G109: bounded below by MaxPageSize clamp
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

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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

	// Persist to Valkey.
	if h.cache != nil {
		if raw, err := json.Marshal(stored); err == nil {
			key := vkcache.MustKey("visitor:journey:%s", token)
			_ = h.cache.Do(r.Context(), h.cache.B().Set().Key(key).Value(string(raw)).Ex(guestJourneyTTL).Build()).Error()
		}
	}

	// Set the cookie. Not HttpOnly so the frontend can read the token for
	// localStorage sync; the actual values live in Valkey.
	http.SetCookie(w, &http.Cookie{
		Name:     "jc_journey_token",
		Value:    token,
		Path:     "/",
		MaxAge:   int(guestJourneyTTL.Seconds()),
		SameSite: http.SameSiteLaxMode,
	})

	// Return the resolved context for the current request. The middleware has
	// already passed, so this is the previously-resolved context rather than
	// the new one; the caller should refresh to see the applied context.
	vc := visitor.FromContext(r.Context())
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": vc})
}

// GetCardStyle handles GET /api/v1/feed/card-style — public endpoint.
// Returns the active feed card style from system_settings.
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md
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
