// Package ai — tool_handlers.go provides HTTP handlers for AI tool registry admin.
//
// Routes:
//
//	GET /admin/ai/tools           — list all registered AI tools
//	GET /admin/ai/tools/{id}      — get AI tool by ID
//	PUT /admin/ai/tools/{id}      — update AI tool (enable/disable, config)
//	GET /admin/ai/tools/usage     — usage stats for a tool
package ai

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/tenant"
)

// ToolHandlers holds dependencies for AI tool admin HTTP handlers.
type ToolHandlers struct {
	Registry *ToolRegistry
}

// NewToolHandlers creates ToolHandlers with the given registry.
func NewToolHandlers(registry *ToolRegistry) *ToolHandlers {
	return &ToolHandlers{Registry: registry}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func toolWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func toolWriteError(w http.ResponseWriter, status int, code, message string) {
	toolWriteJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (h *ToolHandlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		toolWriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// ---------------------------------------------------------------------------
// Admin Handlers — AI Tool Registry
// ---------------------------------------------------------------------------

// ListTools returns all AI tools for the tenant.
// GET /admin/ai/tools.
func (h *ToolHandlers) ListTools(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tools, err := h.Registry.ListTools(r.Context(), tenantID)
	if err != nil {
		slog.Error("listing ai tools", "error", err)
		toolWriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list tools")

		return
	}

	toolWriteJSON(w, http.StatusOK, map[string]any{
		"data":        tools,
		"total_count": len(tools),
	})
}

// GetTool returns a single AI tool by ID.
// GET /admin/ai/tools/{id}.
func (h *ToolHandlers) GetTool(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		toolWriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tool ID")
		return
	}

	tool, err := h.Registry.GetTool(r.Context(), tenantID, id)
	if err != nil {
		toolWriteError(w, http.StatusNotFound, "not_found", "Tool not found")
		return
	}

	toolWriteJSON(w, http.StatusOK, map[string]any{"data": tool})
}

// UpdateTool updates an existing AI tool's configuration.
// PUT /admin/ai/tools/{id}.
func (h *ToolHandlers) UpdateTool(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		toolWriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tool ID")
		return
	}

	var input struct {
		Description        *string         `json:"description,omitempty"`
		Config             json.RawMessage `json:"config,omitempty"`
		IsEnabled          *bool           `json:"is_enabled,omitempty"`
		RequiredPermission *string         `json:"required_permission,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		toolWriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	tool, err := h.Registry.UpdateTool(r.Context(), tenantID, id, UpdateToolInput{
		Description:        input.Description,
		Config:             input.Config,
		IsEnabled:          input.IsEnabled,
		RequiredPermission: input.RequiredPermission,
	})
	if err != nil {
		slog.Error("updating ai tool", "error", err)
		toolWriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update tool")

		return
	}

	toolWriteJSON(w, http.StatusOK, map[string]any{"data": tool})
}

// ListToolUsage returns usage statistics for a tool.
// GET /admin/ai/tools/usage?tool_id=...&days=30.
func (h *ToolHandlers) ListToolUsage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	toolIDStr := r.URL.Query().Get("tool_id")
	if toolIDStr == "" {
		toolWriteError(w, http.StatusBadRequest, "validation_error", "tool_id query parameter is required")
		return
	}

	var toolID pgtype.UUID
	if err := toolID.Scan(toolIDStr); err != nil {
		toolWriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tool_id")
		return
	}

	days := 30

	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			days = n
		}
	}

	since := time.Now().AddDate(0, 0, -days)

	stats, err := h.Registry.GetToolUsageStats(r.Context(), tenantID, toolID, since)
	if err != nil {
		slog.Error("fetching tool usage stats", "error", err)
		toolWriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch usage stats")

		return
	}

	toolWriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"tool_id":     toolID.String(),
			"period_days": days,
			"stats":       stats,
		},
	})
}
