package spaces

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Space Type Handlers
// ---------------------------------------------------------------------------

var allowedSpaceTypeCategories = map[string]bool{
	"education":  true,
	"business":   true,
	"profession": true,
	"interest":   true,
	"lifestyle":  true,
}

// isAllowedCategory returns true for the categories that can be assigned to
// admin-created (non-system) space types.
func isAllowedCategory(category string) bool {
	return allowedSpaceTypeCategories[strings.ToLower(strings.TrimSpace(category))]
}

// ListSpaceTypes handles GET /api/v1/space-types and GET /api/v1/admin/space-types.
// Supports optional q (search by name/slug) and category query params for the
// admin panel DataTable search and filter. The public member-facing endpoint
// can pass selectable=true to hide geo (country/state/city) types.
func (h *Handlers) ListSpaceTypes(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	categoryFilter := strings.TrimSpace(r.URL.Query().Get("category"))
	selectableOnly := strings.TrimSpace(r.URL.Query().Get("selectable")) == "true"

	if q != "" || categoryFilter != "" || selectableOnly {
		types, err := h.Service.ListSpaceTypesFiltered(r.Context(), q, categoryFilter, selectableOnly)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})

		return
	}

	types, err := h.Service.ListSpaceTypes(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// ListSpaceTypeCategories handles GET /api/v1/admin/space-type-categories.
func (h *Handlers) ListSpaceTypeCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.Service.ListSpaceTypeCategories(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": categories})
}

// GetSpaceType handles GET /api/v1/space-types/{id}.
func (h *Handlers) GetSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	st, err := h.Service.GetSpaceType(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// CreateSpaceType handles POST /api/v1/space-types. API-created types are
// never system types and must be in an allowed category.
func (h *Handlers) CreateSpaceType(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string  `json:"name"`
		Slug         string  `json:"slug"`
		Description  *string `json:"description"`
		Icon         *string `json:"icon"`
		FieldGroupID string  `json:"field_group_id"`
		Category     string  `json:"category"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
		return
	}

	if !isAllowedCategory(req.Category) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_category",
			"Category must be one of: education, business, profession, interest, lifestyle")
		return
	}

	var fieldGroupID pgtype.UUID
	if req.FieldGroupID != "" {
		if err := fieldGroupID.Scan(req.FieldGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid field_group_id format")
			return
		}
	}

	category := strings.ToLower(strings.TrimSpace(req.Category))

	st, err := h.Service.CreateSpaceType(r.Context(), gen.CreateSpaceTypeParams{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Icon:         req.Icon,
		FieldGroupID: fieldGroupID,
		Category:     &category,
		IsSystem:     false,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": st})
}

// UpdateSpaceType handles PATCH /api/v1/space-types/{id}. Slug is immutable
// and therefore not accepted in the request body.
func (h *Handlers) UpdateSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Name         string  `json:"name"`
		Description  *string `json:"description"`
		Icon         *string `json:"icon"`
		FieldGroupID string  `json:"field_group_id"`
		Category     string  `json:"category"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	if req.Category != "" && !isAllowedCategory(req.Category) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_category",
			"Category must be one of: education, business, profession, interest, lifestyle")
		return
	}

	var fieldGroupID pgtype.UUID
	if req.FieldGroupID != "" {
		if err := fieldGroupID.Scan(req.FieldGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid field_group_id format")
			return
		}
	}

	var category *string

	if req.Category != "" {
		c := strings.ToLower(strings.TrimSpace(req.Category))
		category = &c
	}

	st, err := h.Service.UpdateSpaceType(r.Context(), gen.UpdateSpaceTypeParams{
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		FieldGroupID: fieldGroupID,
		Category:     category,
		ID:           id,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// DeleteSpaceType handles DELETE /api/v1/space-types/{id}.
func (h *Handlers) DeleteSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// BUG-022: surface 403/409 instead of a raw 500 when the type is a
	// system type or still in use by spaces.
	st, err := h.Service.GetSpaceType(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if st.IsSystem {
		errpkg.WriteError(w, http.StatusForbidden, "system_type", "System space types cannot be deleted")
		return
	}
	inUse, err := h.Service.read.CountSpacesByType(r.Context(), gen.CountSpacesByTypeParams{SpaceTypeID: id})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if inUse > 0 {
		errpkg.WriteError(w, http.StatusConflict, "type_in_use", "Space type is still used by existing spaces")
		return
	}

	if err := h.Service.DeleteSpaceType(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
