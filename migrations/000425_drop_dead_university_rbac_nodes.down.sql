-- 000425_drop_dead_university_rbac_nodes.down.sql
-- Re-seeds the two dead university RBAC nodes that 000425 dropped, and
-- re-grants them to super_admin_role. NOTE: these nodes are intentionally
-- NOT enforced by any RequirePermission call (see 000425 up.sql for why);
-- this down migration restores the pre-000425 state for rollback only.

BEGIN;

-- 1. Re-insert the two permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('university.enrollment.view_own','View own course enrollments and progress'),
    ('university.review.moderate',    'Moderate course reviews (flag/remove/restore)')
ON CONFLICT (key) DO NOTHING;

-- 2. Re-grant to super_admin_role (idempotent).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN ('university.review.moderate')
ON CONFLICT DO NOTHING;

COMMIT;
