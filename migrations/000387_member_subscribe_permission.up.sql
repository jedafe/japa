-- 000387_member_subscribe_permission.up.sql
-- Grant payments.subscriptions.manage to the member role so regular users
-- can self-subscribe via POST /subscriptions. Previously only super_admin
-- (via wildcard bypass) and admin roles could create subscriptions.

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'payments.subscriptions.manage'
ON CONFLICT DO NOTHING;
