package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// MaxDeliveryAttempts is the roadmap-mandated retry count for webhook delivery.
const MaxDeliveryAttempts = 5

// Service coordinates webhook endpoint lookup, signed delivery, and delivery
// status tracking. It implements the worker.WebhookProcessor interface.
type Service struct {
	store *Store
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{store: NewStore(database)}
}

// ---------------------------------------------------------------------------
// Admin CRUD — Webhook Endpoints
// ---------------------------------------------------------------------------

// ListEndpoints returns all webhook endpoints for the given tenant.
func (s *Service) ListEndpoints(ctx context.Context, tenantID pgtype.UUID) ([]WebhookEndpointWithSecret, error) {
	rows, err := s.store.db.Reader.Query(ctx, `
		SELECT id, tenant_id, url, secret, event_types, status, created_at, updated_at
		FROM webhook_endpoints
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing webhook endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []WebhookEndpointWithSecret

	for rows.Next() {
		var ep WebhookEndpointWithSecret
		if err := rows.Scan(
			&ep.ID, &ep.TenantID, &ep.URL, &ep.Secret, &ep.EventTypes,
			&ep.Status, &ep.CreatedAt, &ep.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning webhook endpoint: %w", err)
		}

		endpoints = append(endpoints, ep)
	}

	return endpoints, rows.Err()
}

// GetEndpoint returns a single webhook endpoint by ID.
func (s *Service) GetEndpoint(ctx context.Context, tenantID, id pgtype.UUID) (WebhookEndpointWithSecret, error) {
	var ep WebhookEndpointWithSecret

	err := s.store.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, url, secret, event_types, status, created_at, updated_at
		FROM webhook_endpoints
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&ep.ID, &ep.TenantID, &ep.URL, &ep.Secret, &ep.EventTypes,
		&ep.Status, &ep.CreatedAt, &ep.UpdatedAt,
	)
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("fetching webhook endpoint %s: %w", id, err)
	}

	return ep, nil
}

// CreateEndpoint registers a new webhook endpoint.
func (s *Service) CreateEndpoint(ctx context.Context, tenantID pgtype.UUID, input CreateWebhookRequest) (WebhookEndpointWithSecret, error) {
	status := input.Status
	if status == "" {
		status = "active"
	}

	var ep WebhookEndpointWithSecret

	err := s.store.db.Writer.QueryRow(ctx, `
		INSERT INTO webhook_endpoints (tenant_id, url, secret, event_types, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, url, secret, event_types, status, created_at, updated_at
	`, tenantID, input.URL, input.Secret, input.EventTypes, status).Scan(
		&ep.ID, &ep.TenantID, &ep.URL, &ep.Secret, &ep.EventTypes,
		&ep.Status, &ep.CreatedAt, &ep.UpdatedAt,
	)
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("creating webhook endpoint: %w", err)
	}

	return ep, nil
}

// UpdateEndpoint updates an existing webhook endpoint.
func (s *Service) UpdateEndpoint(ctx context.Context, tenantID, id pgtype.UUID, input UpdateWebhookRequest) (WebhookEndpointWithSecret, error) {
	// Fetch current values for merge.
	current, err := s.GetEndpoint(ctx, tenantID, id)
	if err != nil {
		return WebhookEndpointWithSecret{}, err
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

	var updated WebhookEndpointWithSecret

	err = s.store.db.Writer.QueryRow(ctx, `
		UPDATE webhook_endpoints
		SET url = $3, secret = $4, event_types = $5, status = $6, updated_at = now()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, url, secret, event_types, status, created_at, updated_at
	`, tenantID, id, url, secret, eventTypes, status).Scan(
		&updated.ID, &updated.TenantID, &updated.URL, &updated.Secret, &updated.EventTypes,
		&updated.Status, &updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("updating webhook endpoint %s: %w", id, err)
	}

	return updated, nil
}

// DeleteEndpoint removes a webhook endpoint by ID.
func (s *Service) DeleteEndpoint(ctx context.Context, tenantID, id pgtype.UUID) error {
	tag, err := s.store.db.Writer.Exec(ctx, `
		DELETE FROM webhook_endpoints
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return fmt.Errorf("deleting webhook endpoint %s: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("webhook endpoint %s not found", id)
	}

	return nil
}

// TestEndpoint sends a test event to a webhook endpoint and records the delivery.
func (s *Service) TestEndpoint(ctx context.Context, tenantID, endpointID pgtype.UUID, eventType string) (pgtype.UUID, error) {
	// Verify the endpoint exists and belongs to this tenant.
	endpoint, err := s.GetEndpoint(ctx, tenantID, endpointID)
	if err != nil {
		return pgtype.UUID{}, err
	}

	testPayload := map[string]any{
		"type":    "test",
		"message": "This is a test webhook delivery from JapaConnect",
	}

	var eventID pgtype.UUID
	if err := eventID.Scan("00000000-0000-0000-0000-000000000000"); err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating test event ID: %w", err)
	}

	deliveryID, err := s.CreateDelivery(ctx, tenantID, endpoint.ID, eventID, eventType, testPayload)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating test delivery: %w", err)
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

	deliverErr := DeliverWithRetry(ctx, endpoint.URL, endpoint.Secret, envelope, MaxDeliveryAttempts)
	attempts := int32(MaxDeliveryAttempts)

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

// CreateDelivery records a pending webhook delivery in the database. Callers
// enqueue the Asynq webhook_delivery task with the returned delivery ID.
func (s *Service) CreateDelivery(ctx context.Context, tenantID, endpointID, eventID pgtype.UUID, eventType string, payload any) (pgtype.UUID, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("marshaling webhook payload: %w", err)
	}

	var id pgtype.UUID

	err = s.store.db.Writer.QueryRow(ctx, `
		INSERT INTO webhook_deliveries (tenant_id, endpoint_id, event_id, event_type, payload)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, tenantID, endpointID, eventID, eventType, payloadJSON).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating webhook delivery: %w", err)
	}

	return id, nil
}
