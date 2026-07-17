package webhooks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
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

// WebhookDeliveryResponse is the JSON-safe representation of a webhook delivery.
type WebhookDeliveryResponse struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	EndpointID  string     `json:"endpoint_id"`
	EventID     string     `json:"event_id"`
	EventType   string     `json:"event_type"`
	Payload     []byte     `json:"payload"`
	Status      string     `json:"status"`
	Attempts    int32      `json:"attempts"`
	LastError   *string    `json:"last_error"`
	DeliveredAt *time.Time `json:"delivered_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ToResponse converts a WebhookDelivery to a JSON-safe response.
func (d WebhookDelivery) ToResponse() WebhookDeliveryResponse {
	r := WebhookDeliveryResponse{
		ID:         d.ID.String(),
		TenantID:   d.TenantID.String(),
		EndpointID: d.EndpointID.String(),
		EventID:    d.EventID.String(),
		EventType:  d.EventType,
		Payload:    d.Payload,
		Status:     d.Status,
		Attempts:   d.Attempts,
		LastError:  d.LastError,
		CreatedAt:  d.CreatedAt.Time,
	}
	if d.DeliveredAt.Valid {
		r.DeliveredAt = &d.DeliveredAt.Time
	}
	return r
}

// Store provides database access for webhook endpoints and deliveries via
// sqlc-generated queries (internal/db/queries/webhooks.sql).
type Store struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewStore creates a Store backed by the given database pool.
func NewStore(database *db.DB) *Store {
	return &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)}
}

// endpointFromRow converts a sqlc endpoint row to the package's internal type.
func endpointFromRow(row gen.WebhookEndpoint) WebhookEndpointWithSecret {
	return WebhookEndpointWithSecret{
		ID:         row.ID,
		TenantID:   row.TenantID,
		URL:        row.Url,
		Secret:     row.Secret,
		EventTypes: row.EventTypes,
		Status:     row.Status,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

// deliveryFromRow converts a sqlc delivery row to the package's internal type.
func deliveryFromRow(row gen.WebhookDelivery) WebhookDelivery {
	return WebhookDelivery{
		ID:          row.ID,
		TenantID:    row.TenantID,
		EndpointID:  row.EndpointID,
		EventID:     row.EventID,
		EventType:   row.EventType,
		Payload:     row.Payload,
		Status:      row.Status,
		Attempts:    row.Attempts,
		LastError:   row.LastError,
		DeliveredAt: row.DeliveredAt,
		CreatedAt:   row.CreatedAt,
	}
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

// ListEndpoints returns all webhook endpoints for the given tenant, newest first.
func (s *Store) ListEndpoints(ctx context.Context, tenantID pgtype.UUID) ([]WebhookEndpointWithSecret, error) {
	rows, err := s.read.ListWebhookEndpoints(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing webhook endpoints: %w", err)
	}

	endpoints := make([]WebhookEndpointWithSecret, 0, len(rows))
	for _, row := range rows {
		endpoints = append(endpoints, endpointFromRow(row))
	}

	return endpoints, nil
}

// GetEndpoint returns a tenant-scoped webhook endpoint by ID.
func (s *Store) GetEndpoint(ctx context.Context, tenantID, id pgtype.UUID) (WebhookEndpointWithSecret, error) {
	row, err := s.read.GetWebhookEndpoint(ctx, gen.GetWebhookEndpointParams{TenantID: tenantID, ID: id})
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("fetching webhook endpoint %s: %w", id, err)
	}

	return endpointFromRow(row), nil
}

// CreateEndpoint registers a new webhook endpoint.
func (s *Store) CreateEndpoint(ctx context.Context, tenantID pgtype.UUID, url, secret string, eventTypes []string, status string) (WebhookEndpointWithSecret, error) {
	row, err := s.write.CreateWebhookEndpoint(ctx, gen.CreateWebhookEndpointParams{
		TenantID:   tenantID,
		Url:        url,
		Secret:     secret,
		EventTypes: eventTypes,
		Status:     status,
	})
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("creating webhook endpoint: %w", err)
	}

	return endpointFromRow(row), nil
}

// UpdateEndpoint updates an existing webhook endpoint.
func (s *Store) UpdateEndpoint(ctx context.Context, tenantID, id pgtype.UUID, url, secret string, eventTypes []string, status string) (WebhookEndpointWithSecret, error) {
	row, err := s.write.UpdateWebhookEndpoint(ctx, gen.UpdateWebhookEndpointParams{
		TenantID:   tenantID,
		ID:         id,
		Url:        url,
		Secret:     secret,
		EventTypes: eventTypes,
		Status:     status,
	})
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("updating webhook endpoint %s: %w", id, err)
	}

	return endpointFromRow(row), nil
}

// DeleteEndpoint removes a webhook endpoint by ID.
func (s *Store) DeleteEndpoint(ctx context.Context, tenantID, id pgtype.UUID) error {
	rowsAffected, err := s.write.DeleteWebhookEndpoint(ctx, gen.DeleteWebhookEndpointParams{TenantID: tenantID, ID: id})
	if err != nil {
		return fmt.Errorf("deleting webhook endpoint %s: %w", id, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("webhook endpoint %s not found", id)
	}

	return nil
}

// GetWebhookEndpoint fetches an endpoint configuration by ID (worker path — no
// tenant scope; the delivery row already pins the tenant).
func (s *Store) GetWebhookEndpoint(ctx context.Context, id pgtype.UUID) (WebhookEndpoint, error) {
	row, err := s.read.GetWebhookEndpointByID(ctx, id)
	if err != nil {
		return WebhookEndpoint{}, fmt.Errorf("fetching webhook endpoint %s: %w", id, err)
	}

	return WebhookEndpoint{
		ID:         row.ID,
		TenantID:   row.TenantID,
		URL:        row.Url,
		Secret:     row.Secret,
		EventTypes: row.EventTypes,
		Status:     row.Status,
	}, nil
}

// ---------------------------------------------------------------------------
// Deliveries
// ---------------------------------------------------------------------------

// CreateDelivery records a pending webhook delivery and returns its ID.
func (s *Store) CreateDelivery(ctx context.Context, tenantID, endpointID, eventID pgtype.UUID, eventType string, payloadJSON []byte) (pgtype.UUID, error) {
	id, err := s.write.CreateWebhookDelivery(ctx, gen.CreateWebhookDeliveryParams{
		TenantID:   tenantID,
		EndpointID: endpointID,
		EventID:    eventID,
		EventType:  eventType,
		Payload:    payloadJSON,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating webhook delivery: %w", err)
	}

	return id, nil
}

// GetWebhookDelivery fetches a delivery row by ID (worker path — no tenant scope).
func (s *Store) GetWebhookDelivery(ctx context.Context, id pgtype.UUID) (WebhookDelivery, error) {
	row, err := s.read.GetWebhookDelivery(ctx, id)
	if err != nil {
		return WebhookDelivery{}, fmt.Errorf("fetching webhook delivery %s: %w", id, err)
	}

	return deliveryFromRow(row), nil
}

// GetDelivery returns a tenant-scoped delivery record.
func (s *Store) GetDelivery(ctx context.Context, tenantID, deliveryID pgtype.UUID) (WebhookDelivery, error) {
	row, err := s.read.GetWebhookDeliveryForTenant(ctx, gen.GetWebhookDeliveryForTenantParams{TenantID: tenantID, ID: deliveryID})
	if err != nil {
		return WebhookDelivery{}, fmt.Errorf("get delivery: %w", err)
	}

	return deliveryFromRow(row), nil
}

// ListDeliveries returns delivery records for a specific endpoint, newest first.
func (s *Store) ListDeliveries(ctx context.Context, tenantID, endpointID pgtype.UUID, limit, offset int32) ([]WebhookDelivery, error) {
	rows, err := s.read.ListWebhookDeliveries(ctx, gen.ListWebhookDeliveriesParams{
		TenantID:   tenantID,
		EndpointID: endpointID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}

	deliveries := make([]WebhookDelivery, 0, len(rows))
	for _, row := range rows {
		deliveries = append(deliveries, deliveryFromRow(row))
	}

	return deliveries, nil
}

// RetryDelivery resets a failed delivery to pending for immediate re-enqueue.
func (s *Store) RetryDelivery(ctx context.Context, tenantID, deliveryID pgtype.UUID) error {
	rowsAffected, err := s.write.RetryWebhookDelivery(ctx, gen.RetryWebhookDeliveryParams{TenantID: tenantID, ID: deliveryID})
	if err != nil {
		return fmt.Errorf("retry delivery: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("delivery not found or not in failed state")
	}

	return nil
}

// MarkWebhookDelivered records a successful delivery.
func (s *Store) MarkWebhookDelivered(ctx context.Context, id pgtype.UUID, attempts int32) error {
	err := s.write.MarkWebhookDelivered(ctx, gen.MarkWebhookDeliveredParams{
		ID:          id,
		Attempts:    attempts,
		DeliveredAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("marking webhook delivery %s as delivered: %w", id, err)
	}

	return nil
}

// MarkWebhookFailed records a failed delivery after all retries are exhausted.
func (s *Store) MarkWebhookFailed(ctx context.Context, id pgtype.UUID, attempts int32, lastError string) error {
	err := s.write.MarkWebhookFailed(ctx, gen.MarkWebhookFailedParams{
		ID:        id,
		Attempts:  attempts,
		LastError: &lastError,
	})
	if err != nil {
		return fmt.Errorf("marking webhook delivery %s as failed: %w", id, err)
	}

	return nil
}
