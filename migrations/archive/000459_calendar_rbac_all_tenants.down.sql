-- 000459_calendar_rbac_all_tenants.down.sql
-- Reverses 000459_calendar_rbac_all_tenants.up.sql.
-- Removes calendar RBAC grants from all non-default tenants.
-- Grants in the default tenant (from 000315/000379) are preserved.

BEGIN;

-- Remove calendar.submit from member/super_admin in all non-default tenants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'calendar.submit'
  AND ur.tenant_id != '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin');

-- Remove calendar.review from moderator/admin/super_admin in all non-default tenants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'calendar.review'
  AND ur.tenant_id != '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('moderator', 'admin', 'super_admin');

-- Remove calendar.manage from admin/super_admin in all non-default tenants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'calendar.manage'
  AND ur.tenant_id != '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin');

COMMIT;
