package content

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ListReactionTypes handles GET /api/v1/reaction-types.
func (h *Handlers) ListReactionTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.Service.ListReactionTypes(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}

// CreateReactionType handles POST /api/v1/reaction-types.
func (h *Handlers) CreateReactionType(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Key      string `json:"key"`
		Emoji    string `json:"emoji"`
		Label    string `json:"label"`
		Enabled  bool   `json:"enabled"`
		Position int32  `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}
	if input.Emoji == "" {
		writeError(w, http.StatusBadRequest, "missing_emoji", "Emoji is required")
		return
	}
	if input.Label == "" {
		writeError(w, http.StatusBadRequest, "missing_label", "Label is required")
		return
	}

	rt, err := h.Service.CreateReactionType(r.Context(), input.Key, input.Emoji, input.Label, input.Enabled, input.Position)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": rt})
}

// UpdateReactionType handles PATCH /api/v1/reaction-types/{key}.
func (h *Handlers) UpdateReactionType(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}

	var input struct {
		Emoji    string `json:"emoji"`
		Label    string `json:"label"`
		Enabled  bool   `json:"enabled"`
		Position int32  `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Emoji == "" {
		writeError(w, http.StatusBadRequest, "missing_emoji", "Emoji is required")
		return
	}
	if input.Label == "" {
		writeError(w, http.StatusBadRequest, "missing_label", "Label is required")
		return
	}

	rt, err := h.Service.UpdateReactionType(r.Context(), key, input.Emoji, input.Label, input.Enabled, input.Position)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rt})
}

// DeleteReactionType handles DELETE /api/v1/reaction-types/{key}.
func (h *Handlers) DeleteReactionType(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}
	if err := h.Service.DeleteReactionType(r.Context(), key); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// Ensure strconv import is used (position parsing placeholder for future use).
var _ = strconv.Atoi