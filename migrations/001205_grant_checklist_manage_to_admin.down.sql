-- 001205_grant_checklist_manage_to_admin.down.sql
-- Reverses the admin/admin-group-role grant added in 001205.

BEGIN;

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.slug IN ('admin', 'admin-group-role')
  AND p.key = 'checklist.manage';

COMMIT;
