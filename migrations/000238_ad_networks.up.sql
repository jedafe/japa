CREATE TABLE IF NOT EXISTS ad_networks (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id),
    provider        text NOT NULL CHECK (provider IN ('adsense', 'ezoic', 'media_net', 'custom')),
    name            text NOT NULL,
    client_id       text,
    api_key         text,
    slot_mapping    jsonb DEFAULT '{}',
    config          jsonb DEFAULT '{}',
    is_active       boolean NOT NULL DEFAULT false,
    priority        int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_ad_networks_tenant ON ad_networks (tenant_id);

CREATE OR REPLACE FUNCTION update_ad_networks_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ad_networks_updated_at ON ad_networks;
CREATE TRIGGER trg_ad_networks_updated_at
    BEFORE UPDATE ON ad_networks
    FOR EACH ROW EXECUTE FUNCTION update_ad_networks_updated_at();
