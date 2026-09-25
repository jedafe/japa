-- 000464_seed_push_fcm_service_account_bundle_id.down.sql
-- Reverses 000464_seed_push_fcm_service_account_bundle_id.up.sql.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('push.fcm_service_account', 'push.apns_bundle_id');

COMMIT;
