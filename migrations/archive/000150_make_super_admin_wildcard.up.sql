-- 000150_make_super_admin_wildcard.up.sql
-- Remove the redundant exhaustive list of per-node permission grants from the
-- super-admin role(s) and rely on the wildcard "*" node instead. The RBAC
-- resolver (internal/rbac/rbac.go) now short-circuits HasPermission to true
-- whenever a user holds the "*" node, so an explicit grant per node is dead
-- weight that must be kept in sync by hand.
--
-- Idempotent; safe to re-run.

BEGIN;

-- 1. Ensure the wildcard node exists (keep whatever id is already present).
INSERT INTO permissions (key, description)
VALUES ('*', 'Wildcard permission — grants all access (super admin)')
ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description;

-- 2. Grant the wildcard node to the super-admin role(s).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key = '*'
ON CONFLICT DO NOTHING;

-- 3. Drop every NON-wildcard grant from the super-admin role(s).
--    After this, super-admin holds exactly one node: "*".
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key <> '*';

COMMIT;
