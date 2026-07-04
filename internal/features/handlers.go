package features

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for feature flag HTTP handlers.
type Handlers struct {
	checker *Checker
}

// NewHandlers creates a new Handlers with the given Checker.
func NewHandlers(checker *Checker) *Handlers {
	return &Handlers{checker: checker}
}

// HandleGetFeatures handles GET /api/v1/features.
// Returns all feature flags and their enabled state for the current tenant.
// This endpoint is public (no auth required) because the frontend layout
// needs flags before the user logs in to render navigation correctly.
func (h *Handlers) HandleGetFeatures(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	flags := h.checker.GetAllEnabled(r.Context(), t.ID)
	writeJSON(w, http.StatusOK, map[string]any{"data": flags})
}

// writeJSON serializes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("features: json encode error", "error", err)
	}
}

// writeError writes a JSON error response in the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
