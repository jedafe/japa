package bookmarkfolders

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// Handlers holds dependencies for bookmark folder HTTP handlers.
type Handlers struct {
	service *Service
	lit     literalResolver // nil disables literal resolution (§14)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// SetLiteralResolver injects the template-literal catalog so user-facing
// titles/descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a response DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
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
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	folders, err := h.service.ListFolders(r.Context(), user.UserID)
	if err != nil {
		slog.Warn("list bookmark folders failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list bookmark folders")

		return
	}

	h.resolveLiterals(r, &folders)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": folders})
}

// CreateFolder handles POST /api/v1/me/bookmark-folders.
func (h *Handlers) CreateFolder(w http.ResponseWriter, r *http.Request) {
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

	folder, err := h.service.CreateFolder(r.Context(), user.UserID, input.Name)
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

	folder, err := h.service.UpdateFolder(r.Context(), id, user.UserID, input.Name)
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
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.service.DeleteFolder(r.Context(), id, user.UserID); err != nil {
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

// ReorderFolders handles PATCH /api/v1/me/bookmark-folders/reorder.
// Body: { "folder_ids": ["uuid", "uuid", ...] } — the array order defines
// the new position (0-indexed). Folders not in the list keep their current
// position. Unknown/non-owned folder IDs are silently skipped (the UPDATE
// is platform+owner scoped). BUG-008: §08 spec supports manual folder ordering.
func (h *Handlers) ReorderFolders(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		FolderIDs []string `json:"folder_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.FolderIDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_folder_ids", "folder_ids is required and must be non-empty")
		return
	}

	folderIDs := make([]pgtype.UUID, 0, len(input.FolderIDs))

	for _, fidStr := range input.FolderIDs {
		var fid pgtype.UUID
		if err := fid.Scan(fidStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_folder_id", "Invalid folder_id format")
			return
		}

		folderIDs = append(folderIDs, fid)
	}

	if err := h.service.ReorderFolders(r.Context(), user.UserID, folderIDs); err != nil {
		slog.Warn("reorder bookmark folders failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder bookmark folders")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"reordered": true}})
}
