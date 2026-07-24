package bookmarkfolders

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for bookmark folder HTTP handlers.
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

// ListFolders handles GET /api/v1/me/bookmark-folders.
func (h *Handlers) ListFolders(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	folders, err := h.service.ListFolders(r.Context(), tenantID, user.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list bookmark folders")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": folders})
}

// CreateFolder handles POST /api/v1/me/bookmark-folders.
func (h *Handlers) CreateFolder(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	folder, err := h.service.CreateFolder(r.Context(), tenantID, user.UserID, input.Name)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create bookmark folder")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": folder})
}

// UpdateFolder handles PATCH /api/v1/me/bookmark-folders/{id}.
func (h *Handlers) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	folder, err := h.service.UpdateFolder(r.Context(), id, tenantID, user.UserID, input.Name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "Bookmark folder not found")
			return
		}

		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update bookmark folder")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": folder})
}

// DeleteFolder handles DELETE /api/v1/me/bookmark-folders/{id}.
func (h *Handlers) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.service.DeleteFolder(r.Context(), id, tenantID, user.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "Bookmark folder not found")
			return
		}

		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete bookmark folder")
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}
