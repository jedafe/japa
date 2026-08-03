-- 000257_mobile_push_devices.down.sql
-- Reverses 000257_mobile_push_devices.up.sql.

BEGIN;

-- Revoke push.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('push.manage', 'push.receive');

-- Remove push.* permission nodes.
DELETE FROM permissions WHERE key IN ('push.manage', 'push.receive');

-- Remove push.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('push.fcm_server_key', 'push.apns_key_id', 'push.apns_team_id',
              'push.apns_private_key', 'push.enabled');

-- Drop the device_tokens table.
DROP TABLE IF EXISTS device_tokens;

COMMIT;
