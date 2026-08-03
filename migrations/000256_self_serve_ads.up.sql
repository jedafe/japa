-- 000256_self_serve_ads.up.sql
-- Self-Serve Advertising (Section 61, ADR-079).
-- Adds organization_id, flat_rate, budget_credits columns to ad_campaigns,
-- flat_rate to ad_slot_types, and seeds ads.* system_settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/61-self-serve-ads.md
--      japaconnect-engineering/adr/ADR-079-self-serve-ads.md

BEGIN;

-- =====================================================================
-- ad_slot_types: add flat_rate column (ADR-018: flat-rate pricing).
-- The cost per day for this slot type, in credits.
-- =====================================================================
ALTER TABLE ad_slot_types ADD COLUMN IF NOT EXISTS flat_rate int NOT NULL DEFAULT 100;

-- =====================================================================
-- ad_campaigns: add organization_id, flat_rate, budget_credits columns.
-- organization_id links the campaign to the org that created it (self-serve).
-- flat_rate snapshots the slot type's rate at campaign creation time.
-- budget_credits holds the upfront-paid credits in escrow.
-- =====================================================================
ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations(id) ON DELETE SET NULL;
ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS flat_rate int NOT NULL DEFAULT 0;
ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS budget_credits int NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_ad_campaigns_org
    ON ad_campaigns (tenant_id, organization_id, status)
    WHERE organization_id IS NOT NULL;

-- =====================================================================
-- system_settings — 3 ads.* keys (Section 61, ADR-079)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'ads.self_serve_enabled',    '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'ads.min_campaign_days',     '"3"'),
    ('00000000-0000-0000-0000-000000000001', 'ads.credit_refund_on_reject', '"true"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 3 ads.self_serve.* nodes (Section 61, ADR-079)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('ads.self_serve.create', 'Create self-serve ad campaigns (verified org admins)'),
    ('ads.self_serve.manage', 'Manage own ad campaigns (pause/resume/edit)'),
    ('ads.self_serve.stats',  'View own campaign statistics')
ON CONFLICT (key) DO NOTHING;

-- Grant all 3 ads.self_serve.* to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('ads.self_serve.create', 'ads.self_serve.manage', 'ads.self_serve.stats')
ON CONFLICT DO NOTHING;

-- Grant all 3 ads.self_serve.* to the default 'member' role (org admins
-- are members with organization_id; the handler checks org verification
-- and org membership before allowing operations).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('ads.self_serve.create', 'ads.self_serve.manage', 'ads.self_serve.stats')
ON CONFLICT DO NOTHING;

COMMIT;
