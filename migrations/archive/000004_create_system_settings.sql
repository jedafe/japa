-- System settings table for admin-configurable platform settings.
-- Each setting is scoped to a tenant and accessed by a dot-notation key
-- (e.g. auth.password.min_length, feature.blog).
CREATE TABLE system_settings (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key         text NOT NULL,
    value       jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, key)
);

CREATE INDEX idx_system_settings_tenant_key ON system_settings(tenant_id, key);
