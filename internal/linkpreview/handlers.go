package linkpreview

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jedafe/japa/internal/auth"
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
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Require tenant
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Get URL from query
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "missing_url", "url query parameter is required")
		return
	}

	// Fetch preview
	preview, err := h.Fetcher.Fetch(r.Context(), rawURL, t.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "fetch_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": preview,
	})
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
