package follows

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for follow HTTP handlers.
type Handlers struct {
	service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
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

// ---------------------------------------------------------------------------
// Follow Handlers
// ---------------------------------------------------------------------------

// ToggleFollow handles POST /api/v1/follows.
// Request body: { "entity_type": "user", "entity_id": "..." }
// Response: { "data": { "following": true } }.
func (h *Handlers) ToggleFollow(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.EntityType == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_type", "entity_type is required")
		return
	}

	if input.EntityID == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_id", "entity_id is required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(input.EntityID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id format")
		return
	}

	followed, err := h.service.ToggleFollow(r.Context(), tenantID, user.UserID, input.EntityType, entityID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "follow_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]bool{"following": followed},
	})
}

// GetFollowers handles GET /api/v1/users/{user_id}/followers and
// GET /api/v1/spaces/{space_id}/followers. The entity_type is derived from
// which path parameter is present (user_id → "user", space_id → "space"),
// and the entity_id is read from the same path parameter — not from a query
// string. Ref: japaconnect-engineering/sections/31-follow-system.md
func (h *Handlers) GetFollowers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Derive (entity_type, entity_id) from whichever path param is set.
	// Routes: /users/{user_id}/followers, /spaces/{space_id}/followers.
	entityIDStr := chi.URLParam(r, "user_id")
	entityType := EntityTypeUser

	if entityIDStr == "" {
		entityIDStr = chi.URLParam(r, "space_id")
		entityType = EntityTypeSpace
	}

	if entityIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_id", "user_id or space_id path parameter is required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	followers, err := h.service.GetFollowers(r.Context(), tenantID, entityType, entityID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get followers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": followers})
}

// GetFollowing handles GET /api/v1/users/{user_id}/following.
func (h *Handlers) GetFollowing(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	userIDStr := chi.URLParam(r, "user_id")

	var userID pgtype.UUID

	if err := userID.Scan(userIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	following, err := h.service.GetFollowing(r.Context(), tenantID, userID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get following")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": following})
}

// ListMyFollowing handles GET /api/v1/me/following — everything the caller follows.
// Supports optional ?type=user|space|tag|organization|professional_profile
// filtering per Section 31 contract (H6).
func (h *Handlers) ListMyFollowing(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	// H6: Section 31 contract — ?type= filters by entity type.
	typeFilter := r.URL.Query().Get("type")

	var following []FollowingInfo
	var err error
	if typeFilter != "" {
		et, valid := mapTypeFilter(typeFilter)
		if !valid {
			writeError(w, http.StatusBadRequest, "invalid_type", "type must be one of: user, space, tag, organization, professional_profile")
			return
		}
		following, err = h.service.GetFollowingByType(r.Context(), tenantID, user.UserID, et, limit, offset)
	} else {
		following, err = h.service.GetFollowing(r.Context(), tenantID, user.UserID, limit, offset)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get following")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": following})
}

// ListMyFollowers handles GET /api/v1/me/followers — the caller's own followers
// (H5 — Section 31 contract). Ref: japaconnect-engineering/sections/31-follow-system.md
func (h *Handlers) ListMyFollowers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	followers, err := h.service.GetFollowers(r.Context(), tenantID, EntityTypeUser, user.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get followers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": followers})
}

// mapTypeFilter maps a ?type= query value to the internal entity type constant.
func mapTypeFilter(t string) (string, bool) {
	switch t {
	case "user":
		return EntityTypeUser, true
	case "space":
		return EntityTypeSpace, true
	case "tag":
		return EntityTypeTag, true
	case "organization":
		return EntityTypeOrganization, true
	case "profile", "professional_profile":
		return EntityTypeProfessionalProfile, true
	default:
		return "", false
	}
}

// ListMyFollowedTags handles GET /api/v1/me/followed-tags — tags the caller follows.
func (h *Handlers) ListMyFollowedTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	following, err := h.service.GetFollowingByType(r.Context(), tenantID, user.UserID, EntityTypeTag, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get followed tags")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": following})
}

// GetFollowStatus handles GET /api/v1/users/{user_id}/follow-status — whether
// the caller follows the given user. Also supports an optional ?entity_type=
// query param to check follow status for non-user entities (H4 — Section 31).
// When entity_type is omitted, defaults to "user" for backward compatibility.
func (h *Handlers) GetFollowStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	targetIDStr := chi.URLParam(r, "user_id")

	var targetID pgtype.UUID

	if err := targetID.Scan(targetIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	// H4: allow checking follow status for any entity type, not just users.
	entityType := EntityTypeUser
	if et := r.URL.Query().Get("entity_type"); et != "" {
		if _, valid := mapTypeFilter(et); valid {
			entityType, _ = mapTypeFilter(et)
		} else {
			writeError(w, http.StatusBadRequest, "invalid_entity_type", "entity_type must be one of: user, space, tag, organization, professional_profile")
			return
		}
	}

	following, err := h.service.IsFollowing(r.Context(), user.UserID, entityType, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to check follow status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"following": following}})
}

// CheckMutual handles GET /api/v1/users/{user_id}/mutual/{other_user_id}.
// M4: restricted to the caller being one of the two parties (user_id or
// other_user_id must equal the authenticated user's ID) to prevent bulk
// relationship mapping by arbitrary users.
func (h *Handlers) CheckMutual(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	userIDStr := chi.URLParam(r, "user_id")

	var userID pgtype.UUID

	if err := userID.Scan(userIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	otherIDStr := chi.URLParam(r, "other_user_id")

	var otherID pgtype.UUID

	if err := otherID.Scan(otherIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_other_user_id", "Invalid other_user_id format")
		return
	}

	// M4: the caller must be one of the two parties — prevents bulk
	// relationship probing by arbitrary authenticated users.
	if user.UserID != userID && user.UserID != otherID {
		writeError(w, http.StatusForbidden, "forbidden", "You can only check mutual follow status for yourself")
		return
	}

	mutual, err := h.service.IsMutualFollow(r.Context(), userID, otherID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to check mutual follow")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]bool{"mutual": mutual},
	})
}
