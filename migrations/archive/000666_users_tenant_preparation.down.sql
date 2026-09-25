-- Revert Phase 1.2 users subsystem preparation.
DROP INDEX IF EXISTS idx_users_email_global;
DROP INDEX IF EXISTS idx_users_username_global;
DROP INDEX IF EXISTS idx_user_profiles_user_id_global;
DROP INDEX IF EXISTS idx_user_preferences_user_id_global;
DROP INDEX IF EXISTS idx_oauth_accounts_provider_user_id_global;
DROP INDEX IF EXISTS idx_oauth_accounts_user_provider_global;
DROP INDEX IF EXISTS idx_refresh_tokens_user_id;

ALTER TABLE user_privacy_settings DROP CONSTRAINT user_privacy_settings_pkey;
ALTER TABLE user_privacy_settings ADD PRIMARY KEY (tenant_id, user_id);
