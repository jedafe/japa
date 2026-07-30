package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// MaxDeliveryAttempts is the roadmap-mandated retry count for webhook delivery.
const MaxDeliveryAttempts = 5

// webhooksSettingsReader is the subset of config.Settings webhooks needs.
type webhooksSettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// Service coordinates webhook endpoint lookup, signed delivery, and delivery
// status tracking. It implements the worker.WebhookProcessor interface.
type Service struct {
	store    *Store
	settings webhooksSettingsReader
	siteName string
	enqueuer webhookEnqueuer
}

// webhookEnqueuer is the subset of worker.Enqueuer that webhooks needs.
type webhookEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// SetSettings injects the hot-reloadable settings reader (AUDIT-BE-009).
func (s *Service) SetSettings(r webhooksSettingsReader) { s.settings = r }

// SetEnqueuer injects the Asynq enqueuer for webhook delivery tasks.
func (s *Service) SetEnqueuer(e webhookEnqueuer) { s.enqueuer = e }

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, siteName string) *Service {
	return &Service{store: NewStore(database), siteName: siteName}
}

// ---------------------------------------------------------------------------
// Admin CRUD — Webhook Endpoints
// ---------------------------------------------------------------------------

// ListEndpoints returns all webhook endpoints for the given tenant.
func (s *Service) ListEndpoints(ctx context.Context, tenantID pgtype.UUID) ([]WebhookEndpointResponse, error) {
	rows, err := s.store.ListEndpoints(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var endpoints []WebhookEndpointResponse
	for _, ep := range rows {
		endpoints = append(endpoints, ep.ToResponse())
	}

	return endpoints, nil
}

// GetEndpoint returns a single webhook endpoint by ID.
func (s *Service) GetEndpoint(ctx context.Context, tenantID, id pgtype.UUID) (WebhookEndpointResponse, error) {
	ep, err := s.store.GetEndpoint(ctx, tenantID, id)
	if err != nil {
		return WebhookEndpointResponse{}, err
	}
	return ep.ToResponse(), nil
}

// CreateEndpoint registers a new webhook endpoint.
func (s *Service) CreateEndpoint(ctx context.Context, tenantID pgtype.UUID, input CreateWebhookRequest) (WebhookEndpointResponse, error) {
	status := input.Status
	if status == "" {
		status = "active"
	}

	ep, err := s.store.CreateEndpoint(ctx, tenantID, input.URL, input.Secret, input.EventTypes, status)
	if err != nil {
		return WebhookEndpointResponse{}, err
	}

	return ep.ToResponse(), nil
}

// UpdateEndpoint updates an existing webhook endpoint.
func (s *Service) UpdateEndpoint(ctx context.Context, tenantID, id pgtype.UUID, input UpdateWebhookRequest) (WebhookEndpointResponse, error) {
	// Fetch current values for merge.
	current, err := s.store.GetEndpoint(ctx, tenantID, id)
	if err != nil {
		return WebhookEndpointResponse{}, err
	}

	url := current.URL
	if input.URL != nil {
		url = *input.URL
	}

	secret := current.Secret
	if input.Secret != nil {
		secret = *input.Secret
	}

	eventTypes := current.EventTypes
	if input.EventTypes != nil {
		eventTypes = input.EventTypes
	}

	status := current.Status
	if input.Status != nil {
		status = *input.Status
	}

	updated, err := s.store.UpdateEndpoint(ctx, tenantID, id, url, secret, eventTypes, status)
	if err != nil {
		return WebhookEndpointResponse{}, err
	}

	return updated.ToResponse(), nil
}

// DeleteEndpoint removes a webhook endpoint by ID.
func (s *Service) DeleteEndpoint(ctx context.Context, tenantID, id pgtype.UUID) error {
	return s.store.DeleteEndpoint(ctx, tenantID, id)
}

// TestEndpoint sends a test event to a webhook endpoint and records the delivery.
func (s *Service) TestEndpoint(ctx context.Context, tenantID, endpointID pgtype.UUID, eventType string) (pgtype.UUID, error) {
	// Verify the endpoint exists and belongs to this tenant.
	endpoint, err := s.store.GetEndpoint(ctx, tenantID, endpointID)
	if err != nil {
		return pgtype.UUID{}, err
	}

	testPayload := map[string]any{
		"type":    "test",
		"message": fmt.Sprintf("This is a test webhook delivery from %s", s.siteName),
	}

	var eventID pgtype.UUID
	if err := eventID.Scan("00000000-0000-0000-0000-000000000000"); err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating test event ID: %w", err)
	}

	deliveryID, err := s.CreateDelivery(ctx, tenantID, endpoint.ID, eventID, eventType, testPayload)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating test delivery: %w", err)
	}

	// Enqueue the async delivery task.
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID.String()})
		// Without this task the delivery row stays 'pending' forever — surface a failed enqueue.
		if err := s.enqueuer.Enqueue(worker.TypeWebhookDelivery, payload); err != nil {
			slog.Error("webhooks: failed to enqueue test delivery",
				"delivery_id", deliveryID.String(), "error", err)
			errpkg.CaptureException(fmt.Errorf("enqueue %s (delivery %s): %w", worker.TypeWebhookDelivery, deliveryID.String(), err))
		}
	}

	slog.Info(
		"webhook test delivery created",
		"delivery_id", deliveryID,
		"endpoint_id", endpointID,
		"event_type", eventType,
	)

	return deliveryID, nil
}

// DeliverWebhook fetches the delivery and endpoint rows, builds a signed
// envelope, calls DeliverWithRetry, and updates the delivery status in the
// database. Returns an error only if the delivery cannot be attempted (e.g.
// the row is missing). A failed delivery after all retries is recorded as
// 'failed' and returns nil so Asynq does not re-enqueue indefinitely.
func (s *Service) DeliverWebhook(ctx context.Context, deliveryID [16]byte) error {
	id := pgtype.UUID{Bytes: deliveryID, Valid: true}

	delivery, err := s.store.GetWebhookDelivery(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching webhook delivery: %w", err)
	}

	endpoint, err := s.store.GetWebhookEndpoint(ctx, delivery.EndpointID)
	if err != nil {
		return fmt.Errorf("fetching webhook endpoint: %w", err)
	}

	if endpoint.Status != "active" {
		slog.Info("webhook endpoint inactive, skipping delivery",
			"delivery_id", delivery.ID, "endpoint_id", endpoint.ID)
		return s.store.MarkWebhookFailed(ctx, id, delivery.Attempts, "endpoint inactive")
	}

	var data any
	if err := json.Unmarshal(delivery.Payload, &data); err != nil {
		return fmt.Errorf("unmarshaling webhook payload: %w", err)
	}

	envelope := WebhookEnvelope{
		EventID:       delivery.EventID.String(),
		EventType:     delivery.EventType,
		OccurredAt:    delivery.CreatedAt.Time,
		SchemaVersion: SchemaVersion,
		Data:          data,
	}

	maxAttempts := MaxDeliveryAttempts
	if s.settings != nil {
		maxAttempts = s.settings.GetIntOr(ctx, endpoint.TenantID, "notifications.max_delivery_attempts", MaxDeliveryAttempts)
	}
	deliverErr := DeliverWithRetry(ctx, endpoint.URL, endpoint.Secret, envelope, maxAttempts)
	attempts := int32(maxAttempts)

	if deliverErr != nil {
		errMsg := deliverErr.Error()
		if markErr := s.store.MarkWebhookFailed(ctx, id, attempts, errMsg); markErr != nil {
			slog.Error("failed to mark webhook delivery as failed",
				"delivery_id", id, "error", markErr)
		}

		slog.Error("webhook delivery failed after all retries",
			"delivery_id", id, "endpoint", endpoint.URL, "error", deliverErr)

		return nil
	}

	if err := s.store.MarkWebhookDelivered(ctx, id, attempts); err != nil {
		slog.Error("failed to mark webhook delivery as delivered",
			"delivery_id", id, "error", err)
	}

	slog.Info("webhook delivered successfully",
		"delivery_id", id, "endpoint", endpoint.URL, "event_type", delivery.EventType)

	return nil
}

// ---------------------------------------------------------------------------
// Delivery Logs
// ---------------------------------------------------------------------------

// ListDeliveries returns delivery records for a specific endpoint.
func (s *Service) ListDeliveries(ctx context.Context, tenantID, endpointID pgtype.UUID, limit, offset int32) ([]WebhookDeliveryResponse, error) {
	rows, err := s.store.ListDeliveries(ctx, tenantID, endpointID, limit, offset)
	if err != nil {
		return nil, err
	}

	var deliveries []WebhookDeliveryResponse
	for _, d := range rows {
		deliveries = append(deliveries, d.ToResponse())
	}

	return deliveries, nil
}

// GetDelivery returns a specific delivery record.
func (s *Service) GetDelivery(ctx context.Context, tenantID, deliveryID pgtype.UUID) (*WebhookDelivery, error) {
	d, err := s.store.GetDelivery(ctx, tenantID, deliveryID)
	if err != nil {
		return nil, err
	}

	return &d, nil
}

// RetryDelivery reschedules a failed delivery for immediate retry.
func (s *Service) RetryDelivery(ctx context.Context, tenantID, deliveryID pgtype.UUID) error {
	if err := s.store.RetryDelivery(ctx, tenantID, deliveryID); err != nil {
		return err
	}

	// Enqueue the retry delivery task.
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID.String()})
		// The row was just reset to 'pending'; a lost enqueue would strand it — surface the failure.
		if err := s.enqueuer.Enqueue(worker.TypeWebhookDelivery, payload); err != nil {
			slog.Error("webhooks: failed to enqueue retry delivery",
				"delivery_id", deliveryID.String(), "error", err)
			errpkg.CaptureException(fmt.Errorf("enqueue %s (delivery %s): %w", worker.TypeWebhookDelivery, deliveryID.String(), err))
		}
	}

	return nil
}

// CreateDelivery records a pending webhook delivery in the database. Callers
// enqueue the Asynq webhook_delivery task with the returned delivery ID.
func (s *Service) CreateDelivery(ctx context.Context, tenantID, endpointID, eventID pgtype.UUID, eventType string, payload any) (pgtype.UUID, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("marshaling webhook payload: %w", err)
	}

	return s.store.CreateDelivery(ctx, tenantID, endpointID, eventID, eventType, payloadJSON)
}

// EmitEvent dispatches a webhook event to all active endpoints for the tenant
// that subscribe to the given event type. For each matching endpoint it
// creates a pending delivery row and enqueues the Asynq webhook_delivery
// task. This is the canonical entry point for feature services to fire
// outgoing webhooks (Section 12).
//
// Ref: japaconnect-engineering/sections/12-webhooks.md
func (s *Service) EmitEvent(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any) {
	endpoints, err := s.store.ListEndpoints(ctx, tenantID)
	if err != nil {
		slog.Warn("webhooks: failed to list endpoints for emit", "tenant_id", tenantID, "error", err)
		return
	}

	var eventID pgtype.UUID
	if err := eventID.Scan("00000000-0000-0000-0000-000000000000"); err != nil {
		slog.Warn("webhooks: failed to scan zero event ID", "error", err)
		return
	}

	for _, ep := range endpoints {
		if ep.Status != "active" {
			continue
		}

		// Check if the endpoint subscribes to this event type.
		matched := false
		for _, et := range ep.EventTypes {
			if et == eventType || et == "*" {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		deliveryID, err := s.CreateDelivery(ctx, tenantID, ep.ID, eventID, eventType, payload)
		if err != nil {
			slog.Error("webhooks: failed to create delivery for emit",
				"endpoint_id", ep.ID, "event_type", eventType, "error", err)
			errpkg.CaptureException(fmt.Errorf("emit create delivery %s: %w", eventType, err))
			continue
		}

		if s.enqueuer != nil {
			taskPayload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID.String()})
			if err := s.enqueuer.Enqueue(worker.TypeWebhookDelivery, taskPayload); err != nil {
				slog.Error("webhooks: failed to enqueue delivery for emit",
					"delivery_id", deliveryID.String(), "error", err)
				errpkg.CaptureException(fmt.Errorf("enqueue %s (delivery %s): %w", worker.TypeWebhookDelivery, deliveryID.String(), err))
			}
		}
	}
}

// endpointMatches reports whether the endpoint's event_types list includes
// the event (or a "*" wildcard).
func endpointMatches(eventTypes []string, eventType string) bool {
	for _, t := range eventTypes {
		if t == "*" || t == eventType {
			return true
		}
	}
	return false
}
