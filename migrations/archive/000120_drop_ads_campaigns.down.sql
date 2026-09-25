-- Recreate the dead ads_campaigns table (for rollback purposes)
CREATE TABLE IF NOT EXISTS ads_campaigns (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        text NOT NULL,
    status      text NOT NULL DEFAULT 'draft',
    created_at  timestamptz NOT NULL DEFAULT now()
);
