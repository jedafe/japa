package spaces

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
)

// ---------------------------------------------------------------------------
// Space Parent Spaces (Many-to-Many Hierarchy) Handlers
// ---------------------------------------------------------------------------

// canManageOrAdmin returns true when the caller may manage the given space:
// the space owner, an assigned moderator, or a platform admin holding the
// spaces.admin node. On failure it writes the appropriate error response and
// returns false (SPACES-017 — spaces.edit is granted to member_role as
// "edit own spaces", so route-level RBAC alone cannot scope it per space).
func (h *Handlers) canManageOrAdmin(w http.ResponseWriter, r *http.Request, spaceID pgtype.UUID) bool {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return false
	}

	canManage, err := h.Service.CanManageSpace(r.Context(), spaceID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return false
	}

	if canManage {
		return true
	}

	if resolver := rbac.FromContext(r.Context()); resolver != nil &&
		resolver.HasPermission(r.Context(), user.UserID, "spaces.admin") {
		return true
	}

	errpkg.WriteError(w, http.StatusForbidden, "not_space_admin",
		"Only the space owner, an assigned moderator, or a platform admin can manage this space")

	return false
}

// AddParentSpace handles POST /api/v1/spaces/{slug}/parents.
// Links the space to a parent space via the junction table.
func (h *Handlers) AddParentSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	// SPACES-017: spaces.edit is granted to member_role as "edit own spaces"
	// — verify the caller manages this space (owner/moderator) or holds
	// spaces.admin before mutating the hierarchy.
	if !h.canManageOrAdmin(w, r, space.ID) {
		return
	}

	var req struct {
		ParentSpaceID string `json:"parent_space_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.ParentSpaceID) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "parent_space_id is required")
		return
	}

	var parentID pgtype.UUID
	if err := parentID.Scan(req.ParentSpaceID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent_space_id format")
		return
	}
	// Verify the parent space exists and belongs to the same platform.
	parent, err := h.Service.GetSpace(r.Context(), parentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	// SPACES-008: parent spaces must be primary (country/state/city).
	// Custom or global spaces cannot serve as parents (Section 05 line 14).
	if parent.Level != LevelCountry && parent.Level != LevelState && parent.Level != LevelCity {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent",
			"Parent spaces must be primary (country, state, or city)")
		return
	}

	if err := h.Service.AddParentSpace(r.Context(), space.ID, parentID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"space_id":        space.ID,
			"parent_space_id": parentID,
		},
	})
}

// RemoveParentSpace handles DELETE /api/v1/spaces/{slug}/parents/{parentId}.
// Unlinks the space from the given parent.
func (h *Handlers) RemoveParentSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	if !h.canManageOrAdmin(w, r, space.ID) {
		return
	}

	parentID, err := parseUUIDParam(r, "parentId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent ID format")
		return
	}

	if err := h.Service.RemoveParentSpace(r.Context(), space.ID, parentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Parent relationship not found")
			return
		}

		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "removed"}})
}

// ListParentSpaces handles GET /api/v1/spaces/{slug}/parents.
// Returns all parent spaces linked via the junction table.
func (h *Handlers) ListParentSpaces(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	parents, err := h.Service.GetParentSpaces(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": parents})
}

// SuggestParentSpaces handles GET /spaces/suggest-parents.
// SPACES-003: returns primary spaces (country/state/city) eligible to be
// parents of a custom space, filtered by the optional ?q= search param.
func (h *Handlers) SuggestParentSpaces(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	rows, err := h.Service.read.SuggestParentSpaces(r.Context(),

		search,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to suggest parents")
		return
	}

	spaces := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		spaces = append(spaces, map[string]any{"id": row.ID, "name": row.Name, "slug": row.Slug})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": spaces})
}
