-- 000183_seed_university_revenue_rbac_nodes.down.sql
-- Rollback: remove the university.revenue.* grants and permission nodes
-- seeded for the admin revenue-split audit viewer (ADR-063 §9).

-- 1. Revoke the grants from super_admin_role.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN ('university.revenue.list', 'university.revenue.view');

-- 2. Remove the permission nodes.
DELETE FROM permissions
WHERE key IN ('university.revenue.list', 'university.revenue.view');
