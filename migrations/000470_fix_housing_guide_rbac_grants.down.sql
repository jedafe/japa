-- 000470_fix_housing_guide_rbac_grants.down.sql
-- Reverses 000470_fix_housing_guide_rbac_grants.up.sql by removing the
-- grants added there. The permission nodes themselves (inserted by
-- 000336) are left in place; only the role_permissions rows added by
-- 000470 are removed.

BEGIN;

-- Remove housing_guide.manage grant from the admin role.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin')
  AND p.key = 'housing_guide.manage';

-- Remove housing_guide.view grants from all member-level roles.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin', 'super_admin')
  AND p.key = 'housing_guide.view';

COMMIT;
