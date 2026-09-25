-- 000150_make_super_admin_wildcard.down.sql
-- Rollback: restore the exhaustive per-node grants on the super-admin role(s)
-- (the redundant state that preceded this migration). The wildcard node is
-- left in place; it simply becomes a redundant literal again until the
-- resolver re-grants it meaning.

BEGIN;

-- Re-grant every permission node to the super-admin role(s).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'super_admin_role')
ON CONFLICT DO NOTHING;

COMMIT;
