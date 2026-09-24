package publicapi

import (
	"net/http"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// GetDigest handles GET /api/v1/public/me/digest — the aggregate new-tab
// payload for the key owner (api.read.digest scope, enforced by the route).
func (h *Handlers) GetDigest(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	keyCtx := APIKeyFromContext(r.Context())

	digest, err := h.Service.Digest(r.Context(), keyCtx.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": digest})
}
