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
