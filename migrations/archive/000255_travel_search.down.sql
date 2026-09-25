-- 000255_travel_search.down.sql
-- Reverses 000255_travel_search.up.sql.

BEGIN;

-- Revoke travel.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('travel.search', 'travel.manage');

-- Remove travel.* permission nodes.
DELETE FROM permissions WHERE key IN ('travel.search', 'travel.manage');

-- Remove travel.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('travel.enabled', 'travel.cache_ttl_seconds');

-- Drop the travel_providers table.
DROP TABLE IF EXISTS travel_providers;

COMMIT;
