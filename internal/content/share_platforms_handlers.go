package content

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Share Platform Handlers (admin-configurable social media sharing)
// ---------------------------------------------------------------------------

// ListSharePlatforms handles GET /api/v1/share-platforms (admin: all platforms).
func (h *Handlers) ListSharePlatforms(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	platforms, err := h.Service.ListSharePlatforms(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": platforms})
}

// ListEnabledSharePlatforms handles GET /api/v1/share-platforms/enabled (public).
// Returns only enabled platforms for the share dropdown.
func (h *Handlers) ListEnabledSharePlatforms(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	platforms, err := h.Service.ListEnabledSharePlatforms(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": platforms})
}

// CreateSharePlatform handles POST /api/v1/share-platforms.
func (h *Handlers) CreateSharePlatform(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Key         string `json:"key"`
		Label       string `json:"label"`
		Icon        string `json:"icon"`
		UrlTemplate string `json:"url_template"`
		Color       string `json:"color"`
		Enabled     bool   `json:"enabled"`
		Position    int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}

	if input.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_label", "Label is required")
		return
	}

	if input.UrlTemplate == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_url_template", "url_template is required")
		return
	}

	p, err := h.Service.CreateSharePlatform(r.Context(), input.Key, input.Label, input.Icon, input.UrlTemplate, input.Color, input.Enabled, input.Position)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": p})
}

// UpdateSharePlatform handles PATCH /api/v1/share-platforms/{key}.
func (h *Handlers) UpdateSharePlatform(w http.ResponseWriter, r *http.Request) {
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
		Label       string `json:"label"`
		Icon        string `json:"icon"`
		UrlTemplate string `json:"url_template"`
		Color       string `json:"color"`
		Enabled     bool   `json:"enabled"`
		Position    int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_label", "Label is required")
		return
	}

	if input.UrlTemplate == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_url_template", "url_template is required")
		return
	}

	p, err := h.Service.UpdateSharePlatform(r.Context(), key, input.Label, input.Icon, input.UrlTemplate, input.Color, input.Enabled, input.Position)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": p})
}

// DeleteSharePlatform handles DELETE /api/v1/share-platforms/{key}.
func (h *Handlers) DeleteSharePlatform(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}

	if err := h.Service.DeleteSharePlatform(r.Context(), key); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
