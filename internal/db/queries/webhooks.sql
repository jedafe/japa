-- Outgoing Webhooks — endpoint CRUD + delivery audit rows (Section 11 job-queue extension)

-- name: ListWebhookEndpoints :many
SELECT id, tenant_id, url, secret, event_types, status, created_at, updated_at
FROM webhook_endpoints
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: GetWebhookEndpoint :one
SELECT id, tenant_id, url, secret, event_types, status, created_at, updated_at
FROM webhook_endpoints
WHERE tenant_id = @tenant_id AND id = @id;

-- name: GetWebhookEndpointByID :one
SELECT id, tenant_id, url, secret, event_types, status
FROM webhook_endpoints
WHERE id = @id;

-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (tenant_id, url, secret, event_types, status)
VALUES (@tenant_id, @url, @secret, @event_types, @status)
RETURNING id, tenant_id, url, secret, event_types, status, created_at, updated_at;

-- name: UpdateWebhookEndpoint :one
UPDATE webhook_endpoints
SET url = @url, secret = @secret, event_types = @event_types,
    status = @status, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, url, secret, event_types, status, created_at, updated_at;

-- name: DeleteWebhookEndpoint :execrows
DELETE FROM webhook_endpoints
WHERE tenant_id = @tenant_id AND id = @id;

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (tenant_id, endpoint_id, event_id, event_type, payload)
VALUES (@tenant_id, @endpoint_id, @event_id, @event_type, @payload)
RETURNING id;

-- name: GetWebhookDelivery :one
SELECT id, tenant_id, endpoint_id, event_id, event_type, payload,
       status, attempts, last_error, delivered_at, created_at
FROM webhook_deliveries
WHERE id = @id;

-- name: GetWebhookDeliveryForTenant :one
SELECT id, tenant_id, endpoint_id, event_id, event_type, payload,
       status, attempts, last_error, delivered_at, created_at
FROM webhook_deliveries
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListWebhookDeliveries :many
SELECT id, tenant_id, endpoint_id, event_id, event_type, payload,
       status, attempts, last_error, delivered_at, created_at
FROM webhook_deliveries
WHERE tenant_id = @tenant_id AND endpoint_id = @endpoint_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: MarkWebhookDelivered :exec
UPDATE webhook_deliveries
SET status = 'delivered', attempts = @attempts, delivered_at = @delivered_at, last_error = NULL
WHERE id = @id;

-- name: MarkWebhookFailed :exec
UPDATE webhook_deliveries
SET status = 'failed', attempts = @attempts, last_error = @last_error
WHERE id = @id;

-- name: RetryWebhookDelivery :execrows
UPDATE webhook_deliveries
SET status = 'pending', attempts = 0, last_error = NULL
WHERE tenant_id = @tenant_id AND id = @id AND status = 'failed';
