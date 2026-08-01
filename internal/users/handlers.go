package users

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Handlers holds dependencies for user HTTP handlers.
type Handlers struct {
	Service *Service
	// Redis is used for VisitorContext cache invalidation when the user's
	// content_country_preference changes (ADR-064). Optional — when nil,
	// cache invalidation is skipped (the 15-minute TTL will eventually
	// expire the stale entry).
	Redis valkeycompat.Cmdable
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
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

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
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

// handleServiceError translates a Service-layer error into the appropriate
// HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrDuplicate):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// GetUserByUsername handles GET /api/v1/users/{username}.
func (h *Handlers) GetUserByUsername(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	username := chi.URLParam(r, "username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	profile, err := h.Service.GetPublicProfileByUsername(r.Context(), tenantID, username)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// UpdateMyProfile handles PATCH /api/v1/users/me.
func (h *Handlers) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateProfile(r.Context(), user.TenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
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
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// UserModerationConfig is the API payload returned by
// GET /admin/users/config. It surfaces the platform's valid user statuses,
// profile types, violation types, restriction operations, and restriction
// durations so the admin frontend does not need to hardcode them
// (BUG-017 through BUG-021).
// Ref: japaconnect-engineering/sections/03-rbac.md (user statuses)
//
//	japaconnect-engineering/sections/46-impersonation.md (moderation)
type UserModerationConfig struct {
	Statuses             []ConfigOption   `json:"statuses"`
	ProfileTypes         []ConfigOption   `json:"profile_types"`
	ViolationTypes       []ConfigOption   `json:"violation_types"`
	RestrictionOps       []ConfigOption   `json:"restriction_operations"`
	RestrictionDurations []ConfigDuration `json:"restriction_durations"`
}

// ConfigOption is a value/label pair for a dropdown or filter.
type ConfigOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ConfigDuration is a value/label pair for a duration selector.
type ConfigDuration struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// GetUserModerationConfig handles GET /api/v1/admin/users/config.
// Returns the platform's valid user statuses, profile types, violation
// types, restriction operations, and restriction durations so the admin
// frontend does not hardcode them (BUG-017 through BUG-021).
// Ref: japaconnect-engineering/sections/03-rbac.md (user statuses/profile types)
//
//	japaconnect-engineering/sections/46-impersonation.md (moderation)
func (h *Handlers) GetUserModerationConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireTenant(w, r); !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	cfg, err := h.Service.GetUserModerationConfig(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": cfg})
}

// ListUserSessions handles GET /api/v1/admin/users/{id}/sessions.
// Admin-scoped variant of /me/sessions that lists sessions for any user.
func (h *Handlers) ListUserSessions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	sessions, err := h.Service.ListSessions(r.Context(), tenantID, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": sessions})
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	profile, err := h.Service.GetProfile(r.Context(), tenantID, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// ListUserActivity handles GET /api/v1/admin/users/{id}/activity.
// Merged feed of the user's authored content and comments, newest first.
func (h *Handlers) ListUserActivity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": activity})
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
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.Service.SoftDeleteUser(r.Context(), tenantID, userID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": userID.String(), "status": "deleted"})
}

// UpdateProfileType handles PATCH /api/v1/admin/users/{id}/profile-type.
// Allows admins to change a user's profile_type (personal, business, organisation).
func (h *Handlers) UpdateProfileType(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var req struct {
		ProfileType string `json:"profile_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	validTypes := map[string]bool{"personal": true, "business": true, "organisation": true}
	if !validTypes[req.ProfileType] {
		writeError(w, http.StatusBadRequest, "invalid_type", "profile_type must be personal, business, or organisation")
		return
	}

	if err := h.Service.UpdateProfileType(r.Context(), tenantID, userID, req.ProfileType); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": userID.String(), "profile_type": req.ProfileType})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": prefs})
}

// UpdateMyPreferences handles PUT /api/v1/users/me/preferences.
func (h *Handlers) UpdateMyPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdatePreferences(r.Context(), user.TenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// UpdateMyContentCountry handles PATCH /api/v1/me/preferences/content-country.
// Sets the authenticated user's content_country_preference for geo-aware
// personalization (ADR-064). An empty body or "country": "" clears the
// preference, causing the system to fall back to GeoIP-based detection.
func (h *Handlers) UpdateMyContentCountry(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var body struct {
		Country string `json:"country"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	country := strings.ToUpper(strings.TrimSpace(body.Country))
	if country != "" && len(country) != 2 {
		writeError(w, http.StatusBadRequest, "invalid_country", "Country must be a 2-letter ISO code")
		return
	}

	if err := h.Service.SetContentCountry(r.Context(), user.TenantID, user.UserID, country); err != nil {
		handleServiceError(w, err)
		return
	}

	// Invalidate the VisitorContext cache for this user so the next request
	// picks up the new preference immediately. Delete both the session-scoped
	// key (jc:visitor:ctx:{jti}, the primary key for tokens with a JTI) and
	// the user-id fallback key (jc:visitor:ctx:user:{id}, for legacy tokens
	// without a JTI). Without the session-scoped deletion the resolver would
	// return the stale cached context for up to 30 minutes.
	if h.Redis != nil {
		if user.SessionID != "" {
			_ = h.Redis.Del(r.Context(), "jc:visitor:ctx:"+user.SessionID).Err()
		}
		_ = h.Redis.Del(r.Context(), "jc:visitor:ctx:user:"+user.UserID.String()).Err()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":          "updated",
			"content_country": country,
		},
	})
}

// GetUserAnalytics handles GET /users/{id}/analytics — the profile Analytics
// dashboard. Owner-only per JEOS api-routes-catalog §342: [auth, self] — the
// caller must be the profile owner (viewer id == {id}). Guests and other users
// receive 403 analytics_forbidden. Gated by requireActiveProfileUser on the
// route (404 for banned/suspended/deleted). Returns aggregate stats for the
// {id} user's published+public content plus follower/following counts.
//
// Ref: japaconnect-engineering/sections/33-professional-profiles.md
//      japaconnect-engineering/conventions/api-routes-catalog.md §342
func (h *Handlers) GetUserAnalytics(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	// Owner-only: analytics is a self-dashboard. Guests are blocked by
	// RequireAuth on the route; other authenticated users are blocked here.
	if userID != user.UserID {
		writeError(w, http.StatusForbidden, "analytics_forbidden", "You can only view your own analytics")
		return
	}

	stats, err := h.Service.store.read.GetPublicAuthorAnalytics(r.Context(), gen.GetPublicAuthorAnalyticsParams{
		TenantID: tenantID,
		AuthorID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		slog.Error("users: GetUserAnalytics failed", "error", err)
		writeError(w, http.StatusInternalServerError, "analytics_error", "Failed to load analytics")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": stats})
}
