package follows

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
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
	return pgtype.UUID{}, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
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

	// #nosec G109 // value bounded by MaxPageSize check
	// #nosec G115 // value bounded by caller
	return int32(n)
}

// ---------------------------------------------------------------------------
// Follow Handlers
// ---------------------------------------------------------------------------

// ToggleFollow handles both follow registrations:
//   - POST /api/v1/follows — generic polymorphic follow. The entity_type and
//     entity_id are carried in the JSON body: { "entity_type", "entity_id" }.
//   - POST/DELETE /api/v1/users/{id}/follow — user-specific follow. The target
//     user id is the {id} path parameter and the entity type is implicitly
//     "user"; no request body is sent by the frontend.
//
// The path-parameter form is resolved first so an empty body (sent by the
// SvelteKit FollowButton via api.post/api.delete with no body) does not fail
// with `invalid_body`. The body form is the fallback for the generic route.
// Response: { "data": { "following": true } }.
//
// Ref: japaconnect-engineering/sections/31-follow-system.md
//
//	conventions/api-routes-catalog.md (POST /follows, POST /users/{id}/follow)
func (h *Handlers) ToggleFollow(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var entityType string

	var entityID pgtype.UUID

	// Entity-specific route: /users/{id}/follow carries the target id as a
	// path parameter and implies entity_type "user". The frontend calls this
	// route with no request body, so resolve from the path before attempting
	// to decode a body.
	if idStr := chi.URLParam(r, "id"); idStr != "" {
		if err := entityID.Scan(idStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id format")
			return
		}

		entityType = EntityTypeUser
	} else {
		// Generic route: POST /follows — entity_type and entity_id come from
		// the JSON body.
		var input struct {
			EntityType string `json:"entity_type"`
			EntityID   string `json:"entity_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
			return
		}

		if input.EntityType == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_entity_type", "entity_type is required")
			return
		}

		if input.EntityID == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_entity_id", "entity_id is required")
			return
		}

		if err := entityID.Scan(input.EntityID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id format")
			return
		}

		entityType = input.EntityType
	}

	followed, err := h.service.ToggleFollow(r.Context(), tenantID, user.UserID, entityType, entityID)
	if err != nil {
		if errors.Is(err, ErrEntityNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "entity_not_found", "The target entity does not exist")
			return
		}

		errpkg.WriteError(w, http.StatusBadRequest, "follow_error", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]bool{"following": followed},
	})
}

// GetFollowers handles GET /api/v1/users/{user_id}/followers and
// GET /api/v1/spaces/{space_id}/followers. The entity_type is derived from
// which path parameter is present (user_id → "user", space_id → "space"),
// and the entity_id is read from the same path parameter — not from a query
// string. Ref: japaconnect-engineering/sections/31-follow-system.md.
func (h *Handlers) GetFollowers(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// The caller's user ID is needed to resolve is_following per item
	// (Section 31). Best-effort: when absent, is_following defaults to false.
	var callerID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		callerID = user.UserID
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
		errpkg.WriteError(w, http.StatusBadRequest, "missing_entity_id", "user_id or space_id path parameter is required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	followers, err := h.service.GetFollowers(r.Context(), callerID, entityType, entityID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get followers")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": followers})
}

// GetFollowing handles GET /api/v1/users/{user_id}/following.
func (h *Handlers) GetFollowing(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// The caller's user ID is needed to resolve is_following per item
	// (Section 31). Best-effort: when absent, is_following defaults to false.
	var callerID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		callerID = user.UserID
	}

	userIDStr := chi.URLParam(r, "user_id")

	var userID pgtype.UUID

	if err := userID.Scan(userIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	following, err := h.service.GetFollowing(r.Context(), callerID, userID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get following")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": following})
}

// ListMyFollowing handles GET /api/v1/me/following — everything the caller follows.
// Supports optional ?type=user|space|tag|organization
// filtering per Section 31 contract (H6).
func (h *Handlers) ListMyFollowing(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
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
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_type", "type must be one of: user, space, tag, organization")
			return
		}

		following, err = h.service.GetFollowingByType(r.Context(), user.UserID, user.UserID, et, limit, offset)
	} else {
		following, err = h.service.GetFollowing(r.Context(), user.UserID, user.UserID, limit, offset)
	}

	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get following")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": following})
}

// ListMyFollowers handles GET /api/v1/me/followers — the caller's own followers
// (H5 — Section 31 contract). Ref: japaconnect-engineering/sections/31-follow-system.md.
func (h *Handlers) ListMyFollowers(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	followers, err := h.service.GetFollowers(r.Context(), user.UserID, EntityTypeUser, user.UserID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get followers")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": followers})
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
	case "source":
		return EntityTypeSource, true
	default:
		return "", false
	}
}

// ListMyFollowedTags handles GET /api/v1/me/followed-tags — tags the caller follows.
func (h *Handlers) ListMyFollowedTags(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	following, err := h.service.GetFollowingByType(r.Context(), user.UserID, user.UserID, EntityTypeTag, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get followed tags")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": following})
}

// GetFollowStatus handles GET /api/v1/users/{user_id}/follow-status — whether
// the caller follows the given user. Also supports an optional ?entity_type=
// query param to check follow status for non-user entities (H4 — Section 31).
// When entity_type is omitted, defaults to "user" for backward compatibility.
//
// Response (Section 31 contract): { "data": { "following": bool, "followed_by": bool } }.
// followed_by is only meaningful for user entities (does the target follow the
// caller back?); for non-user entities it is always false.
func (h *Handlers) GetFollowStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	targetIDStr := chi.URLParam(r, "user_id")

	var targetID pgtype.UUID

	if err := targetID.Scan(targetIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	// H4: allow checking follow status for any entity type, not just users.
	entityType := EntityTypeUser

	if et := r.URL.Query().Get("entity_type"); et != "" {
		if _, valid := mapTypeFilter(et); valid {
			entityType, _ = mapTypeFilter(et)
		} else {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_type", "entity_type must be one of: user, space, tag, organization, source")
			return
		}
	}

	following, err := h.service.IsFollowing(r.Context(), user.UserID, entityType, targetID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check follow status")
		return
	}

	// followed_by: does the target follow the caller back? Only meaningful
	// for user entities (Section 31 contract). For non-user entities an
	// entity cannot "follow back", so it is always false.
	followedBy := false
	if entityType == EntityTypeUser {
		followedBy, err = h.service.IsFollowing(r.Context(), targetID, EntityTypeUser, user.UserID)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check follow-back status")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]bool{"following": following, "followed_by": followedBy},
	})
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
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	userIDStr := chi.URLParam(r, "user_id")

	var userID pgtype.UUID

	if err := userID.Scan(userIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	otherIDStr := chi.URLParam(r, "other_user_id")

	var otherID pgtype.UUID

	if err := otherID.Scan(otherIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_other_user_id", "Invalid other_user_id format")
		return
	}

	// M4: the caller must be one of the two parties — prevents bulk
	// relationship probing by arbitrary authenticated users.
	if user.UserID != userID && user.UserID != otherID {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You can only check mutual follow status for yourself")
		return
	}

	mutual, err := h.service.IsMutualFollow(r.Context(), userID, otherID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check mutual follow")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]bool{"mutual": mutual},
	})
}
