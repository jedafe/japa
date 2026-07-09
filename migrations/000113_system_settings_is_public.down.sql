DROP INDEX IF EXISTS idx_system_settings_is_public;
ALTER TABLE system_settings DROP COLUMN IF EXISTS is_public;
