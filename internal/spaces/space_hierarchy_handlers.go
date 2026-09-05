package spaces

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Space Parent Spaces (Many-to-Many Hierarchy) Handlers
// ---------------------------------------------------------------------------

// AddParentSpace handles POST /api/v1/spaces/{slug}/parents.
// Links the space to a parent space via the junction table.
func (h *Handlers) AddParentSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
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
	// Verify the parent space exists and belongs to the same tenant.
	parent, err := h.Service.GetSpace(r.Context(), parentID, tenantID)
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	parents, err := h.Service.GetParentSpaces(r.Context(), space.ID, tenantID)
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
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))

	rows, err := h.Service.read.SuggestParentSpaces(r.Context(), gen.SuggestParentSpacesParams{
		TenantID: tenantID,
		Search:   search,
	})
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
