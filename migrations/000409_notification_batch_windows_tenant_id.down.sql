-- Down migration for 000409: remove tenant_id from notification_batch_windows.
DROP INDEX IF EXISTS idx_notification_batch_windows_tenant;
ALTER TABLE notification_batch_windows DROP COLUMN IF EXISTS tenant_id;
