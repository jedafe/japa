package feed

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for feed HTTP handlers.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// SetSettings injects the settings service for reading feed.card_style.
func (h *Handlers) SetSettings(s *config.Settings) {
	h.settings = s
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// optionalUser resolves the authenticated user from the request context, if present.
func (h *Handlers) optionalUser(r *http.Request) *auth.AuthenticatedUser {
	return auth.FromContext(r.Context())
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
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
func (h *Handlers) GetExploreFeed(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sort := parseSort(r)

	items, err := h.service.GetFeed(r.Context(), tenantID, "explore", nil, nil, nil, sort, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get explore feed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetPrimaryFeed handles GET /api/v1/feed/primary — personalized feed.
func (h *Handlers) GetPrimaryFeed(w http.ResponseWriter, r *http.Request) {
	user := h.optionalUser(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sort := parseSort(r)

	items, err := h.service.GetFeed(r.Context(), tenantID, "primary", &user.UserID, nil, nil, sort, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get primary feed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetForYouFeed handles GET /api/v1/feed/for-you — personalized ranked feed.
func (h *Handlers) GetForYouFeed(w http.ResponseWriter, r *http.Request) {
	user := h.optionalUser(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sortMode := parseSort(r)

	items, err := h.service.GetForYouFeed(r.Context(), tenantID, user.UserID, sortMode, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get For-You feed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetSpaceFeed handles GET /api/v1/feed/space/{space_id}.
func (h *Handlers) GetSpaceFeed(w http.ResponseWriter, r *http.Request) {
	user := h.optionalUser(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	spaceIDStr := chi.URLParam(r, "space_id")

	var spaceID pgtype.UUID

	if err := spaceID.Scan(spaceIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space ID format")
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sort := parseSort(r)

	items, err := h.service.GetFeed(r.Context(), tenantID, "space", nil, &spaceID, nil, sort, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get space feed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetProfileFeed handles GET /api/v1/feed/profile/{user_id}. Public per
// Section 32:12 (profile timeline audience = "Anyone"): the viewer is
// optional — guests get the same public rows as an authenticated viewer,
// since this handler does not (yet) pass the viewer into GetFeed for
// personalization.
func (h *Handlers) GetProfileFeed(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	authorIDStr := chi.URLParam(r, "user_id")

	var authorID pgtype.UUID

	if err := authorID.Scan(authorIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := h.parsePageSize(r)
	offset := parsePageOffset(r)
	sort := parseSort(r)

	items, err := h.service.GetFeed(r.Context(), tenantID, "profile", nil, nil, &authorID, sort, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get profile feed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetCardStyle handles GET /api/v1/feed/card-style — public endpoint.
// Returns the active feed card style from system_settings.
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md
func (h *Handlers) GetCardStyle(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	raw := ""
	if h.settings != nil {
		if val, err := h.settings.GetSetting(r.Context(), t.ID, "feed.card_style"); err == nil {
			raw = val
		}
	}

	cfg := resolveCardStyleConfig(raw)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": cfg,
	})
}
