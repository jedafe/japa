// Package webhooks — types.go defines request/response types for webhook admin CRUD.
package webhooks

import "github.com/jackc/pgx/v5/pgtype"

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
type WebhookEndpointWithSecret struct {
	ID         pgtype.UUID `json:"id"`
	TenantID   pgtype.UUID `json:"tenant_id"`
	URL        string      `json:"url"`
	Secret     string      `json:"secret"`
	EventTypes []string    `json:"event_types"`
	Status     string      `json:"status"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
	UpdatedAt  pgtype.Timestamptz `json:"updated_at"`
}
