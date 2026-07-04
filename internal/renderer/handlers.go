package renderer

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for renderer HTTP handlers.
type Handlers struct {
	Renderer *Renderer
}

// NewHandlers creates Handlers with the given Renderer.
func NewHandlers(r *Renderer) *Handlers {
	return &Handlers{Renderer: r}
}

// Render handles POST /api/v1/content/render.
// Request body: {"content_id": "...", "updated_at": "...", "json": {...}}
// Response: {"data": {"html": "..."}}.
func (h *Handlers) Render(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	var input struct {
		ContentID string          `json:"content_id"`
		UpdatedAt string          `json:"updated_at"`
		JSON      json.RawMessage `json:"json"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id format")
		return
	}

	var updatedAt pgtype.Timestamptz
	if err := updatedAt.Scan(input.UpdatedAt); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_updated_at", "Invalid updated_at format")
		return
	}

	html, err := h.Renderer.Render(r.Context(), contentID, updatedAt, input.JSON)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "render_failed", "Failed to render content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"html": html,
		},
	})
}

// writeJSON serializes v as JSON and writes it to the response.
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
