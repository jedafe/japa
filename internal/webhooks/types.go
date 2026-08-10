// Package webhooks — types.go defines request/response types for webhook admin CRUD.
package webhooks

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ListWebhooksResponse wraps a paginated list of webhook endpoints.
type ListWebhooksResponse struct {
	Data       []WebhookEndpointResponse `json:"data"`
	TotalCount int                       `json:"total_count"`
	Page       int32                     `json:"page"`
	PageSize   int32                     `json:"page_size"`
}

// CreateWebhookRequest is the input for creating a new webhook endpoint.
// The secret is server-generated and returned once in the create response;
// the client does not supply it (§12: "Returns: { id, secret } — secret shown once").
type CreateWebhookRequest struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
}

// UpdateWebhookRequest is the input for updating an existing webhook endpoint.
type UpdateWebhookRequest struct {
	Name       *string  `json:"name,omitempty"`
	URL        *string  `json:"url,omitempty"`
	Secret     *string  `json:"secret,omitempty"`
	EventTypes []string `json:"event_types,omitempty"`
	IsActive   *bool    `json:"is_active,omitempty"`
}

// TestWebhookRequest is the input for sending a test event to a webhook endpoint.
type TestWebhookRequest struct {
	EventType string `json:"event_type"`
}

// WebhookEndpointWithSecret is a webhook endpoint that includes the secret for
// internal service/store use. The secret must never be serialized to a JSON
// response except in the single create-response (§12: "secret shown once").
type WebhookEndpointWithSecret struct {
	ID         pgtype.UUID
	TenantID   pgtype.UUID
	Name       string
	URL        string
	Secret     string
	EventTypes []string
	Status     string
	IsActive   bool
	VerifiedAt pgtype.Timestamptz
	CreatedBy  pgtype.UUID
	CreatedAt  pgtype.Timestamptz
	UpdatedAt  pgtype.Timestamptz
}

// WebhookEndpointResponse is the JSON-safe representation of a webhook endpoint.
// The secret is never included (§12: "secret shown once" — only on create).
type WebhookEndpointResponse struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	EventTypes []string  `json:"event_types"`
	Status     string    `json:"status"`
	IsActive   bool      `json:"is_active"`
	VerifiedAt *time.Time `json:"verified_at"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// WebhookEndpointCreateResponse is returned only from POST /admin/webhooks and
// includes the server-generated secret exactly once (§12).
type WebhookEndpointCreateResponse struct {
	WebhookEndpointResponse
	Secret string `json:"secret"`
}

// ToResponse converts a WebhookEndpointWithSecret to a JSON-safe response
// without the secret.
func (e WebhookEndpointWithSecret) ToResponse() WebhookEndpointResponse {
	r := WebhookEndpointResponse{
		ID:         e.ID.String(),
		TenantID:   e.TenantID.String(),
		Name:       e.Name,
		URL:        e.URL,
		EventTypes: e.EventTypes,
		Status:     e.Status,
		IsActive:   e.IsActive,
		CreatedBy:  e.CreatedBy.String(),
		CreatedAt:  e.CreatedAt.Time,
		UpdatedAt:  e.UpdatedAt.Time,
	}
	if e.VerifiedAt.Valid {
		t := e.VerifiedAt.Time
		r.VerifiedAt = &t
	}
	return r
}

// ToCreateResponse converts a WebhookEndpointWithSecret to a create response
// that includes the secret exactly once.
func (e WebhookEndpointWithSecret) ToCreateResponse() WebhookEndpointCreateResponse {
	return WebhookEndpointCreateResponse{
		WebhookEndpointResponse: e.ToResponse(),
		Secret:                  e.Secret,
	}
}
