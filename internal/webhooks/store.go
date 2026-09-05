package webhooks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// WebhookEndpoint is the configuration for a single outgoing webhook target.
type WebhookEndpoint struct {
	ID         pgtype.UUID
	TenantID   pgtype.UUID
	Name       string
	URL        string
	Secret     string
	EventTypes []string
	Status     string
	IsActive   bool
}

// WebhookDelivery is a single delivery attempt record stored in the database.
type WebhookDelivery struct {
	ID               pgtype.UUID
	TenantID         pgtype.UUID
	EndpointID       pgtype.UUID
	EventID          pgtype.UUID
	EventType        string
	Payload          []byte
	Status           string
	Attempts         int32
	LastError        *string
	NextAttemptAt    pgtype.Timestamptz
	LastResponseCode *int32
	LastResponseBody *string
	DeliveredAt      pgtype.Timestamptz
	CreatedAt        pgtype.Timestamptz
}

// WebhookDeliveryResponse is the JSON-safe representation of a webhook delivery.
type WebhookDeliveryResponse struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	EndpointID       string     `json:"endpoint_id"`
	EventID          string     `json:"event_id"`
	EventType        string     `json:"event_type"`
	Payload          []byte     `json:"payload"`
	Status           string     `json:"status"`
	Attempts         int32      `json:"attempts"`
	LastError        *string    `json:"last_error"`
	NextAttemptAt    *time.Time `json:"next_attempt_at"`
	LastResponseCode *int32     `json:"last_response_code"`
	LastResponseBody *string    `json:"last_response_body"`
	DeliveredAt      *time.Time `json:"delivered_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ToResponse converts a WebhookDelivery to a JSON-safe response.
func (d WebhookDelivery) ToResponse() WebhookDeliveryResponse {
	r := WebhookDeliveryResponse{
		ID:               d.ID.String(),
		TenantID:         d.TenantID.String(),
		EndpointID:       d.EndpointID.String(),
		EventID:          d.EventID.String(),
		EventType:        d.EventType,
		Payload:          d.Payload,
		Status:           d.Status,
		Attempts:         d.Attempts,
		LastError:        d.LastError,
		LastResponseCode: d.LastResponseCode,
		LastResponseBody: d.LastResponseBody,
		CreatedAt:        d.CreatedAt.Time,
	}
	if d.DeliveredAt.Valid {
		r.DeliveredAt = &d.DeliveredAt.Time
	}

	if d.NextAttemptAt.Valid {
		r.NextAttemptAt = &d.NextAttemptAt.Time
	}

	return r
}

// Store provides database access for webhook endpoints and deliveries via
// sqlc-generated queries (internal/db/queries/webhooks.sql).
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates a Store backed by the given database pool.
func NewStore(database *db.DB) *Store {
	return &Store{read: gen.New(database.Reader), write: gen.New(database.Writer), db: database}
}

// endpointFromListRow converts a ListWebhookEndpointsRow.
func endpointFromListRow(row gen.ListWebhookEndpointsRow) WebhookEndpointWithSecret {
	return WebhookEndpointWithSecret{
		ID: row.ID, TenantID: row.TenantID, Name: row.Name, URL: row.Url,
		Secret: row.Secret, EventTypes: row.EventTypes, Status: row.Status,
		IsActive: row.IsActive, VerifiedAt: row.VerifiedAt, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// endpointFromGetRow converts a GetWebhookEndpointRow.
func endpointFromGetRow(row gen.GetWebhookEndpointRow) WebhookEndpointWithSecret {
	return WebhookEndpointWithSecret{
		ID: row.ID, TenantID: row.TenantID, Name: row.Name, URL: row.Url,
		Secret: row.Secret, EventTypes: row.EventTypes, Status: row.Status,
		IsActive: row.IsActive, VerifiedAt: row.VerifiedAt, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// endpointFromCreateRow converts a CreateWebhookEndpointRow.
func endpointFromCreateRow(row gen.CreateWebhookEndpointRow) WebhookEndpointWithSecret {
	return WebhookEndpointWithSecret{
		ID: row.ID, TenantID: row.TenantID, Name: row.Name, URL: row.Url,
		Secret: row.Secret, EventTypes: row.EventTypes, Status: row.Status,
		IsActive: row.IsActive, VerifiedAt: row.VerifiedAt, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// endpointFromUpdateRow converts a UpdateWebhookEndpointRow.
func endpointFromUpdateRow(row gen.UpdateWebhookEndpointRow) WebhookEndpointWithSecret {
	return WebhookEndpointWithSecret{
		ID: row.ID, TenantID: row.TenantID, Name: row.Name, URL: row.Url,
		Secret: row.Secret, EventTypes: row.EventTypes, Status: row.Status,
		IsActive: row.IsActive, VerifiedAt: row.VerifiedAt, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// deliveryFromGetRow converts a GetWebhookDeliveryRow.
func deliveryFromGetRow(row gen.GetWebhookDeliveryRow) WebhookDelivery {
	return WebhookDelivery{
		ID: row.ID, TenantID: row.TenantID, EndpointID: row.EndpointID,
		EventID: row.EventID, EventType: row.EventType, Payload: row.Payload,
		Status: row.Status, Attempts: row.Attempts, LastError: row.LastError,
		NextAttemptAt: row.NextAttemptAt, LastResponseCode: row.LastResponseCode,
		LastResponseBody: row.LastResponseBody, DeliveredAt: row.DeliveredAt,
		CreatedAt: row.CreatedAt,
	}
}

// deliveryFromGetForTenantRow converts a GetWebhookDeliveryForTenantRow.
func deliveryFromGetForTenantRow(row gen.GetWebhookDeliveryForTenantRow) WebhookDelivery {
	return WebhookDelivery{
		ID: row.ID, TenantID: row.TenantID, EndpointID: row.EndpointID,
		EventID: row.EventID, EventType: row.EventType, Payload: row.Payload,
		Status: row.Status, Attempts: row.Attempts, LastError: row.LastError,
		NextAttemptAt: row.NextAttemptAt, LastResponseCode: row.LastResponseCode,
		LastResponseBody: row.LastResponseBody, DeliveredAt: row.DeliveredAt,
		CreatedAt: row.CreatedAt,
	}
}

// deliveryFromListRow converts a ListWebhookDeliveriesRow.
func deliveryFromListRow(row gen.ListWebhookDeliveriesRow) WebhookDelivery {
	return WebhookDelivery{
		ID: row.ID, TenantID: row.TenantID, EndpointID: row.EndpointID,
		EventID: row.EventID, EventType: row.EventType, Payload: row.Payload,
		Status: row.Status, Attempts: row.Attempts, LastError: row.LastError,
		NextAttemptAt: row.NextAttemptAt, LastResponseCode: row.LastResponseCode,
		LastResponseBody: row.LastResponseBody, DeliveredAt: row.DeliveredAt,
		CreatedAt: row.CreatedAt,
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
		endpoints = append(endpoints, endpointFromListRow(row))
	}

	return endpoints, nil
}

// GetEndpoint returns a tenant-scoped webhook endpoint by ID.
func (s *Store) GetEndpoint(ctx context.Context, tenantID, id pgtype.UUID) (WebhookEndpointWithSecret, error) {
	row, err := s.read.GetWebhookEndpoint(ctx, gen.GetWebhookEndpointParams{TenantID: tenantID, ID: id})
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("fetching webhook endpoint %s: %w", id, err)
	}

	return endpointFromGetRow(row), nil
}

// CreateEndpoint registers a new webhook endpoint. The secret is server-generated
// and encrypted at rest before storage (BUG-016). New endpoints default to
// is_active=false until challenge/response verification (§12). createdBy is
// the admin user who created the endpoint.
func (s *Store) CreateEndpoint(ctx context.Context, tenantID pgtype.UUID, name, url, secret string, eventTypes []string, createdBy pgtype.UUID) (WebhookEndpointWithSecret, error) {
	encSecret, err := crypto.Encrypt(secret)
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("encrypting webhook secret: %w", err)
	}

	row, err := s.write.CreateWebhookEndpoint(ctx, gen.CreateWebhookEndpointParams{
		TenantID:   tenantID,
		Name:       name,
		Url:        url,
		Secret:     encSecret,
		EventTypes: eventTypes,
		Status:     "disabled",
		IsActive:   false,
		CreatedBy:  createdBy,
	})
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("creating webhook endpoint: %w", err)
	}

	ep := endpointFromCreateRow(row)
	// Return the plaintext secret in the in-memory struct so the service can
	// include it in the create response exactly once (§12). The DB stores the
	// encrypted form.
	ep.Secret = secret

	return ep, nil
}

// UpdateEndpoint updates an existing webhook endpoint. If the secret starts
// with "v1:" it is already encrypted; otherwise it is plaintext and gets
// encrypted before storage (BUG-016).
func (s *Store) UpdateEndpoint(ctx context.Context, tenantID, id pgtype.UUID, name, url, secret string, eventTypes []string, status string, isActive bool) (WebhookEndpointWithSecret, error) {
	encSecret := secret

	if !strings.HasPrefix(secret, "v1:") {
		enc, err := crypto.Encrypt(secret)
		if err != nil {
			return WebhookEndpointWithSecret{}, fmt.Errorf("encrypting webhook secret: %w", err)
		}

		encSecret = enc
	}

	row, err := s.write.UpdateWebhookEndpoint(ctx, gen.UpdateWebhookEndpointParams{
		TenantID:   tenantID,
		ID:         id,
		Name:       name,
		Url:        url,
		Secret:     encSecret,
		EventTypes: eventTypes,
		Status:     status,
		IsActive:   isActive,
	})
	if err != nil {
		return WebhookEndpointWithSecret{}, fmt.Errorf("updating webhook endpoint %s: %w", id, err)
	}

	return endpointFromUpdateRow(row), nil
}

// VerifyEndpoint marks a webhook endpoint as verified and active after a
// successful challenge/response (§12).
func (s *Store) VerifyEndpoint(ctx context.Context, tenantID, id pgtype.UUID) error {
	if err := s.write.VerifyWebhookEndpoint(ctx, gen.VerifyWebhookEndpointParams{
		TenantID: tenantID,
		ID:       id,
	}); err != nil {
		return fmt.Errorf("verifying webhook endpoint %s: %w", id, err)
	}

	return nil
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
// tenant scope; the delivery row already pins the tenant). The secret is
// decrypted from its at-rest encrypted form (BUG-016). If the secret is not
// yet encrypted (legacy plaintext from before migration 000363), it is
// re-encrypted and updated in-place (graceful migration).
func (s *Store) GetWebhookEndpoint(ctx context.Context, id pgtype.UUID) (WebhookEndpoint, error) {
	row, err := s.read.GetWebhookEndpointByID(ctx, id)
	if err != nil {
		return WebhookEndpoint{}, fmt.Errorf("fetching webhook endpoint %s: %w", id, err)
	}

	plaintextSecret := row.Secret
	if strings.HasPrefix(row.Secret, "v1:") {
		plaintextSecret, err = crypto.Decrypt(row.Secret)
		if err != nil {
			return WebhookEndpoint{}, fmt.Errorf("decrypting webhook secret for %s: %w", id, err)
		}
	} else {
		// Legacy plaintext secret — re-encrypt and update in-place (graceful
		// migration from 000363). Log but don't fail if the re-encryption
		// update fails; the delivery can still proceed with the plaintext.
		if enc, encErr := crypto.Encrypt(row.Secret); encErr == nil {
			_, _ = s.db.Writer.Exec(ctx, `UPDATE webhook_endpoints SET secret = $1 WHERE id = $2`, enc, id)
		} else {
			slog.Warn("webhooks: failed to re-encrypt legacy plaintext secret", "endpoint_id", id, "error", encErr)
		}
	}

	return WebhookEndpoint{
		ID:         row.ID,
		TenantID:   row.TenantID,
		Name:       row.Name,
		URL:        row.Url,
		Secret:     plaintextSecret,
		EventTypes: row.EventTypes,
		Status:     row.Status,
		IsActive:   row.IsActive,
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

	return deliveryFromGetRow(row), nil
}

// GetDelivery returns a tenant-scoped delivery record.
func (s *Store) GetDelivery(ctx context.Context, tenantID, deliveryID pgtype.UUID) (WebhookDelivery, error) {
	row, err := s.read.GetWebhookDeliveryForTenant(ctx, gen.GetWebhookDeliveryForTenantParams{TenantID: tenantID, ID: deliveryID})
	if err != nil {
		return WebhookDelivery{}, fmt.Errorf("get delivery: %w", err)
	}

	return deliveryFromGetForTenantRow(row), nil
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
		deliveries = append(deliveries, deliveryFromListRow(row))
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
		return errors.New("delivery not found or not in failed state")
	}

	return nil
}

// MarkWebhookDelivered records a successful delivery.
func (s *Store) MarkWebhookDelivered(ctx context.Context, id pgtype.UUID, attempts int32, responseCode *int32, responseBody *string) error {
	err := s.write.MarkWebhookDelivered(ctx, gen.MarkWebhookDeliveredParams{
		ID:               id,
		Attempts:         attempts,
		DeliveredAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
		LastResponseCode: responseCode,
		LastResponseBody: responseBody,
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

// MarkWebhookRetrying records a delivery that will be retried later, storing
// the next attempt time and last response details (§12 next_attempt_at).
func (s *Store) MarkWebhookRetrying(ctx context.Context, id pgtype.UUID, attempts int32, lastError string, nextAttemptAt time.Time, responseCode *int32, responseBody *string) error {
	err := s.write.MarkWebhookRetrying(ctx, gen.MarkWebhookRetryingParams{
		ID:               id,
		Attempts:         attempts,
		LastError:        &lastError,
		NextAttemptAt:    pgtype.Timestamptz{Time: nextAttemptAt, Valid: true},
		LastResponseCode: responseCode,
		LastResponseBody: responseBody,
	})
	if err != nil {
		return fmt.Errorf("marking webhook delivery %s as retrying: %w", id, err)
	}

	return nil
}
