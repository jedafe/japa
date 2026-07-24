// Package trending — HTTP handlers for Trending & Discovery (Phase 28).
//
// Every handler is a method on Handlers so dependencies (Store) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}.
package trending

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

// Pagination defaults.
const (
	defaultLimit = 20
	maxLimit     = 50
)

// Handlers holds dependencies for trending HTTP handlers.
type Handlers struct {
	Store *Store
}

// NewHandlers creates Handlers with the given Store.
func NewHandlers(store *Store) *Handlers {
	return &Handlers{Store: store}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

func parseLimit(r *http.Request) int32 {
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			if parsed > maxLimit {
				parsed = maxLimit
			}

			return int32(parsed) //nolint:gosec // G115: bounded by maxLimit
		}
	}

	return defaultLimit
}

func parseWindow(r *http.Request) string {
	if v := r.URL.Query().Get("window"); v != "" {
		switch v {
		case "24h", "7d", "30d":
			return v
		}
	}

	return "24h"
}

func parseGeoCode(r *http.Request) string {
	return r.URL.Query().Get("geo")
}

// ---------------------------------------------------------------------------
// Public trending endpoints
// ---------------------------------------------------------------------------

// GetTrendingContent handles GET /api/v1/trending/content.
func (h *Handlers) GetTrendingContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)

	data, err := h.Store.GetTrendingContent(r.Context(), tenantID, window, geoCode, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingTags handles GET /api/v1/trending/tags.
func (h *Handlers) GetTrendingTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)

	data, err := h.Store.GetTrendingTags(r.Context(), tenantID, window, geoCode, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingSpaces handles GET /api/v1/trending/spaces.
func (h *Handlers) GetTrendingSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)

	data, err := h.Store.GetTrendingSpaces(r.Context(), tenantID, window, geoCode, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingUsers handles GET /api/v1/trending/users.
func (h *Handlers) GetTrendingUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)

	data, err := h.Store.GetTrendingUsers(r.Context(), tenantID, window, geoCode, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// ---------------------------------------------------------------------------
// Auth-required suggestion endpoints
// ---------------------------------------------------------------------------

// GetSuggestedUsers handles GET /api/v1/suggestions/users.
func (h *Handlers) GetSuggestedUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)

	data, err := h.Store.GetSuggestedUsers(r.Context(), user.UserID, tenantID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetSuggestedSpaces handles GET /api/v1/suggestions/spaces.
func (h *Handlers) GetSuggestedSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)

	data, err := h.Store.GetSuggestedSpaces(r.Context(), user.UserID, tenantID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetSuggestedTags handles GET /api/v1/suggestions/tags.
func (h *Handlers) GetSuggestedTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)

	data, err := h.Store.GetSuggestedTags(r.Context(), user.UserID, tenantID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}
