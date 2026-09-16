package config

import (
	"log/slog"
	"net/http"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds HTTP handlers for public config endpoints.
type Handlers struct {
	Settings *Settings
}

// NewHandlers creates a new Handlers with the given Settings.
func NewHandlers(settings *Settings) *Handlers {
	return &Handlers{Settings: settings}
}

// GetPublicSettings handles GET /site/settings/public.
// Returns all settings marked as public for the current platform.
func (h *Handlers) GetPublicSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Settings.GetPublicSettings(r.Context())
	if err != nil {
		slog.Error("config: get public settings", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch public settings")

		return
	}

	w.Header().Set("Cache-Control", "private, max-age=60")
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
}
