package publicapi

// Phase 3 (spec 2026-09-24-agent-data-api-phase3): personal webhook
// subscriptions owned by the API-key holder. Endpoints are scope='user' rows
// on webhook_endpoints; they only ever receive content.published, and
// EmitEvent filters each event by owner-visible content before creating a
// delivery. Ref: japaconnect-engineering/sections/12-webhooks.md

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// userWebhookEventTypes is the Phase 3 allowlist — personal subscriptions
// only ever see content.published. Broadening requires widening the
// EmitEvent visibility filter to carry the relevant subject for each type.
var userWebhookEventTypes = map[string]bool{"content.published": true}

// ErrWebhookLimitReached is returned when the owner already has
// api.webhooks.max_per_key personal endpoints.
var ErrWebhookLimitReached = errors.New("webhook endpoint limit reached")

// PublicWebhookEndpoint is the wire shape for personal subscriptions.
// Secret is populated only on the create response — §12 "secret shown once".
type PublicWebhookEndpoint struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	URL        string     `json:"url"`
	EventTypes []string   `json:"event_types"`
	Status     string     `json:"status"`
	IsActive   bool       `json:"is_active"`
	VerifiedAt *time.Time `json:"verified_at"`
	CreatedAt  time.Time  `json:"created_at"`
	Secret     string     `json:"secret,omitempty"`
}

// PublicWebhookDelivery is the wire shape for a delivery audit row. The
// payload is omitted — the owner already knows what they subscribed to, and
// not echoing it keeps delivery logs lean.
type PublicWebhookDelivery struct {
	ID               string     `json:"id"`
	EventID          string     `json:"event_id"`
	EventType        string     `json:"event_type"`
	Status           string     `json:"status"`
	Attempts         int32      `json:"attempts"`
	LastError        *string    `json:"last_error,omitempty"`
	LastResponseCode *int32     `json:"last_response_code,omitempty"`
	NextAttemptAt    *time.Time `json:"next_attempt_at,omitempty"`
	DeliveredAt      *time.Time `json:"delivered_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// WebhookSubscriptionService is the narrow seam into internal/webhooks.
// Satisfied by an adapter in internal/server over *webhooks.Service.
type WebhookSubscriptionService interface {
	CreateUserEndpoint(ctx context.Context, ownerID pgtype.UUID, name, url string, eventTypes []string) (PublicWebhookEndpoint, error)
	ListUserEndpoints(ctx context.Context, ownerID pgtype.UUID) ([]PublicWebhookEndpoint, error)
	CountUserEndpoints(ctx context.Context, ownerID pgtype.UUID) (int64, error)
	// VerifyUserEndpoint runs challenge/response on an owner-scoped
	// endpoint; DeleteUserEndpoint and ListUserDeliveries return
	// errpkg.ErrNotFound for absent/foreign IDs.
	VerifyUserEndpoint(ctx context.Context, ownerID, id pgtype.UUID) error
	DeleteUserEndpoint(ctx context.Context, ownerID, id pgtype.UUID) error
	ListUserDeliveries(ctx context.Context, ownerID, endpointID pgtype.UUID, limit, offset int32) ([]PublicWebhookDelivery, error)
}

// SetWebhookSubscriptions injects the personal-webhook adapter.
func (h *Handlers) SetWebhookSubscriptions(svc WebhookSubscriptionService) {
	h.webhooks = svc
}

// CreateMyWebhook handles POST /api/v1/public/me/webhooks.
// Requires api.write.me. Secret is returned exactly once.
func (h *Handlers) CreateMyWebhook(w http.ResponseWriter, r *http.Request) {
	keyCtx, svc, ok := h.requireWebhookCtx(w, r)
	if !ok {
		return
	}

	var input struct {
		Name       string   `json:"name"`
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.URL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_url", "url is required")
		return
	}

	// Phase 3: personal subscriptions are restricted to content.published.
	// An absent event_types defaults to it; anything else is rejected.
	if len(input.EventTypes) == 0 {
		input.EventTypes = []string{"content.published"}
	}

	for _, t := range input.EventTypes {
		if !userWebhookEventTypes[t] {
			errpkg.WriteError(w, http.StatusBadRequest, "unsupported_event_type",
				"personal webhooks only support event_type \"content.published\"")
			return
		}
	}

	// api.webhooks.max_per_key cap (Rule 3 — settings-driven).
	maxPerKey := 5
	if h.Service != nil {
		maxPerKey = h.Service.GetIntSetting(r.Context(), "api.webhooks.max_per_key", maxPerKey)
	}

	count, err := svc.CountUserEndpoints(r.Context(), keyCtx.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if count >= int64(maxPerKey) {
		handleServiceError(w, ErrWebhookLimitReached)
		return
	}

	ep, err := svc.CreateUserEndpoint(r.Context(), keyCtx.UserID, input.Name, input.URL, input.EventTypes)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": ep})
}

// ListMyWebhooks handles GET /api/v1/public/me/webhooks.
// Requires api.read.me.
func (h *Handlers) ListMyWebhooks(w http.ResponseWriter, r *http.Request) {
	keyCtx, svc, ok := h.requireWebhookCtx(w, r)
	if !ok {
		return
	}

	eps, err := svc.ListUserEndpoints(r.Context(), keyCtx.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if eps == nil {
		eps = []PublicWebhookEndpoint{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": eps})
}

// VerifyMyWebhook handles POST /api/v1/public/me/webhooks/{id}/verify —
// challenge/response activation. Requires api.write.me.
func (h *Handlers) VerifyMyWebhook(w http.ResponseWriter, r *http.Request) {
	keyCtx, svc, ok := h.requireWebhookCtx(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	if err := svc.VerifyUserEndpoint(r.Context(), keyCtx.UserID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "verified"}})
}

// DeleteMyWebhook handles DELETE /api/v1/public/me/webhooks/{id}.
// Requires api.write.me.
func (h *Handlers) DeleteMyWebhook(w http.ResponseWriter, r *http.Request) {
	keyCtx, svc, ok := h.requireWebhookCtx(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	if err := svc.DeleteUserEndpoint(r.Context(), keyCtx.UserID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListMyWebhookDeliveries handles
// GET /api/v1/public/me/webhooks/{id}/deliveries. Requires api.read.me.
func (h *Handlers) ListMyWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	keyCtx, svc, ok := h.requireWebhookCtx(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)

	deliveries, err := svc.ListUserDeliveries(r.Context(), keyCtx.UserID, id, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if deliveries == nil {
		deliveries = []PublicWebhookDelivery{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": deliveries})
}

// requireWebhookCtx resolves the API-key context and the webhook service,
// writing the standard errors when either is absent.
func (h *Handlers) requireWebhookCtx(w http.ResponseWriter, r *http.Request) (*APIKeyContext, WebhookSubscriptionService, bool) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return nil, nil, false
	}

	if h.webhooks == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return nil, nil, false
	}

	return keyCtx, h.webhooks, true
}
