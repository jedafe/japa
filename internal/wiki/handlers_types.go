// Package wiki — handlers_types.go provides the HTTP handlers for wiki
// type & category template administration (§80 Wiki Types & Field-Group
// Pages amendment, migration 000739).
//
// Routes:
//
//	GET    /api/v1/wiki/types                                  — enabled wiki types (public)
//	GET    /api/v1/admin/wiki/types                            — types + parent edges (wiki.manage)
//	POST   /api/v1/admin/wiki/types                            — create/enable a wiki type (wiki.manage)
//	PATCH  /api/v1/admin/wiki/types/:space_type_id             — update type config (wiki.manage)
//	DELETE /api/v1/admin/wiki/types/:space_type_id             — remove; 409 while pages exist (wiki.manage)
//	GET    /api/v1/admin/wiki/types/:space_type_id/categories  — category template (wiki.manage)
//	POST   /api/v1/admin/wiki/types/:space_type_id/categories  — add category (wiki.manage)
//	PATCH  /api/v1/admin/wiki/categories/:id                   — label/group/position (wiki.manage)
//	DELETE /api/v1/admin/wiki/categories/:id                   — non-system; 409 while prose exists
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
package wiki

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// HandleListWikiTypes handles GET /wiki/types — enabled wiki types for
// public navigation and destination filters.
func (h *Handlers) HandleListWikiTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.service.ListEnabledWikiTypes(r.Context())
	if err != nil {
		slog.Error("wiki: list types failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "types_failed", "Failed to list wiki types")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// HandleAdminListWikiTypes handles GET /admin/wiki/types — all wiki types
// with their parent edges (wiki.manage).
func (h *Handlers) HandleAdminListWikiTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.service.AdminListWikiTypes(r.Context())
	if err != nil {
		slog.Error("wiki: admin list types failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "types_failed", "Failed to list wiki types")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// HandleUpsertWikiType handles POST /admin/wiki/types — create or re-enable
// a wiki type for a space type, with its parent edges (wiki.manage).
func (h *Handlers) HandleUpsertWikiType(w http.ResponseWriter, r *http.Request) {
	var req UpsertWikiTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	t, err := h.service.UpsertWikiType(r.Context(), req)
	if err != nil {
		writeWikiTypeErr(w, err, "upsert type")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": t})
}

// HandleUpdateWikiType handles PATCH /admin/wiki/types/{space_type_id} —
// optional-field update; parent_type_ids replaces edges when present.
func (h *Handlers) HandleUpdateWikiType(w http.ResponseWriter, r *http.Request) {
	var req UpdateWikiTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if err := h.service.UpdateWikiType(r.Context(), chi.URLParam(r, "space_type_id"), req); err != nil {
		writeWikiTypeErr(w, err, "update type")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}

// HandleDeleteWikiType handles DELETE /admin/wiki/types/{space_type_id} —
// removes the wiki type; blocked while spaces of the type have active wiki
// pages.
func (h *Handlers) HandleDeleteWikiType(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteWikiType(r.Context(), chi.URLParam(r, "space_type_id")); err != nil {
		writeWikiTypeErr(w, err, "delete type")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleAdminListTypeCategories handles
// GET /admin/wiki/types/{space_type_id}/categories — the type's category
// template with bound field-group identities.
func (h *Handlers) HandleAdminListTypeCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.service.ListWikiTypeCategories(r.Context(), chi.URLParam(r, "space_type_id"))
	if err != nil {
		writeWikiTypeErr(w, err, "list categories")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// HandleCreateWikiCategory handles
// POST /admin/wiki/types/{space_type_id}/categories — adds a category to the
// type's template; an optional field_group_id binds the shared schema.
func (h *Handlers) HandleCreateWikiCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateWikiCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	c, err := h.service.CreateWikiTypeCategory(r.Context(), chi.URLParam(r, "space_type_id"), req)
	if err != nil {
		writeWikiTypeErr(w, err, "create category")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": c})
}

// HandleUpdateWikiCategory handles PATCH /admin/wiki/categories/{id} —
// label, bound group, and position are editable; slug/category are not.
func (h *Handlers) HandleUpdateWikiCategory(w http.ResponseWriter, r *http.Request) {
	var req UpdateWikiCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if err := h.service.UpdateWikiTypeCategory(r.Context(), chi.URLParam(r, "id"), req); err != nil {
		writeWikiTypeErr(w, err, "update category")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}

// HandleDeleteWikiCategory handles DELETE /admin/wiki/categories/{id} —
// removes a non-system category; blocked while its pages carry prose.
func (h *Handlers) HandleDeleteWikiCategory(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteWikiTypeCategory(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeWikiTypeErr(w, err, "delete category")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeWikiTypeErr maps wiki type/category service errors to the standard
// error envelope (Rule 5 — snake_case codes).
func writeWikiTypeErr(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, ErrWikiTypeNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "wiki_type_not_found", "Wiki type not found")
	case errors.Is(err, ErrWikiTypeInUse):
		errpkg.WriteError(w, http.StatusConflict, "wiki_type_in_use", "Spaces of this type still have active wiki pages")
	case errors.Is(err, ErrWikiCategoryNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "wiki_category_not_found", "Wiki category not found")
	case errors.Is(err, ErrWikiCategoryProtected):
		errpkg.WriteError(w, http.StatusConflict, "wiki_category_protected", "System categories cannot be deleted")
	case errors.Is(err, ErrWikiCategoryInUse):
		errpkg.WriteError(w, http.StatusConflict, "wiki_category_in_use", "Pages in this category still have content")
	case errors.Is(err, ErrFieldGroupNotFound):
		errpkg.WriteError(w, http.StatusBadRequest, "field_group_not_found", "Field group not found")
	default:
		slog.Error("wiki: "+op+" failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Operation failed")
	}
}
