package config

import (
	"encoding/json"
	"log/slog"
	"net/http"

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
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	settings, err := h.Settings.GetPublicSettings(r.Context(), t.ID)
	if err != nil {
		slog.Error("config: get public settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch public settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("config: json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
