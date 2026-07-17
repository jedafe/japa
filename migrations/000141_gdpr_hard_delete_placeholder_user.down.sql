-- 000141_gdpr_hard_delete_placeholder_user.down.sql
-- Removes the Deleted User placeholder and its setting.

BEGIN;

DELETE FROM system_settings WHERE key = 'gdpr.deleted_user_placeholder_uuid';
DELETE FROM users WHERE id = 'ffffffff-0000-0000-0000-000000000001';

COMMIT;
