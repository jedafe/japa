-- 000400_fix_digest_rbac_admin_grant.down.sql
-- Revert: remove the admin role grant for digest.manage added by 000400.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id FROM permissions WHERE key = 'digest.manage'
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'admin'
);

COMMIT;
