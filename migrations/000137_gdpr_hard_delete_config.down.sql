-- 000137_gdpr_hard_delete_config.down.sql
-- Removes GDPR hard-delete configuration settings.

BEGIN;

DELETE FROM system_settings WHERE key IN ('gdpr.hard_delete_enabled', 'gdpr.retention_days');

COMMIT;
