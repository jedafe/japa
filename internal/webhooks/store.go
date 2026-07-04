package webhooks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// WebhookEndpoint is the configuration for a single outgoing webhook target.
type WebhookEndpoint struct {
	ID         pgtype.UUID
	TenantID   pgtype.UUID
	URL        string
	Secret     string
	EventTypes []string
	Status     string
}

// WebhookDelivery is a single delivery attempt record stored in the database.
type WebhookDelivery struct {
	ID          pgtype.UUID
	TenantID    pgtype.UUID
	EndpointID  pgtype.UUID
	EventID     pgtype.UUID
	EventType   string
	Payload     []byte
	Status      string
	Attempts    int32
	LastError   *string
	DeliveredAt pgtype.Timestamptz
	CreatedAt   pgtype.Timestamptz
}

// Store provides database access for webhook delivery using direct pgx
// queries. The webhook tables are managed outside sqlc to keep the delivery
// subsystem self-contained.
type Store struct {
	db *db.DB
}

// NewStore creates a Store backed by the given database pool.
func NewStore(database *db.DB) *Store {
	return &Store{db: database}
}

// GetWebhookDelivery fetches a delivery row by ID.
func (s *Store) GetWebhookDelivery(ctx context.Context, id pgtype.UUID) (WebhookDelivery, error) {
	var d WebhookDelivery

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, endpoint_id, event_id, event_type, payload,
		       status, attempts, last_error, delivered_at, created_at
		FROM webhook_deliveries
		WHERE id = $1
	`, id).Scan(
		&d.ID, &d.TenantID, &d.EndpointID, &d.EventID, &d.EventType, &d.Payload,
		&d.Status, &d.Attempts, &d.LastError, &d.DeliveredAt, &d.CreatedAt,
	)
	if err != nil {
		return WebhookDelivery{}, fmt.Errorf("fetching webhook delivery %s: %w", id, err)
	}

	return d, nil
}

// GetWebhookEndpoint fetches an endpoint configuration by ID.
func (s *Store) GetWebhookEndpoint(ctx context.Context, id pgtype.UUID) (WebhookEndpoint, error) {
	var e WebhookEndpoint

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, url, secret, event_types, status
		FROM webhook_endpoints
		WHERE id = $1
	`, id).Scan(
		&e.ID, &e.TenantID, &e.URL, &e.Secret, &e.EventTypes, &e.Status,
	)
	if err != nil {
		return WebhookEndpoint{}, fmt.Errorf("fetching webhook endpoint %s: %w", id, err)
	}

	return e, nil
}

// MarkWebhookDelivered records a successful delivery.
func (s *Store) MarkWebhookDelivered(ctx context.Context, id pgtype.UUID, attempts int32) error {
	_, err := s.db.Writer.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = 'delivered', attempts = $2, delivered_at = $3, last_error = NULL
		WHERE id = $1
	`, id, attempts, time.Now())
	if err != nil {
		return fmt.Errorf("marking webhook delivery %s as delivered: %w", id, err)
	}

	return nil
}

// MarkWebhookFailed records a failed delivery after all retries are exhausted.
func (s *Store) MarkWebhookFailed(ctx context.Context, id pgtype.UUID, attempts int32, lastError string) error {
	_, err := s.db.Writer.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = 'failed', attempts = $2, last_error = $3
		WHERE id = $1
	`, id, attempts, lastError)
	if err != nil {
		return fmt.Errorf("marking webhook delivery %s as failed: %w", id, err)
	}

	return nil
}
