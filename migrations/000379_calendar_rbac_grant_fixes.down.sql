-- 000379_calendar_rbac_grant_fixes.down.sql
BEGIN;

-- Remove calendar.review grants from moderator and admin.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('moderator', 'admin')
  AND p.key = 'calendar.review';

-- Remove calendar.manage grant from admin.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin')
  AND p.key = 'calendar.manage';

COMMIT;
