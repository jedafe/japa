package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jedafe/japa/internal/tenant"
)

// WebhookHandler handles incoming webhooks from payment providers.
type WebhookHandler struct {
	Service *Service
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(service *Service) *WebhookHandler {
	return &WebhookHandler{Service: service}
}

// HandlePaystackWebhook handles POST /api/v1/payments/webhook/paystack.
func (h *WebhookHandler) HandlePaystackWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, ProviderPaystack)
}

// HandleStripeWebhook handles POST /api/v1/payments/webhook/stripe.
func (h *WebhookHandler) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, ProviderStripe)
}

// handleWebhook processes a webhook from any provider.
func (h *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request, provider string) {
	// Read raw body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse event
	var event map[string]interface{}
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}

	// Extract event ID — Paystack uses numeric IDs.
	var eventID string
	switch v := event["id"].(type) {
	case string:
		eventID = v
	case float64:
		eventID = fmt.Sprintf("%.0f", v)
	default:
		writeError(w, http.StatusBadRequest, "missing_event_id", "Missing event ID")
		return
	}
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "missing_event_id", "Missing event ID")
		return
	}

	// Get tenant from context
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Create webhook event
	webhookEvent := WebhookEvent{
		Provider:   provider,
		EventID:    eventID,
		Payload:    event,
		RawPayload: body,
	}

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), t.ID, provider, webhookEvent); err != nil {
		writeError(w, http.StatusInternalServerError, "webhook_error", "Failed to process webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}