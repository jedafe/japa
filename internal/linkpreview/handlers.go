package linkpreview

import (
	"net/http"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
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
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Require tenant
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Get URL from query
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_url", "url query parameter is required")
		return
	}

	// Fetch preview
	preview, err := h.Fetcher.Fetch(r.Context(), rawURL)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "fetch_failed", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": preview,
	})
}
