-- 000256_self_serve_ads.down.sql
-- Reverses 000256_self_serve_ads.up.sql.

BEGIN;

-- Revoke ads.self_serve.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('ads.self_serve.create', 'ads.self_serve.manage', 'ads.self_serve.stats');

-- Remove ads.self_serve.* permission nodes.
DELETE FROM permissions WHERE key IN ('ads.self_serve.create', 'ads.self_serve.manage', 'ads.self_serve.stats');

-- Remove ads.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('ads.self_serve_enabled', 'ads.min_campaign_days', 'ads.credit_refund_on_reject');

-- Drop the added columns.
DROP INDEX IF EXISTS idx_ad_campaigns_org;
ALTER TABLE ad_campaigns DROP COLUMN IF EXISTS budget_credits;
ALTER TABLE ad_campaigns DROP COLUMN IF EXISTS flat_rate;
ALTER TABLE ad_campaigns DROP COLUMN IF EXISTS organization_id;
ALTER TABLE ad_slot_types DROP COLUMN IF EXISTS flat_rate;

COMMIT;
