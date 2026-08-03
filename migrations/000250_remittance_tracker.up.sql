-- 000250_remittance_tracker.up.sql
-- Remittance Rate Tracker (Section 74, ADR-075).
-- Creates remittance_providers, remittance_rates, remittance_rate_alerts
-- tables, seeds remittance.* system_settings, RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/74-remittance-tracker.md
--      japaconnect-engineering/adr/ADR-075-remittance-rate-tracker.md

BEGIN;

-- =====================================================================
-- remittance_providers — Wise, Remitly, WorldRemit, Sendwave, etc.
-- data_source distinguishes API-sourced vs manual-curation providers.
-- api_config stores encrypted provider API credentials (§17 treatment).
-- =====================================================================
CREATE TABLE IF NOT EXISTS remittance_providers (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name           text NOT NULL,
    slug           text NOT NULL,
    logo_media_id  uuid REFERENCES media(id),
    website_url    text,
    data_source    text NOT NULL CHECK (data_source IN ('api','manual')),
    api_config     jsonb,
    is_active      boolean NOT NULL DEFAULT true,
    display_order  int  NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_remittance_providers_tenant
    ON remittance_providers (tenant_id, is_active, display_order);

-- =====================================================================
-- remittance_rates — live rate snapshots (one row per provider+corridor+
-- fetch). All rows retained for history; no delete on refresh.
-- =====================================================================
CREATE TABLE IF NOT EXISTS remittance_rates (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider_id      uuid NOT NULL REFERENCES remittance_providers(id) ON DELETE CASCADE,
    send_currency    text NOT NULL,
    receive_currency text NOT NULL,
    rate             numeric(18,6) NOT NULL,
    fee_flat         numeric(10,2),
    fee_pct          numeric(5,4),
    transfer_time    text,
    fetched_at       timestamptz NOT NULL DEFAULT now(),
    is_manual        boolean NOT NULL DEFAULT false,
    source_url       text
);

CREATE INDEX IF NOT EXISTS idx_remittance_rates_corridor
    ON remittance_rates (tenant_id, send_currency, receive_currency, provider_id, fetched_at DESC);

CREATE INDEX IF NOT EXISTS idx_remittance_rates_history
    ON remittance_rates (tenant_id, send_currency, receive_currency, fetched_at DESC);

-- =====================================================================
-- remittance_rate_alerts — user subscriptions to corridor + threshold.
-- =====================================================================
CREATE TABLE IF NOT EXISTS remittance_rate_alerts (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    send_currency    text NOT NULL,
    receive_currency text NOT NULL,
    target_rate      numeric(18,6) NOT NULL,
    direction        text NOT NULL DEFAULT 'above' CHECK (direction IN ('above','below')),
    is_active        boolean NOT NULL DEFAULT true,
    last_triggered_at timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id, send_currency, receive_currency, direction)
);

CREATE INDEX IF NOT EXISTS idx_rate_alerts_active
    ON remittance_rate_alerts (tenant_id, send_currency, receive_currency, is_active)
    WHERE is_active = true;

-- =====================================================================
-- system_settings — 5 remittance.* keys (Section 74, ADR-075)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'remittance.enabled',                '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'remittance.fetch_interval_minutes',  '"30"'),
    ('00000000-0000-0000-0000-000000000001', 'remittance.manual_stale_hours',      '"24"'),
    ('00000000-0000-0000-0000-000000000001', 'remittance.history_retention_days',  '"365"'),
    ('00000000-0000-0000-0000-000000000001', 'remittance.default_send_currency',   '"GBP"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 4 remittance.* nodes (Section 74, ADR-075)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('remittance.view',   'View remittance tracker and rates (all users incl. anonymous)'),
    ('remittance.alert',  'Create/manage rate alerts (authenticated users)'),
    ('remittance.manage', 'Admin: manage providers, enter manual rates, configure'),
    ('remittance.fetch',  'Trigger manual rate fetch (admin only)')
ON CONFLICT (key) DO NOTHING;

-- Grant remittance.manage + remittance.fetch to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('remittance.manage', 'remittance.fetch')
ON CONFLICT DO NOTHING;

-- Grant remittance.view + remittance.alert to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('remittance.view', 'remittance.alert')
ON CONFLICT DO NOTHING;

COMMIT;
