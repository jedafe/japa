-- 000481_planning_group_rbac_all_tenants.down.sql
-- Reverses 000481 by removing the all-tenant grants added here. The
-- original 000307 grants (tenant 1 only) are unaffected and remain.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'planning_group.manage_own'
)
AND role_id IN (
    SELECT id FROM user_roles WHERE slug IN ('member', 'super_admin')
);

-- Remove only the all-tenant grants for create/manage_any that 000481
-- added. We cannot distinguish them from 000307's tenant-1 grants by
-- role_id alone, so we remove all grants for these keys outside tenant 1
-- (000307 only seeded tenant 1).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'planning_group.create'
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE slug IN ('member', 'super_admin')
      AND tenant_id <> '00000000-0000-0000-0000-000000000001'
);

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'planning_group.manage_any'
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE slug IN ('admin', 'super_admin')
      AND tenant_id <> '00000000-0000-0000-0000-000000000001'
);

COMMIT;
