package spaces

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Permission Preset Handlers
// ---------------------------------------------------------------------------

// ListPermissionPresets handles GET /api/v1/space-permission-presets.
func (h *Handlers) ListPermissionPresets(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	presets, err := h.Service.ListPermissionPresets(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": presets})
}

// UpdatePermissionPreset handles PUT /api/v1/space-permission-presets/{name}.
// The preset name is taken from the URL path; the five axis values come from
// the request body.
func (h *Handlers) UpdatePermissionPreset(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "Preset name is required")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	existing, err := h.Service.GetPermissionPreset(r.Context(), name)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var req struct {
		Visibility     string `json:"visibility"`
		PostingPerm    string `json:"posting_perm"`
		ReplyingPerm   string `json:"replying_perm"`
		CommentingPerm string `json:"commenting_perm"`
		JoiningPerm    string `json:"joining_perm"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	axes := SpacePermAxes{
		Visibility: req.Visibility,
		Posting:    req.PostingPerm,
		Replying:   req.ReplyingPerm,
		Commenting: req.CommentingPerm,
		Joining:    req.JoiningPerm,
	}
	if !ValidPermissionAxes(axes) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_permission_axes", "One or more permission axes are invalid")
		return
	}

	preset, err := h.Service.UpdatePermissionPreset(r.Context(), gen.UpdatePermissionPresetParams{
		Visibility:     req.Visibility,
		PostingPerm:    req.PostingPerm,
		ReplyingPerm:   req.ReplyingPerm,
		CommentingPerm: req.CommentingPerm,
		JoiningPerm:    req.JoiningPerm,
		ID:             existing.ID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": preset})
}
