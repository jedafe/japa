-- 000208_user_privacy_settings_tenant_id.down.sql
-- Reverse of 000208: drop tenant_id from user_privacy_settings.

DROP INDEX IF EXISTS idx_user_privacy_settings_tenant;

ALTER TABLE user_privacy_settings DROP CONSTRAINT IF EXISTS user_privacy_settings_pkey;
ALTER TABLE user_privacy_settings ADD PRIMARY KEY (user_id);

ALTER TABLE user_privacy_settings DROP COLUMN IF EXISTS tenant_id;
