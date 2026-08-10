-- 000258_advanced_analytics.down.sql
-- Reverses 000258_advanced_analytics.up.sql.

BEGIN;

-- Revoke analytics.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND rp.permission_id = p.id
  AND p.key IN ('analytics.advanced', 'analytics.export');

-- Remove analytics.* permission nodes.
DELETE FROM permissions WHERE key IN ('analytics.advanced', 'analytics.export');

-- Remove analytics.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'analytics.export_enabled';

-- Drop materialized views.
DROP MATERIALIZED VIEW IF EXISTS mv_revenue_streams;
DROP MATERIALIZED VIEW IF EXISTS mv_top_content_90d;
DROP MATERIALIZED VIEW IF EXISTS mv_top_content_30d;
DROP MATERIALIZED VIEW IF EXISTS mv_top_content_7d;

COMMIT;
