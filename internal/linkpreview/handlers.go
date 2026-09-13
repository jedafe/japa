package linkpreview

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds dependencies for link preview HTTP handlers.
type Handlers struct {
	Fetcher *Fetcher
}

// NewHandlers creates Handlers with the given Fetcher.
func NewHandlers(fetcher *Fetcher) *Handlers {
	return &Handlers{Fetcher: fetcher}
}

// Fetch handles GET /api/v1/link-preview?url=...
func (h *Handlers) Fetch(w http.ResponseWriter, r *http.Request) {
	// Require auth
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Get URL from query
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_url", "url query parameter is required")
		return
	}

	// Fetch preview
	preview, err := h.Fetcher.Fetch(r.Context(), rawURL, pgtype.UUID{})
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "fetch_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": preview,
	})
}
