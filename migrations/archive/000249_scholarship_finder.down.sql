-- 000249_scholarship_finder.down.sql
-- Reverses 000249_scholarship_finder.up.sql.

BEGIN;

-- Revoke scholarship.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('scholarship.manage', 'scholarship.browse', 'scholarship.save');

-- Remove scholarship.* permission nodes.
DELETE FROM permissions WHERE key IN ('scholarship.browse', 'scholarship.save', 'scholarship.manage');

-- Remove scholarship_finder.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('scholarship_finder.enabled', 'scholarship_finder.default_reminder_days', 'scholarship_finder.completion_points');

-- Drop the saved_scholarships table.
DROP TABLE IF EXISTS saved_scholarships;

COMMIT;
