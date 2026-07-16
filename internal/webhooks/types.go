// Package webhooks — types.go defines request/response types for webhook admin CRUD.
package webhooks

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ListWebhooksResponse wraps a paginated list of webhook endpoints.
type ListWebhooksResponse struct {
	Data       []WebhookEndpoint `json:"data"`
	TotalCount int               `json:"total_count"`
	Page       int32             `json:"page"`
	PageSize   int32             `json:"page_size"`
}

// CreateWebhookRequest is the input for creating a new webhook endpoint.
type CreateWebhookRequest struct {
	URL        string   `json:"url"`
	Secret     string   `json:"secret"`
	EventTypes []string `json:"event_types"`
	Status     string   `json:"status,omitempty"`
}

// UpdateWebhookRequest is the input for updating an existing webhook endpoint.
type UpdateWebhookRequest struct {
	URL        *string  `json:"url,omitempty"`
	Secret     *string  `json:"secret,omitempty"`
	EventTypes []string `json:"event_types,omitempty"`
	Status     *string  `json:"status,omitempty"`
}

// TestWebhookRequest is the input for sending a test event to a webhook endpoint.
type TestWebhookRequest struct {
	EventType string `json:"event_type"`
}

// WebhookEndpointWithSecret is a webhook endpoint that includes the secret for admin viewing.
// Kept internal (pgtype fields) for the service/store layer.
type WebhookEndpointWithSecret struct {
	ID         pgtype.UUID
	TenantID   pgtype.UUID
	URL        string
	Secret     string
	EventTypes []string
	Status     string
	CreatedAt  pgtype.Timestamptz
	UpdatedAt  pgtype.Timestamptz
}

// WebhookEndpointResponse is the JSON-safe representation of a webhook endpoint.
type WebhookEndpointResponse struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	URL        string    `json:"url"`
	Secret     string    `json:"secret"`
	EventTypes []string  `json:"event_types"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ToResponse converts a WebhookEndpointWithSecret to a JSON-safe response.
func (e WebhookEndpointWithSecret) ToResponse() WebhookEndpointResponse {
	return WebhookEndpointResponse{
		ID:         e.ID.String(),
		TenantID:   e.TenantID.String(),
		URL:        e.URL,
		Secret:     e.Secret,
		EventTypes: e.EventTypes,
		Status:     e.Status,
		CreatedAt:  e.CreatedAt.Time,
		UpdatedAt:  e.UpdatedAt.Time,
	}
}
