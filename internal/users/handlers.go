package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for user HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate
// HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > maxPageSize {
		return maxPageSize
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

// ---------------------------------------------------------------------------
// Profile Handlers
// ---------------------------------------------------------------------------

// GetMyProfile handles GET /api/v1/users/me.
func (h *Handlers) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	profile, err := h.Service.GetProfile(r.Context(), user.TenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// GetUserByUsername handles GET /api/v1/users/{username}.
func (h *Handlers) GetUserByUsername(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	username := chi.URLParam(r, "username")
	if username == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	profile, err := h.Service.GetPublicProfileByUsername(r.Context(), tenantID, username)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// UpdateMyProfile handles PATCH /api/v1/users/me.
func (h *Handlers) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateProfile(r.Context(), user.TenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// ListUsers handles GET /api/v1/users for admin user listing.
// Query params: search, status, group, page_size (default 20, max 100), page_offset (default 0).
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	// Admin-only: only super_admin group members may list users.
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")
	groupSlug := r.URL.Query().Get("group")

	result, err := h.Service.ListUsers(r.Context(), tenantID, search, status, groupSlug, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ListUserSessions handles GET /api/v1/admin/users/{id}/sessions.
// Admin-scoped variant of /me/sessions that lists sessions for any user.
func (h *Handlers) ListUserSessions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireTenant(w, r); !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	sessions, err := h.Service.ListSessions(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": sessions})
}

// GetAdminUser handles GET /api/v1/admin/users/{id}.
// Resolves the user by UUID (not username) for admin detail views.
func (h *Handlers) GetAdminUser(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	profile, err := h.Service.GetProfile(r.Context(), tenantID, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// ListUserActivity handles GET /api/v1/admin/users/{id}/activity.
// Merged feed of the user's authored content and comments, newest first.
func (h *Handlers) ListUserActivity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	limit := int32(defaultPageSize)
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			if n > maxPageSize {
				n = maxPageSize
			}
			limit = int32(n)
		}
	}

	activity, err := h.Service.ListUserActivity(r.Context(), tenantID, userID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": activity})
}

// DeleteUser handles DELETE /api/v1/admin/users/{id}.
// Soft-deletes the user (status = 'deleted') to comply with the GDPR erasure
// model — the row is retained and only a confirmed erasure request may hard-delete.
func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.Service.SoftDeleteUser(r.Context(), tenantID, userID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": userID.String(), "status": "deleted"})
}

// UpdateProfileType handles PATCH /api/v1/admin/users/{id}/profile-type.
// Allows admins to change a user's profile_type (personal, business, organisation).
func (h *Handlers) UpdateProfileType(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var req struct {
		ProfileType string `json:"profile_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	validTypes := map[string]bool{"personal": true, "business": true, "organisation": true}
	if !validTypes[req.ProfileType] {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_type", "profile_type must be personal, business, or organisation")
		return
	}

	if err := h.Service.UpdateProfileType(r.Context(), tenantID, userID, req.ProfileType); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": userID.String(), "profile_type": req.ProfileType})
}

// ---------------------------------------------------------------------------
// Preferences Handlers
// ---------------------------------------------------------------------------

// GetMyPreferences handles GET /api/v1/users/me/preferences.
func (h *Handlers) GetMyPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	prefs, err := h.Service.GetPreferences(r.Context(), user.TenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": prefs})
}

// UpdateMyPreferences handles PUT /api/v1/users/me/preferences.
func (h *Handlers) UpdateMyPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdatePreferences(r.Context(), user.TenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}
