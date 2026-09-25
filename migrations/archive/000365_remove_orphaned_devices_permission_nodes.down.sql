-- 000365_remove_orphaned_devices_permission_nodes.down.sql
-- Reverse migration 000365: re-add the orphaned devices.manage/devices.admin
-- permission nodes and restore their role grants.
-- NOTE: these nodes are NOT in the permission-key-naming catalog. They are
-- re-created only to make the down-migration reversible; the code no longer
-- enforces them. See the up-migration header for context.
-- Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md

BEGIN;

-- Re-add the orphaned permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('devices.manage', 'Register and deregister own push device tokens'),
    ('devices.admin',  'Admin: send test push notifications to any device')
ON CONFLICT (key) DO NOTHING;

-- Restore super_admin grants for both nodes.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('devices.manage', 'devices.admin')
ON CONFLICT DO NOTHING;

-- Restore member grant for devices.manage.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'devices.manage'
ON CONFLICT DO NOTHING;

COMMIT;
