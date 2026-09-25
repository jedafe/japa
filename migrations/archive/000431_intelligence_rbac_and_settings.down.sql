-- 000431_intelligence_rbac_and_settings.down.sql
BEGIN;

-- Remove system_settings
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key LIKE 'intel.%';

-- Remove role_permissions for intel.* nodes
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key LIKE 'intel.%';

-- Remove permission nodes
DELETE FROM permissions WHERE key LIKE 'intel.%';

COMMIT;
