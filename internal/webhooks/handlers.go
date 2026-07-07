// Package webhooks — handlers.go provides HTTP handlers for webhook admin CRUD.
//
// Routes:
//
//	GET    /admin/webhooks              — list all registered webhooks
//	GET    /admin/webhooks/{id}         — get webhook by ID
//	POST   /admin/webhooks              — register a new webhook endpoint
//	PUT    /admin/webhooks/{id}         — update an existing webhook endpoint
//	DELETE /admin/webhooks/{id}         — delete a webhook endpoint
//	POST   /admin/webhooks/{id}/test    — send a test event
package webhooks

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for webhook admin HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case contains(err.Error(), "not found"):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case contains(err.Error(), "already"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// ---------------------------------------------------------------------------
// Admin CRUD Handlers
// ---------------------------------------------------------------------------

// ListWebhooks returns all webhook endpoints for the tenant.
// GET /admin/webhooks.
func (h *Handlers) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	endpoints, err := h.Service.ListEndpoints(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":        endpoints,
		"total_count": len(endpoints),
	})
}

// GetWebhook returns a single webhook endpoint by ID.
// GET /admin/webhooks/{id}.
func (h *Handlers) GetWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	endpoint, err := h.Service.GetEndpoint(r.Context(), tenantID, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": endpoint})
}

// CreateWebhook registers a new webhook endpoint.
// POST /admin/webhooks.
func (h *Handlers) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.URL == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "url is required")
		return
	}

	if input.Secret == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "secret is required")
		return
	}

	if len(input.EventTypes) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "event_types is required and must not be empty")
		return
	}

	endpoint, err := h.Service.CreateEndpoint(r.Context(), tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": endpoint})
}

// UpdateWebhook updates an existing webhook endpoint.
// PUT /admin/webhooks/{id}.
func (h *Handlers) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	var input UpdateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	endpoint, err := h.Service.UpdateEndpoint(r.Context(), tenantID, id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": endpoint})
}

// DeleteWebhook removes a webhook endpoint.
// DELETE /admin/webhooks/{id}.
func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	if err := h.Service.DeleteEndpoint(r.Context(), tenantID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// TestWebhook sends a test event to a webhook endpoint.
// POST /admin/webhooks/{id}/test.
func (h *Handlers) TestWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	var input TestWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.EventType == "" {
		input.EventType = "webhook.test"
	}

	deliveryID, err := h.Service.TestEndpoint(r.Context(), tenantID, id, input.EventType)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"delivery_id": deliveryID.String(),
			"status":      "pending",
		},
	})
}

// ListDeliveries handles GET /admin/webhooks/{id}/deliveries.
func (h *Handlers) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	deliveries, err := h.Service.ListDeliveries(r.Context(), tenantID, id, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": deliveries})
}

// RetryDelivery handles POST /admin/webhooks/{id}/retry/{delivery_id}.
func (h *Handlers) RetryDelivery(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	deliveryID, err := parseUUIDParam(r, "delivery_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid delivery ID")
		return
	}

	if err := h.Service.RetryDelivery(r.Context(), tenantID, deliveryID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "retried"},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parsePageSize(r *http.Request) int32 {
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			return int32(n)
		}
	}
	return 20
}

func parsePageSizeOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("page_token"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return int32(n)
		}
	}
	return 0
}
