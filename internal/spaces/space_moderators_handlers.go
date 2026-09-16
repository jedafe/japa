package spaces

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Space Moderator Handlers
// ---------------------------------------------------------------------------

// AssignModerator handles POST /api/v1/spaces/{slug}/moderators.
// Adds a user as a moderator of the space.
func (h *Handlers) AssignModerator(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var targetUserID pgtype.UUID

	switch {
	case strings.TrimSpace(req.UserID) != "":
		if err := targetUserID.Scan(req.UserID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user_id format")
			return
		}
	case strings.TrimSpace(req.Username) != "":
		// Resolve username → user_id within the platform (Rule 2 — sqlc query,
		// no raw SQL). Lets the space owner assign a moderator by handle
		// without first looking up the UUID.
		var usernameErr error

		targetUserID, usernameErr = h.Service.GetUserIDByUsername(r.Context(), strings.TrimSpace(req.Username))
		if usernameErr != nil {
			// GetUserIDByUsername returns a plain "user not found" error for
			// an unknown handle; map it to a 404 so the client gets a clear
			// message instead of a generic 500 (HandleServiceError does not
			// recognise the non-sentinel error).
			if strings.Contains(usernameErr.Error(), "user not found") {
				errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "No user with that username was found")
				return
			}

			slog.Error("spaces: AssignModerator username resolution failed", "username", req.Username, "error", usernameErr)
			handleServiceError(w, usernameErr)

			return
		}
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "user_id or username is required")
		return
	}

	// Determine who is assigning: site_admin or space_owner.
	// The route is behind spaces.moderator.assign permission. If the caller
	// is the space owner, use the space_owner path and require the target to
	// be an existing member (Section 03). Otherwise treat as site_admin.
	assignedBy := "site_admin"
	assignedByID := user.UserID

	isOwner, err := h.Service.IsOwner(r.Context(), space.ID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if isOwner {
		assignedBy = "space_owner"
		// Section 03: space_owner path requires the target to be an existing
		// member of the space before promoting to moderator.
		isMember, err := h.Service.IsMember(r.Context(), space.ID, targetUserID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if !isMember {
			errpkg.WriteError(w, http.StatusBadRequest, "not_a_member",
				"Target user must be a member of the space before being assigned as a moderator")
			return
		}
	}

	if err := h.Service.AssignModerator(r.Context(), gen.AssignSpaceModeratorParams{

		SpaceID:          space.ID,
		UserID:           targetUserID,
		AssignedBy:       assignedBy,
		AssignedByUserID: assignedByID,
	}); err != nil {
		slog.Error("spaces: AssignModerator failed", "space_id", space.ID.String(), "target_user_id", targetUserID.String(), "assigned_by", assignedBy, "error", err)
		handleServiceError(w, err)

		return
	}

	// Rule 6: invalidate the TARGET user's permission cache so the newly
	// assigned moderator's spaces.moderate.assigned node is visible
	// immediately (the middleware only flushed the CALLER's cache).
	h.invalidateTargetPermCache(r.Context(), targetUserID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"space_id":    space.ID,
			"user_id":     targetUserID,
			"assigned_by": assignedBy,
		},
	})
}

// RemoveModerator handles DELETE /api/v1/spaces/{slug}/moderators/{userId}.
// Removes a user from the space moderator list.
func (h *Handlers) RemoveModerator(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	userID, err := parseUUIDParam(r, "userId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID format")
		return
	}

	if err := h.Service.RemoveModerator(r.Context(), space.ID, userID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Rule 6: invalidate the removed moderator's permission cache so their
	// spaces.moderate.assigned node is revoked immediately.
	h.invalidateTargetPermCache(r.Context(), userID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "removed"}})
}

// ListModerators handles GET /api/v1/spaces/{slug}/moderators.
// Returns all moderators assigned to the space.
func (h *Handlers) ListModerators(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	mods, err := h.Service.ListModerators(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": mods})
}

// AdminListModerators handles GET /api/v1/admin/spaces/{id}/moderators.
func (h *Handlers) AdminListModerators(w http.ResponseWriter, r *http.Request) {

	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space ID")
		return
	}

	_, err = h.Service.GetSpaceByID(r.Context(), spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	mods, err := h.Service.ListModerators(r.Context(), spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": mods})
}

// AdminAssignModerator handles POST /api/v1/admin/spaces/{id}/moderators.
// Admin-scoped variant of the space moderator assignment that resolves the
// space by ID (not slug) and accepts either a user_id or username.
func (h *Handlers) AdminAssignModerator(w http.ResponseWriter, r *http.Request) {

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space ID")
		return
	}

	space, err := h.Service.GetSpaceByID(r.Context(), spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var targetUserID pgtype.UUID

	switch {
	case req.UserID != "":
		if err := targetUserID.Scan(req.UserID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user_id format")
			return
		}
	case req.Username != "":
		targetUserID, err = h.Service.GetUserIDByUsername(r.Context(), req.Username)
		if err != nil {
			handleServiceError(w, err)
			return
		}
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "user_id or username is required")
		return
	}

	if err := h.Service.AssignModerator(r.Context(), gen.AssignSpaceModeratorParams{

		SpaceID:          space.ID,
		UserID:           targetUserID,
		AssignedBy:       "site_admin",
		AssignedByUserID: user.UserID,
	}); err != nil {
		handleServiceError(w, err)
		return
	}

	// Rule 6: invalidate the TARGET user's permission cache.
	h.invalidateTargetPermCache(r.Context(), targetUserID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"space_id": space.ID.String(),
			"user_id":  targetUserID.String(),
		},
	})
}

// AdminRemoveModerator handles DELETE /api/v1/admin/spaces/{id}/moderators/{userId}.
func (h *Handlers) AdminRemoveModerator(w http.ResponseWriter, r *http.Request) {

	if user := auth.FromContext(r.Context()); user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space ID")
		return
	}

	space, err := h.Service.GetSpaceByID(r.Context(), spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var targetUserID pgtype.UUID

	switch {
	case chi.URLParam(r, "moderator_id") != "":
		if err := targetUserID.Scan(chi.URLParam(r, "moderator_id")); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid moderator_id format")
			return
		}
	case chi.URLParam(r, "userId") != "":
		if err := targetUserID.Scan(chi.URLParam(r, "userId")); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid userId format")
			return
		}
	case chi.URLParam(r, "username") != "":
		targetUserID, err = h.Service.GetUserIDByUsername(r.Context(), chi.URLParam(r, "username"))
		if err != nil {
			handleServiceError(w, err)
			return
		}
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "userId or username is required")
		return
	}

	if err := h.Service.RemoveModerator(r.Context(), space.ID, targetUserID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Rule 6: invalidate the removed moderator's permission cache.
	h.invalidateTargetPermCache(r.Context(), targetUserID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"space_id": space.ID.String(),
			"user_id":  targetUserID.String(),
		},
	})
}
