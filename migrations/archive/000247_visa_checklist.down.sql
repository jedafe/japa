-- 000247_visa_checklist.down.sql
-- Reverses 000247_visa_checklist.up.sql.

BEGIN;

-- Revoke checklist.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('checklist.manage', 'checklist.use');

-- Remove checklist.* permission nodes.
DELETE FROM permissions WHERE key IN ('checklist.use', 'checklist.manage');

-- Remove checklist.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('checklist.enabled', 'checklist.completion_points');

-- Drop the checklist_progress table.
DROP TABLE IF EXISTS checklist_progress;

COMMIT;
