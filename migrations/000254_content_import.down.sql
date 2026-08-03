-- 000254_content_import.down.sql
-- Reverses 000254_content_import.up.sql.

BEGIN;

-- Revoke import.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('import.use', 'import.manage');

-- Remove import.* permission nodes.
DELETE FROM permissions WHERE key IN ('import.use', 'import.manage');

-- Remove import.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'import.max_items_per_run';

-- Drop the content_imports table.
DROP TABLE IF EXISTS content_imports;

COMMIT;
