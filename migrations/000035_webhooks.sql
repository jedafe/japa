-- Section 11 (Job Queue extension): Outgoing Webhook Delivery
--
-- Stores endpoint configurations and per-delivery audit rows so the Asynq
-- webhook_delivery task can fetch, sign, send, and retry outgoing webhooks
-- with full observability.

CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id),
    url         text NOT NULL,
    secret      text NOT NULL,
    event_types text[] NOT NULL DEFAULT '{}',
    status      text NOT NULL DEFAULT 'active',  -- 'active' | 'disabled'
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhook_endpoints_tenant ON webhook_endpoints(tenant_id, status);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id),
    endpoint_id  uuid NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    event_id     uuid NOT NULL,
    event_type   text NOT NULL,
    payload      jsonb NOT NULL,
    status       text NOT NULL DEFAULT 'pending',  -- 'pending' | 'delivered' | 'failed'
    attempts     int NOT NULL DEFAULT 0,
    last_error   text,
    delivered_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhook_deliveries_endpoint ON webhook_deliveries(endpoint_id, status);
CREATE INDEX idx_webhook_deliveries_pending ON webhook_deliveries(status, created_at) WHERE status = 'pending';
