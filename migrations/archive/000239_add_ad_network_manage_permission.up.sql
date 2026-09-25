-- 000239_add_ad_network_manage_permission.up.sql
-- Seed the ad.network.manage permission node for the ad-networks-settings feature.
-- Routes (Task 6) already enforce rbac.RequirePermission("ad.network.manage");
-- without this node no role (incl. super_admin) can hold it.

BEGIN;

-- 1. Ensure the permission node exists.
INSERT INTO permissions (key, description)
VALUES ('ad.network.manage', 'Ad Network Manage')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant super_admin the new node.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'ad.network.manage'
ON CONFLICT DO NOTHING;

COMMIT;
