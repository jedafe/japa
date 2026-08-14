package bookmarkfolders

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
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
		slog.Warn("list bookmark folders failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list bookmark folders")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": folders})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}
	if len(input.Name) > MaxFolderNameLength {
		errpkg.WriteError(w, http.StatusBadRequest, "name_too_long", "Folder name must be 100 characters or fewer")
		return
	}

	folder, err := h.service.CreateFolder(r.Context(), tenantID, user.UserID, input.Name)
	if err != nil {
		if errors.Is(err, errpkg.ErrDuplicate) {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_bookmark_folder", "A folder with this name already exists")
			return
		}
		slog.Warn("create bookmark folder failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create bookmark folder")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": folderResponseFromModel(folder)})
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

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}
	if len(input.Name) > MaxFolderNameLength {
		errpkg.WriteError(w, http.StatusBadRequest, "name_too_long", "Folder name must be 100 characters or fewer")
		return
	}

	folder, err := h.service.UpdateFolder(r.Context(), id, tenantID, user.UserID, input.Name)
	if err != nil {
		if errors.Is(err, errpkg.ErrNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Bookmark folder not found")
			return
		}
		if errors.Is(err, errpkg.ErrDuplicate) {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_bookmark_folder", "A folder with this name already exists")
			return
		}
		slog.Warn("update bookmark folder failed", "error", err, "user_id", user.UserID, "folder_id", id)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update bookmark folder")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": folderResponseFromModel(folder)})
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

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.service.DeleteFolder(r.Context(), id, tenantID, user.UserID); err != nil {
		if errors.Is(err, errpkg.ErrNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Bookmark folder not found")
			return
		}

		slog.Warn("delete bookmark folder failed", "error", err, "user_id", user.UserID, "folder_id", id)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete bookmark folder")
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// parseUUIDParam extracts a chi URL param and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(chi.URLParam(r, name))

	return id, err
}
