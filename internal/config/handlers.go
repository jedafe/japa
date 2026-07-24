package config

import (
	"log/slog"
	"net/http"

	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for config HTTP handlers.
type Handlers struct {
	Settings *Settings
}

// NewHandlers creates a new Handlers with the given Settings.
func NewHandlers(settings *Settings) *Handlers {
	return &Handlers{Settings: settings}
}

// GetPublicSettings handles GET /site/settings/public.
// Returns all settings marked as public for the current tenant.
func (h *Handlers) GetPublicSettings(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	settings, err := h.Settings.GetPublicSettings(r.Context(), t.ID)
	if err != nil {
		slog.Error("config: get public settings", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch public settings")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
}
