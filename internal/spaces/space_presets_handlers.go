package spaces

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Permission Preset Handlers
// ---------------------------------------------------------------------------

// ListPermissionPresets handles GET /api/v1/space-permission-presets.
func (h *Handlers) ListPermissionPresets(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	presets, err := h.Service.ListPermissionPresets(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": presets})
}

// UpdatePermissionPreset handles PUT /api/v1/space-permission-presets/{name}.
// The preset name is taken from the URL path; the five axis values come from
// the request body.
func (h *Handlers) UpdatePermissionPreset(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Preset name is required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	existing, err := h.Service.GetPermissionPreset(r.Context(), name, tenantID)
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
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
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "One or more permission axes are invalid")
		return
	}

	preset, err := h.Service.UpdatePermissionPreset(r.Context(), gen.UpdatePermissionPresetParams{
		Visibility:     req.Visibility,
		PostingPerm:    req.PostingPerm,
		ReplyingPerm:   req.ReplyingPerm,
		CommentingPerm: req.CommentingPerm,
		JoiningPerm:    req.JoiningPerm,
		ID:             existing.ID,
		TenantID:       tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": preset})
}
