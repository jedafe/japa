package content

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ListReactionTypes handles GET /api/v1/reaction-types.
func (h *Handlers) ListReactionTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	types, err := h.Service.ListReactionTypes(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// CreateReactionType handles POST /api/v1/reaction-types.
func (h *Handlers) CreateReactionType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Key      string `json:"key"`
		Emoji    string `json:"emoji"`
		Label    string `json:"label"`
		Enabled  bool   `json:"enabled"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}

	if input.Emoji == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_emoji", "Emoji is required")
		return
	}

	if input.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_label", "Label is required")
		return
	}

	rt, err := h.Service.CreateReactionType(r.Context(), input.Key, input.Emoji, input.Label, input.Enabled, input.Position)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": rt})
}

// UpdateReactionType handles PATCH /api/v1/reaction-types/{key}.
func (h *Handlers) UpdateReactionType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}

	var input struct {
		Emoji    string `json:"emoji"`
		Label    string `json:"label"`
		Enabled  bool   `json:"enabled"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Emoji == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_emoji", "Emoji is required")
		return
	}

	if input.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_label", "Label is required")
		return
	}

	rt, err := h.Service.UpdateReactionType(r.Context(), key, input.Emoji, input.Label, input.Enabled, input.Position)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rt})
}

// DeleteReactionType handles DELETE /api/v1/reaction-types/{key}.
func (h *Handlers) DeleteReactionType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}

	if err := h.Service.DeleteReactionType(r.Context(), key); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
