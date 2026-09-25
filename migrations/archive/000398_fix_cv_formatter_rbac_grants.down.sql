-- 000398_fix_cv_formatter_rbac_grants.down.sql
-- Revert the RBAC grant fixes for CV Formatter (§95).

BEGIN;

-- Remove cv_formatter.unlimited from premium role.
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'cv_formatter.unlimited')
  AND role_id = (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'premium'
    LIMIT 1
  );

-- Remove cv_formatter.manage from admin role.
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'cv_formatter.manage')
  AND role_id = (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'admin'
    LIMIT 1
  );

COMMIT;
