-- Revoke payments.subscriptions.manage from the member role.

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'payments.subscriptions.manage')
  AND role_id = (
      SELECT id FROM user_roles
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug = 'member'
  );
