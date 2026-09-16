// Package content — HTTP handler for the page builder block type registry.
//
// The block-types endpoint is available to any authenticated user (no
// special permission required) because it returns a static registry, not
// platform data. The front-end calls this when the page builder editor
// mounts to populate the block picker.
package content

import (
	"net/http"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// ListBlockTypes handles GET /api/v1/page-builder/block-types.
// It returns the full block template registry.
func (h *Handlers) ListBlockTypes(w http.ResponseWriter, _ *http.Request) {
	templates := GetBlockTemplates()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"block_types": templates,
		},
	})
}
