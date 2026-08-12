-- 000426_grant_visa_routes_manage_to_admin.down.sql
-- Reverses the admin role grant added in 000426.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'visa_routes.manage')
  AND role_id IN (
      SELECT id FROM user_roles
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug = 'admin'
  );

COMMIT;
