-- 000255_travel_search.up.sql
-- Travel Search & Compare (Section 67, ADR-085).
-- Creates the travel_providers table for affiliate travel API config,
-- and seeds travel.* system_settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/67-travel-search.md
--      japaconnect-engineering/adr/ADR-085-travel-search.md

BEGIN;

-- =====================================================================
-- travel_providers — affiliate travel API provider configuration.
-- api_config stores encrypted API credentials (§17 treatment).
-- affiliate_config stores tracking params injected at render time (ADR-009).
-- =====================================================================
CREATE TABLE IF NOT EXISTS travel_providers (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name             text NOT NULL,
    slug             text NOT NULL,
    provider_type    text NOT NULL CHECK (provider_type IN ('flight','hotel','both')),
    api_config       jsonb,
    affiliate_config jsonb,
    is_active        boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_travel_providers_tenant
    ON travel_providers (tenant_id, is_active);

-- =====================================================================
-- system_settings — 2 travel.* keys (Section 67, ADR-085)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'travel.enabled',           '"false"'),
    ('00000000-0000-0000-0000-000000000001', 'travel.cache_ttl_seconds',  '"300"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 2 travel.* nodes (Section 67, ADR-085)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('travel.search', 'Search flights/hotels (all users incl. anonymous)'),
    ('travel.manage', 'Admin: configure travel providers')
ON CONFLICT (key) DO NOTHING;

-- Grant travel.manage to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'travel.manage'
ON CONFLICT DO NOTHING;

-- Grant travel.search to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'travel.search'
ON CONFLICT DO NOTHING;

COMMIT;
