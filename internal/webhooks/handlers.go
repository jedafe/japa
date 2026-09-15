// Package webhooks — handlers.go provides HTTP handlers for webhook admin CRUD.
//
// Routes:
//
//	GET    /admin/webhooks              — list all registered webhooks
//	GET    /admin/webhooks/{id}         — get webhook by ID
//	POST   /admin/webhooks              — register a new webhook endpoint
//	PUT    /admin/webhooks/{id}         — update an existing webhook endpoint
//	PATCH  /admin/webhooks/{id}         — partial update
//	DELETE /admin/webhooks/{id}         — delete a webhook endpoint
//	POST   /admin/webhooks/{id}/test    — send a test event
//	POST   /admin/webhooks/{id}/verify  — challenge/response verification (§12)
//	GET    /admin/webhooks/{id}/deliveries — delivery log, paginated
//	POST   /admin/webhooks/{id}/deliveries/{delivery_id}/retry — retry failed delivery
//	GET    /webhook-events              — available event types
package webhooks

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
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

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case strings.Contains(err.Error(), "not verified"):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "endpoint_not_verified", err.Error())
	case strings.Contains(err.Error(), "not found"):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "already"):
		errpkg.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "challenge/response") || strings.Contains(err.Error(), "challenge"):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "verification_failed", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
	}
}


// requireUser extracts the authenticated user ID from the request context.
// Returns false (and writes a 401) if no authenticated user is present.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	user := auth.FromContext(r.Context())
	if user == nil || !user.UserID.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}

	return user.UserID, true
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

	endpoints, err := h.Service.ListEndpoints(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        endpoints,
		"total_count": len(endpoints),
	})
}

// GetWebhook returns a single webhook endpoint by ID.
// GET /admin/webhooks/{id}.
func (h *Handlers) GetWebhook(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	endpoint, err := h.Service.GetEndpoint(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": endpoint})
}

// CreateWebhook registers a new webhook endpoint. The signing secret is
// server-generated and returned exactly once in the response (§12).
// POST /admin/webhooks.
func (h *Handlers) CreateWebhook(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input CreateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	if input.URL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "url is required")
		return
	}

	if len(input.EventTypes) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "event_types is required and must not be empty")
		return
	}

	endpoint, err := h.Service.CreateEndpoint(r.Context(), userID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// §12: return the secret exactly once on create.
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": endpoint.ToCreateResponse()})
}

// UpdateWebhook updates an existing webhook endpoint.
// PUT/PATCH /admin/webhooks/{id}.
func (h *Handlers) UpdateWebhook(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	var input UpdateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	endpoint, err := h.Service.UpdateEndpoint(r.Context(), id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": endpoint})
}

// DeleteWebhook removes a webhook endpoint.
// DELETE /admin/webhooks/{id}.
func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	if err := h.Service.DeleteEndpoint(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// TestWebhook sends a test event to a webhook endpoint.
// POST /admin/webhooks/{id}/test.
func (h *Handlers) TestWebhook(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	var input TestWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.EventType == "" {
		input.EventType = "webhook.test"
	}

	deliveryID, err := h.Service.TestEndpoint(r.Context(), id, input.EventType)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"delivery_id": deliveryID.String(),
			"status":      "pending",
		},
	})
}

// VerifyWebhook performs the §12 challenge/response verification. Sends a
// GET to {url}?challenge={token} and verifies the endpoint echoes the token
// back. On success, marks the endpoint as verified and active.
// POST /admin/webhooks/{id}/verify.
func (h *Handlers) VerifyWebhook(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	if err := h.Service.VerifyEndpoint(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"status": "verified"},
	})
}

// ListDeliveries handles GET /admin/webhooks/{id}/deliveries.
func (h *Handlers) ListDeliveries(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	deliveries, err := h.Service.ListDeliveries(r.Context(), id, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": deliveries})
}

// RetryDelivery handles POST /admin/webhooks/{id}/deliveries/{delivery_id}/retry.
// It verifies that the delivery belongs to the webhook endpoint {id} in the
// URL path before retrying (BUG-025: previously ignored {id}, allowing a
// user to retry a delivery from any webhook by guessing the delivery_id).
func (h *Handlers) RetryDelivery(w http.ResponseWriter, r *http.Request) {

	endpointID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	deliveryID, err := parseUUIDParam(r, "delivery_id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid delivery ID")
		return
	}

	if err := h.Service.RetryDelivery(r.Context(), endpointID, deliveryID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "retried"},
	})
}

// webhookEventCatalog is the §12 default event catalog scoped to non-PII
// business events. The spec lists 9 core events; this catalog includes those
// plus a reviewed set of additional non-PII business events. User/DM/moderation
// events that carry PII are intentionally excluded (BUG-008).
//
// Tenants can override this via the `webhooks.event_catalog` system setting
// (BUG-009), which is a JSON array of {key,label,description} objects.
//
// Ref: japaconnect-engineering/sections/12-webhooks.md.
var defaultWebhookEventCatalog = []map[string]string{
	// Content (§14)
	{"key": "content.created", "label": "Content Created", "description": "When content is first created (draft or published)"},
	{"key": "content.published", "label": "Content Published", "description": "When content is published"},
	{"key": "content.updated", "label": "Content Updated", "description": "When content is updated"},
	{"key": "content.deleted", "label": "Content Deleted", "description": "When content is soft-deleted"},
	// Claims (§07)
	{"key": "claim.approved", "label": "Claim Approved", "description": "When an ownership claim is approved"},
	{"key": "claim.rejected", "label": "Claim Rejected", "description": "When an ownership claim is rejected"},
	// Reviews (§29)
	{"key": "review.created", "label": "Review Created", "description": "When a new review is created"},
	// Organizations (§07)
	{"key": "organization.verified", "label": "Organization Verified", "description": "When an organization is verified"},
	{"key": "organization.unverified", "label": "Organization Unverified", "description": "When an organization's verification is revoked"},
	// Payments (§17)
	{"key": "payment.succeeded", "label": "Payment Succeeded", "description": "When a payment succeeds"},
	{"key": "payment.failed", "label": "Payment Failed", "description": "When a payment fails"},
	// Forum (§08) — non-PII
	{"key": "forum.thread.created", "label": "Forum Thread Created", "description": "When a new forum thread is created"},
	{"key": "forum.thread.updated", "label": "Forum Thread Updated", "description": "When a forum thread is updated"},
	{"key": "forum.thread.deleted", "label": "Forum Thread Deleted", "description": "When a forum thread is deleted"},
	{"key": "forum.thread.solved", "label": "Forum Thread Solved", "description": "When a forum thread is marked as solved"},
	{"key": "forum.reply.created", "label": "Forum Reply Created", "description": "When a new reply is posted in a forum thread"},
	{"key": "forum.reply.updated", "label": "Forum Reply Updated", "description": "When a forum reply is updated"},
	{"key": "forum.reply.deleted", "label": "Forum Reply Deleted", "description": "When a forum reply is deleted"},
	{"key": "forum.vote.cast", "label": "Forum Vote Cast", "description": "When a vote is cast on a forum entity"},
	// AMA (§88) — non-PII
	{"key": "forum.ama.opened", "label": "AMA Opened", "description": "When an Ask-Me-Anything session is opened"},
}

// eventCatalogEntry is a single entry in the webhook event catalog.
type eventCatalogEntry struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// ListWebhookEvents handles GET /webhook-events — returns available webhook
// event types. Scoped to non-PII business events per §12 (BUG-008). The
// catalog can be overridden per-tenant via the `webhooks.event_catalog`
// system setting (BUG-009).
func (h *Handlers) ListWebhookEvents(w http.ResponseWriter, r *http.Request) {
	entries := defaultWebhookEventEntries()

	// Check for a tenant-specific override via the webhooks.event_catalog
	// system setting (BUG-009). If the setting is empty or invalid, fall
	// back to the built-in default catalog.
	if h.Service != nil && h.Service.settings != nil {
		raw := h.Service.settings.GetStringOr(r.Context(), "webhooks.event_catalog", "")
		if raw != "" {
			var custom []eventCatalogEntry
			if err := json.Unmarshal([]byte(raw), &custom); err == nil && len(custom) > 0 {
				data := make([]map[string]string, 0, len(custom))
				for _, e := range custom {
					data = append(data, map[string]string{
						"key":         e.Key,
						"label":       e.Label,
						"description": e.Description,
					})
				}

				errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})

				return
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": entries})
}

// defaultWebhookEventEntries returns the built-in default catalog as a slice
// of maps for JSON serialization.
func defaultWebhookEventEntries() []map[string]string {
	out := make([]map[string]string, len(defaultWebhookEventCatalog))
	copy(out, defaultWebhookEventCatalog)

	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parsePageSize(r *http.Request) int32 {
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			return int32(n) // #nosec
		}
	}

	return 20
}

func parsePageSizeOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return int32(n) // #nosec
		}
	}

	return 0
}
