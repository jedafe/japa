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

// ListSpaceTypes handles GET /api/v1/space-types and GET /api/v1/admin/space-types.
// Supports optional q (search by name/slug) and is_university query params for
// the admin panel DataTable search and filter.
func (h *Handlers) ListSpaceTypes(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	isUniversityFilter := strings.TrimSpace(r.URL.Query().Get("is_university"))

	if q != "" || isUniversityFilter != "" {
		types, err := h.Service.ListSpaceTypesFiltered(r.Context(), tenantID, q, isUniversityFilter)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
		return
	}

	types, err := h.Service.ListSpaceTypes(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// GetSpaceType handles GET /api/v1/space-types/{id}.
func (h *Handlers) GetSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	st, err := h.Service.GetSpaceType(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// CreateSpaceType handles POST /api/v1/space-types. API-created types are
// never system types.
func (h *Handlers) CreateSpaceType(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string  `json:"name"`
		Slug         string  `json:"slug"`
		Description  *string `json:"description"`
		Icon         *string `json:"icon"`
		FieldGroupID string  `json:"field_group_id"`
		IsUniversity bool    `json:"is_university"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var fieldGroupID pgtype.UUID
	if req.FieldGroupID != "" {
		if err := fieldGroupID.Scan(req.FieldGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid field_group_id format")
			return
		}
	}

	st, err := h.Service.CreateSpaceType(r.Context(), gen.CreateSpaceTypeParams{
		TenantID:     tenantID,
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Icon:         req.Icon,
		FieldGroupID: fieldGroupID,
		IsUniversity: req.IsUniversity,
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
		IsUniversity bool    `json:"is_university"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var fieldGroupID pgtype.UUID
	if req.FieldGroupID != "" {
		if err := fieldGroupID.Scan(req.FieldGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid field_group_id format")
			return
		}
	}

	st, err := h.Service.UpdateSpaceType(r.Context(), gen.UpdateSpaceTypeParams{
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		FieldGroupID: fieldGroupID,
		IsUniversity: req.IsUniversity,
		ID:           id,
		TenantID:     tenantID,
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteSpaceType(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
