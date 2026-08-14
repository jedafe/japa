-- 000479_mutual_aid_rbac_all_tenants.down.sql
-- Reverses 000479: removes the all-tenant RBAC grants added for mutual aid.
-- Only removes grants that match the role slugs this migration added; the
-- original 000313 grants (tenant 1, member+super_admin for create_offer/
-- create_skill_swap, super_admin for moderate) are left intact so the down
-- migration is reversible without losing the original seed state.

BEGIN;

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'mutual_aid.create_need'
  AND ur.slug IN ('member', 'super_admin');

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'mutual_aid.moderate'
  AND ur.slug IN ('moderator', 'admin');

COMMIT;
