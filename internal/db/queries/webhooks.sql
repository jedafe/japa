-- Outgoing Webhooks — endpoint CRUD + delivery audit rows (Section 12)

-- name: ListWebhookEndpoints :many
SELECT id, name, url, secret, event_types, status, is_active, verified_at, created_by, created_at, updated_at
FROM webhook_endpoints

ORDER BY created_at DESC;

-- name: GetWebhookEndpoint :one
SELECT id, name, url, secret, event_types, status, is_active, verified_at, created_by, created_at, updated_at
FROM webhook_endpoints
WHERE id = @id;

-- name: GetWebhookEndpointByID :one
SELECT id, name, url, secret, event_types, status, is_active
FROM webhook_endpoints
WHERE id = @id;

-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (name, url, secret, event_types, status, is_active, created_by) VALUES (@name, @url, @secret, @event_types, @status, @is_active, @created_by)

RETURNING id, name, url, secret, event_types, status, is_active, verified_at, created_by, created_at, updated_at;

-- name: UpdateWebhookEndpoint :one
UPDATE webhook_endpoints
SET name = @name, url = @url, secret = @secret, event_types = @event_types,
 status = @status, is_active = @is_active, updated_at = now()
WHERE id = @id
RETURNING id, name, url, secret, event_types, status, is_active, verified_at, created_by, created_at, updated_at;

-- name: VerifyWebhookEndpoint :exec
UPDATE webhook_endpoints
SET is_active = true, verified_at = now(), status = 'active', updated_at = now()
WHERE id = @id;

-- name: DeleteWebhookEndpoint :execrows
DELETE FROM webhook_endpoints
WHERE id = @id;

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (endpoint_id, event_id, event_type, payload) VALUES (@endpoint_id, @event_id, @event_type, @payload)

RETURNING id;

-- name: GetWebhookDelivery :one
SELECT id, endpoint_id, event_id, event_type, payload,
 status, attempts, last_error, next_attempt_at, last_response_code, last_response_body,
 delivered_at, created_at
FROM webhook_deliveries
WHERE id = @id;

-- name: GetWebhookDeliveryForPlatform :one
SELECT id, endpoint_id, event_id, event_type, payload,
 status, attempts, last_error, next_attempt_at, last_response_code, last_response_body,
 delivered_at, created_at
FROM webhook_deliveries
WHERE id = @id;

-- name: ListWebhookDeliveries :many
SELECT id, endpoint_id, event_id, event_type, payload,
 status, attempts, last_error, next_attempt_at, last_response_code, last_response_body,
 delivered_at, created_at
FROM webhook_deliveries
WHERE endpoint_id = @endpoint_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: MarkWebhookDelivered :exec
UPDATE webhook_deliveries
SET status = 'delivered', attempts = @attempts, delivered_at = @delivered_at,
 last_error = NULL, next_attempt_at = NULL,
 last_response_code = @last_response_code, last_response_body = @last_response_body
WHERE id = @id;

-- name: MarkWebhookFailed :exec
UPDATE webhook_deliveries
SET status = 'failed', attempts = @attempts, last_error = @last_error,
 next_attempt_at = NULL
WHERE id = @id;

-- name: MarkWebhookRetrying :exec
UPDATE webhook_deliveries
SET status = 'retrying', attempts = @attempts, last_error = @last_error,
 next_attempt_at = @next_attempt_at,
 last_response_code = @last_response_code, last_response_body = @last_response_body
WHERE id = @id;

-- name: RetryWebhookDelivery :execrows
UPDATE webhook_deliveries
SET status = 'pending', attempts = 0, last_error = NULL, next_attempt_at = NULL
WHERE id = @id AND status = 'failed';
