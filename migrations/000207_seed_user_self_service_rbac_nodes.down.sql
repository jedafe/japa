-- 000207_seed_user_self_service_rbac_nodes.down.sql
-- Reverse of 000207: revoke and remove the 4 self-service RBAC nodes.

BEGIN;

-- Revoke from super_admin_role and member role.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'user.profile.edit_own',
        'user.account.manage_own',
        'user.data.export_own',
        'user.account.delete_own'
    )
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('super_admin', 'member')
);

-- Remove the permission nodes.
DELETE FROM permissions
WHERE key IN (
    'user.profile.edit_own',
    'user.account.manage_own',
    'user.data.export_own',
    'user.account.delete_own'
);

COMMIT;
